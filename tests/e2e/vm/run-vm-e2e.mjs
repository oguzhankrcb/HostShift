#!/usr/bin/env node
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const vmDir = fileURLToPath(new URL(".", import.meta.url));
const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const matrixPath = path.join(vmDir, "matrix.yaml");
const planTemplatePath = path.join(vmDir, "providers/lima/instance-plan.json.tmpl");
const commonBootstrap = path.join(vmDir, "fixtures/common-bootstrap.sh");
const sourceBootstrap = path.join(vmDir, "fixtures/source-bootstrap.sh");
const targetBootstrap = path.join(vmDir, "fixtures/target-bootstrap.sh");
const nodeHostShiftBin = path.join(repoRoot, "bin/hostshift.js");
const goHostShiftBin = path.join(repoRoot, "dist/hostshift");
const defaultHostShiftBin = fs.existsSync(goHostShiftBin) ? goHostShiftBin : nodeHostShiftBin;

const args = parseArgs(process.argv.slice(2));
const config = loadMatrix(matrixPath);
validateMatrix(config);

const providerName = args.provider || process.env.HOSTSHIFT_VM_PROVIDER || config.providers.default;
const provider = config.providers[providerName];
if (!provider) {
  console.error(`unknown VM provider: ${providerName}`);
  process.exit(2);
}

const selectedPairs = selectPairs(config.pairs, args.pair);
if (args.pair && selectedPairs.length === 0) {
  console.error(`unknown matrix pair: ${args.pair}`);
  process.exit(2);
}

if (args.list) {
  for (const pair of selectedPairs) {
    console.log(`${pair.source} -> ${pair.target}`);
  }
  process.exit(0);
}

const runVm = process.env.HOSTSHIFT_RUN_VM_E2E === "1";
console.log(`HostShift VM e2e matrix: ${selectedPairs.length} pairs (provider: ${providerName})`);

try {
  runProviderPreflight(providerName, provider);

  const workspaces = selectedPairs.map((pair) => {
    console.log(`${pair.source} -> ${pair.target}`);
    const workspace = renderPairWorkspace({
      pair,
      platforms: config.platforms,
      providerName,
      emitDir: args.emitDir
    });
    console.log(`  rendered workspace: ${workspace.workspaceDir}`);
    return workspace;
  });

  if (!runVm) {
    console.log("Dry-run only. Set HOSTSHIFT_RUN_VM_E2E=1 for provider preflight and VM boot. Add --apply to execute the real provider workflow.");
    process.exit(0);
  }

  for (const workspace of workspaces) {
    console.log(`  provider preflight ok: ${workspace.pair.source} -> ${workspace.pair.target}`);
  }

  if (!args.apply) {
    console.log("Provider preflight completed. Add --apply to boot VMs, assemble SSH config, and run HostShift discover/plan dry-runs.");
    process.exit(0);
  }

  for (const workspace of workspaces) {
    executePair(workspace);
  }

  console.log("VM apply executor completed successfully.");
} catch (error) {
  if (error instanceof Error) {
    console.error(error.message);
  } else {
    console.error(String(error));
  }
  process.exit(1);
}

function parseArgs(argv) {
  const parsed = {
    list: false,
    apply: false,
    pair: "",
    provider: "",
    emitDir: ""
  };

  for (let index = 0; index < argv.length; index += 1) {
    const value = argv[index];
    switch (value) {
      case "--list":
        parsed.list = true;
        break;
      case "--apply":
        parsed.apply = true;
        break;
      case "--pair":
        parsed.pair = argv[index + 1] ?? "";
        index += 1;
        break;
      case "--provider":
        parsed.provider = argv[index + 1] ?? "";
        index += 1;
        break;
      case "--emit-dir":
        parsed.emitDir = argv[index + 1] ?? "";
        index += 1;
        break;
      default:
        throw new Error(`unknown argument: ${value}`);
    }
  }

  return parsed;
}

function loadMatrix(filename) {
  return JSON.parse(fs.readFileSync(filename, "utf8"));
}

function validateMatrix(config) {
  if (!config || typeof config !== "object") {
    throw new Error("invalid VM matrix: expected object");
  }
  if (!config.providers || typeof config.providers !== "object") {
    throw new Error("invalid VM matrix: missing providers");
  }
  if (!config.platforms || typeof config.platforms !== "object") {
    throw new Error("invalid VM matrix: missing platforms");
  }
  if (!Array.isArray(config.pairs) || config.pairs.length === 0) {
    throw new Error("invalid VM matrix: missing pairs");
  }

  const seen = new Set();
  for (const pair of config.pairs) {
    const source = config.platforms[pair.source];
    const target = config.platforms[pair.target];
    if (!source) {
      throw new Error(`invalid VM matrix: unknown source platform ${pair.source}`);
    }
    if (!target) {
      throw new Error(`invalid VM matrix: unknown target platform ${pair.target}`);
    }
    if (!source.templateUrl) {
      throw new Error(`invalid VM matrix: missing templateUrl for ${pair.source}`);
    }
    if (!target.templateUrl) {
      throw new Error(`invalid VM matrix: missing templateUrl for ${pair.target}`);
    }
    const key = `${pair.source}->${pair.target}`;
    if (seen.has(key)) {
      throw new Error(`invalid VM matrix: duplicate pair ${key}`);
    }
    seen.add(key);
  }
}

function selectPairs(pairs, filter) {
  if (!filter) {
    return pairs;
  }
  return pairs.filter((pair) => `${pair.source}->${pair.target}` === filter);
}

function renderPairWorkspace({ pair, platforms, providerName, emitDir }) {
  const pairKey = `${pair.source}-to-${pair.target}`;
  const baseDir = emitDir ? path.resolve(emitDir) : fs.mkdtempSync(path.join(os.tmpdir(), `hostshift-vm-${pairKey}-`));
  const workspaceDir = emitDir ? path.join(baseDir, pairKey) : baseDir;
  fs.mkdirSync(workspaceDir, { recursive: true });

  const fixtureDir = path.join(workspaceDir, "fixtures");
  fs.mkdirSync(fixtureDir, { recursive: true });
  copyFixture(commonBootstrap, path.join(fixtureDir, "common-bootstrap.sh"));
  copyFixture(sourceBootstrap, path.join(fixtureDir, "source-bootstrap.sh"));
  copyFixture(targetBootstrap, path.join(fixtureDir, "target-bootstrap.sh"));

  const sourcePlan = renderProviderPlan({
    providerName,
    pairKey,
    role: "source",
    platformKey: pair.source,
    platform: platforms[pair.source],
    bootstrapScript: "./fixtures/source-bootstrap.sh",
    sourcePolicy: "strict-read-only"
  });
  const targetPlan = renderProviderPlan({
    providerName,
    pairKey,
    role: "target",
    platformKey: pair.target,
    platform: platforms[pair.target],
    bootstrapScript: "./fixtures/target-bootstrap.sh",
    sourcePolicy: "target-mutable"
  });

  sourcePlan.commonBootstrapScript = "./fixtures/common-bootstrap.sh";
  targetPlan.commonBootstrapScript = "./fixtures/common-bootstrap.sh";
  sourcePlan.templatePath = "./source.lima.yaml";
  targetPlan.templatePath = "./target.lima.yaml";

  const sourceTemplate = renderLimaTemplate(sourcePlan);
  const targetTemplate = renderLimaTemplate(targetPlan);
  fs.writeFileSync(path.join(workspaceDir, "source.lima.yaml"), sourceTemplate);
  fs.writeFileSync(path.join(workspaceDir, "target.lima.yaml"), targetTemplate);

  fs.writeFileSync(path.join(workspaceDir, "source.plan.json"), `${JSON.stringify(sourcePlan, null, 2)}\n`);
  fs.writeFileSync(path.join(workspaceDir, "target.plan.json"), `${JSON.stringify(targetPlan, null, 2)}\n`);
  fs.writeFileSync(path.join(workspaceDir, "pair.json"), `${JSON.stringify({
    provider: providerName,
    sourcePolicy: "strict-read-only",
    pair
  }, null, 2)}\n`);
  fs.writeFileSync(path.join(workspaceDir, "commands.json"), `${JSON.stringify(buildCommandPlan(workspaceDir, sourcePlan, targetPlan), null, 2)}\n`);

  return {
    pair,
    workspaceDir,
    providerName,
    sourcePlan,
    targetPlan
  };
}

function copyFixture(sourcePath, destPath) {
  fs.copyFileSync(sourcePath, destPath);
  fs.chmodSync(destPath, 0o755);
}

function renderProviderPlan({ providerName, pairKey, role, platformKey, platform, bootstrapScript, sourcePolicy }) {
  const instanceName = `hostshift-${pairKey}-${role}`.replace(/[^a-zA-Z0-9-]/g, "-");
  const sshAlias = `${instanceName}-ssh`;
  const sshLocalPort = role === "source" ? "60022" : "60023";
  const template = fs.readFileSync(planTemplatePath, "utf8");
  const rendered = replaceTemplate(template, {
    INSTANCE_NAME: instanceName,
    ROLE: role,
    PLATFORM_KEY: platformKey,
    PLATFORM_FAMILY: platform.family,
    PLATFORM_RELEASE: platform.release,
    PLATFORM_IMAGE_REF: platform.providerImageRef,
    PLATFORM_TEMPLATE_URL: platform.templateUrl,
    SSH_ALIAS: sshAlias,
    SSH_LOCAL_PORT: sshLocalPort,
    REPO_ROOT: repoRoot,
    COMMON_BOOTSTRAP_SCRIPT: "./fixtures/common-bootstrap.sh",
    ROLE_BOOTSTRAP_SCRIPT: bootstrapScript,
    SOURCE_POLICY: sourcePolicy
  });
  const plan = JSON.parse(rendered);
  plan.provider = providerName;
  return plan;
}

function replaceTemplate(template, variables) {
  return Object.entries(variables).reduce((output, [key, value]) => output.replaceAll(`{{${key}}}`, String(value)), template);
}

function renderLimaTemplate(plan) {
  const repoPath = yamlQuote(repoRoot);
  const commonScript = yamlQuote(plan.commonBootstrapScript);
  const roleScript = yamlQuote(plan.fixtures.roleBootstrapScript);
  const templateUrl = yamlQuote(plan.platform.templateUrl);
  const mountPoint = yamlQuote("/mnt/hostshift");

  return [
    'minimumLimaVersion: "2.0.0"',
    `base: ${templateUrl}`,
    "mounts:",
    `  - location: ${repoPath}`,
    `    mountPoint: ${mountPoint}`,
    "    writable: true",
    "ssh:",
    `  localPort: ${plan.ssh.localPort}`,
    "provision:",
    "  - mode: system",
    "    file:",
    `      url: ${commonScript}`,
    "  - mode: system",
    "    file:",
    `      url: ${roleScript}`,
    ""
  ].join("\n");
}

function buildCommandPlan(workspaceDir, sourcePlan, targetPlan) {
  const template = (filename) => path.join(workspaceDir, filename);
  const hostShiftBin = process.env.HOSTSHIFT_VM_HOSTSHIFT_BIN || defaultHostShiftBin;
  const discoveredProfile = path.join(workspaceDir, "discovered.profile.yaml");
  const fixtureProfile = path.join(workspaceDir, "fixture.profile.json");
  const stateDir = path.join(workspaceDir, "state");

  return {
    sourcePolicy: "strict-read-only",
    commands: [
      ["limactl", "validate", template("source.lima.yaml")],
      ["limactl", "validate", template("target.lima.yaml")],
      ["limactl", "start", "--tty=false", "--name", sourcePlan.instanceName, template("source.lima.yaml")],
      ["limactl", "start", "--tty=false", "--name", targetPlan.instanceName, template("target.lima.yaml")],
      ["limactl", "show-ssh", "--format=options", sourcePlan.instanceName],
      ["limactl", "show-ssh", "--format=options", targetPlan.instanceName],
      hostShiftCommand(hostShiftBin, ["discover", "--source", sourcePlan.ssh.alias, "--name", sourcePlan.platform.key, "--profile", discoveredProfile, "--json"]),
      hostShiftCommand(hostShiftBin, ["plan", "--profile", discoveredProfile, "--target", targetPlan.ssh.alias, "--json"]),
      hostShiftCommand(hostShiftBin, ["plan", "--profile", fixtureProfile, "--target", targetPlan.ssh.alias, "--json"]),
      hostShiftCommand(hostShiftBin, ["prepare", "--profile", fixtureProfile, "--target", targetPlan.ssh.alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-prepare"]),
      hostShiftCommand(hostShiftBin, ["sync", "--profile", fixtureProfile, "--target", targetPlan.ssh.alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-sync"]),
      hostShiftCommand(hostShiftBin, ["verify", "--profile", fixtureProfile, "--target", targetPlan.ssh.alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-verify"]),
      ["limactl", "stop", targetPlan.instanceName],
      ["limactl", "start", targetPlan.instanceName],
      ["limactl", "show-ssh", "--format=options", sourcePlan.instanceName],
      ["limactl", "show-ssh", "--format=options", targetPlan.instanceName],
      hostShiftCommand(hostShiftBin, ["verify", "--profile", fixtureProfile, "--target", targetPlan.ssh.alias, "--apply", "--json", "--state-dir", stateDir, "--run-id", "vm-post-reboot-verify"]),
      ["limactl", "stop", targetPlan.instanceName],
      ["limactl", "stop", sourcePlan.instanceName],
      ["limactl", "delete", "--force", targetPlan.instanceName],
      ["limactl", "delete", "--force", sourcePlan.instanceName]
    ]
  };
}

function executePair(workspace) {
  const instances = [
    { filename: "source.lima.yaml", plan: workspace.sourcePlan },
    { filename: "target.lima.yaml", plan: workspace.targetPlan }
  ];
  const keepInstances = process.env.HOSTSHIFT_VM_KEEP_INSTANCES === "1";
  const stateDir = path.join(workspace.workspaceDir, "state");
  fs.mkdirSync(stateDir, { recursive: true });

  try {
    for (const instance of instances) {
      const templatePath = path.join(workspace.workspaceDir, instance.filename);
      run("limactl", ["validate", templatePath], { cwd: workspace.workspaceDir });
    }

    for (const instance of instances) {
      const templatePath = path.join(workspace.workspaceDir, instance.filename);
      run("limactl", ["start", "--tty=false", "--name", instance.plan.instanceName, templatePath], {
        cwd: workspace.workspaceDir
      });
    }

    const sshConfig = buildApplySshConfig(workspace);
    const sshConfigPath = path.join(workspace.workspaceDir, "ssh_config");
    fs.writeFileSync(sshConfigPath, sshConfig);

    const sourceSnapshotBefore = captureSourceSnapshot(workspace.sourcePlan.ssh.alias, sshConfigPath);
    runHostShiftWorkflow(workspace, sshConfigPath, stateDir);
    verifyTargetBootPersistence(workspace, sshConfigPath, stateDir);
    const sourceSnapshotAfter = captureSourceSnapshot(workspace.sourcePlan.ssh.alias, sshConfigPath);
    if (sourceSnapshotBefore !== sourceSnapshotAfter) {
      throw new Error(`source immutability check failed for ${workspace.pair.source} -> ${workspace.pair.target}`);
    }
  } finally {
    if (keepInstances) {
      return;
    }

    for (const instance of [...instances].reverse()) {
      runQuiet("limactl", ["stop", instance.plan.instanceName], { cwd: workspace.workspaceDir });
      runQuiet("limactl", ["delete", "--force", instance.plan.instanceName], { cwd: workspace.workspaceDir });
    }
  }
}

function buildApplySshConfig(workspace) {
  return [
    sshAliasConfig(workspace.sourcePlan.ssh.alias, readLimaSshOptions(workspace.sourcePlan.instanceName)),
    "",
    sshAliasConfig(workspace.targetPlan.ssh.alias, readLimaSshOptions(workspace.targetPlan.instanceName))
  ].join("\n");
}

function readLimaSshOptions(instanceName) {
  const result = run("limactl", ["show-ssh", "--format=options", instanceName], { capture: true });
  const options = {};

  for (const rawLine of result.stdout.split("\n")) {
    const line = rawLine.trim();
    if (!line) {
      continue;
    }
    const match = line.match(/^([A-Za-z][A-Za-z0-9]*)=(.*)$/);
    if (!match) {
      throw new Error(`unexpected limactl show-ssh output: ${line}`);
    }
    const key = match[1];
    const value = stripWrappedQuotes(match[2]);
    options[key] = value;
  }

  const required = ["Hostname", "Port", "User", "IdentityFile"];
  for (const key of required) {
    if (!options[key]) {
      throw new Error(`missing ${key} from limactl show-ssh for ${instanceName}`);
    }
  }
  return options;
}

function stripWrappedQuotes(value) {
  if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
    return value.slice(1, -1);
  }
  return value;
}

function sshAliasConfig(alias, options) {
  const lines = [
    `Host ${alias}`,
    `  HostName ${options.Hostname}`,
    `  Port ${options.Port}`,
    `  User ${options.User}`,
    `  IdentityFile ${options.IdentityFile}`,
    "  BatchMode yes",
    "  IdentitiesOnly yes",
    "  StrictHostKeyChecking no",
    "  UserKnownHostsFile /dev/null",
    "  LogLevel ERROR"
  ];

  if (options.ProxyCommand) {
    lines.push(`  ProxyCommand ${options.ProxyCommand}`);
  }
  if (options.ForwardAgent) {
    lines.push(`  ForwardAgent ${options.ForwardAgent}`);
  }

  return lines.join("\n");
}

function runHostShiftWorkflow(workspace, sshConfigPath, stateDir) {
  const hostShiftBin = process.env.HOSTSHIFT_VM_HOSTSHIFT_BIN || defaultHostShiftBin;
  const discoveredProfile = path.join(workspace.workspaceDir, "discovered.profile.yaml");
  const fixtureProfile = path.join(workspace.workspaceDir, "fixture.profile.json");
  const env = {
    ...process.env,
    HOSTSHIFT_SSH_CONFIG: sshConfigPath,
    HOSTSHIFT_TARGET_SUDO: "1"
  };

  const discover = runHostShift(hostShiftBin, [
    "discover",
    "--source",
    workspace.sourcePlan.ssh.alias,
    "--name",
    `${workspace.pair.source}-fixture`,
    "--profile",
    discoveredProfile,
    "--json"
  ], env);

  const discoverBody = JSON.parse(discover.stdout);
  if (!Array.isArray(discoverBody.requiredFailures) || discoverBody.requiredFailures.length !== 0) {
    throw new Error(`discover reported required failures for ${workspace.pair.source}: ${JSON.stringify(discoverBody.requiredFailures)}`);
  }

  const discoveredPlan = runHostShift(hostShiftBin, [
    "plan",
    "--profile",
    discoveredProfile,
    "--target",
    workspace.targetPlan.ssh.alias,
    "--json"
  ], env);
  const discoveredPlanBody = JSON.parse(discoveredPlan.stdout);
  if (discoveredPlanBody.sourceWillBeModified !== false) {
    throw new Error(`plan must keep source immutable for ${workspace.pair.source} -> ${workspace.pair.target}`);
  }

  fs.writeFileSync(fixtureProfile, `${JSON.stringify(buildFixtureProfile(workspace), null, 2)}\n`);

  const plan = runHostShift(hostShiftBin, [
    "plan",
    "--profile",
    fixtureProfile,
    "--target",
    workspace.targetPlan.ssh.alias,
    "--json"
  ], env);
  const planBody = JSON.parse(plan.stdout);
  if (planBody.sourceWillBeModified !== false) {
    throw new Error(`fixture plan must keep source immutable for ${workspace.pair.source} -> ${workspace.pair.target}`);
  }
  if (Array.isArray(planBody.blockers) && planBody.blockers.length > 0) {
    throw new Error(`fixture plan reported blockers for ${workspace.pair.source} -> ${workspace.pair.target}: ${JSON.stringify(planBody.blockers)}`);
  }

  const runPrefix = `${workspace.pair.source}-${workspace.pair.target}`;
  const prepare = runHostShift(hostShiftBin, [
    "prepare",
    "--profile",
    fixtureProfile,
    "--target",
    workspace.targetPlan.ssh.alias,
    "--apply",
    "--json",
    "--state-dir",
    stateDir,
    "--run-id",
    `${runPrefix}-prepare`
  ], env);
  assertPhaseResult(JSON.parse(prepare.stdout), "prepare", { expectStream: false });

  const sync = runHostShift(hostShiftBin, [
    "sync",
    "--profile",
    fixtureProfile,
    "--target",
    workspace.targetPlan.ssh.alias,
    "--apply",
    "--json",
    "--state-dir",
    stateDir,
    "--run-id",
    `${runPrefix}-sync`
  ], env);
  assertPhaseResult(JSON.parse(sync.stdout), "sync", { expectStream: true });

  const verify = runHostShift(hostShiftBin, [
    "verify",
    "--profile",
    fixtureProfile,
    "--target",
    workspace.targetPlan.ssh.alias,
    "--apply",
    "--json",
    "--state-dir",
    stateDir,
    "--run-id",
    `${runPrefix}-verify`
  ], env);
  assertPhaseResult(JSON.parse(verify.stdout), "verify", { expectStream: false });
}

function verifyTargetBootPersistence(workspace, sshConfigPath, stateDir) {
  const hostShiftBin = process.env.HOSTSHIFT_VM_HOSTSHIFT_BIN || defaultHostShiftBin;
  const fixtureProfile = path.join(workspace.workspaceDir, "fixture.profile.json");
  const runPrefix = `${workspace.pair.source}-${workspace.pair.target}`;

  run("limactl", ["stop", workspace.targetPlan.instanceName], { cwd: workspace.workspaceDir });
  run("limactl", ["start", workspace.targetPlan.instanceName], { cwd: workspace.workspaceDir });
  fs.writeFileSync(sshConfigPath, buildApplySshConfig(workspace));

  const env = {
    ...process.env,
    HOSTSHIFT_SSH_CONFIG: sshConfigPath,
    HOSTSHIFT_TARGET_SUDO: "1"
  };
  const verify = runHostShift(hostShiftBin, [
    "verify",
    "--profile",
    fixtureProfile,
    "--target",
    workspace.targetPlan.ssh.alias,
    "--apply",
    "--json",
    "--state-dir",
    stateDir,
    "--run-id",
    `${runPrefix}-post-reboot-verify`
  ], env);
  assertPhaseResult(JSON.parse(verify.stdout), "post-reboot verify", { expectStream: false });
}

function buildFixtureProfile(workspace) {
  return {
    schemaVersion: 2,
    name: `vm-${workspace.pair.source}-to-${workspace.pair.target}`,
    source: { ssh: workspace.sourcePlan.ssh.alias },
    target: { ssh: workspace.targetPlan.ssh.alias },
    sourcePolicy: "strict-read-only",
    platforms: {
      source: `${workspace.sourcePlan.platform.family}:${workspace.sourcePlan.platform.release}`,
      target: `${workspace.targetPlan.platform.family}:${workspace.targetPlan.platform.release}`
    },
    firewall: {
      enabled: true,
      enable: false,
      rules: [{ from: "172.17.0.0/16", port: 3306, proto: "tcp" }]
    },
    sshd: {
      settings: {
        ClientAliveInterval: 120,
        ClientAliveCountMax: 720
      }
    },
    mysql: {
      settings: {
        bindAddress: "0.0.0.0",
        mysqlxBindAddress: "127.0.0.1"
      }
    },
    workloads: [
      {
        type: "file-set",
        name: "vm-fixture-files",
        data: {
          paths: [
            "/srv/hostshift-fixture",
            "/etc/nginx/sites-available/hostshift-fixture.conf",
            "/etc/nginx/sites-enabled/hostshift-fixture.conf"
          ],
          targetPath: "/"
        }
      },
      {
        type: "mysql",
        name: "hostshiftvm"
      },
      {
        type: "postgresql",
        name: "hostshiftpg"
      }
    ],
    checks: [
      {
        type: "nginxConfig",
        name: "reload-nginx"
      },
      {
        type: "serviceActive",
        name: "ssh-service",
        data: { service: "ssh" }
      },
      {
        type: "serviceActive",
        name: "nginx-service",
        data: { service: "nginx" }
      },
      {
        type: "serviceActive",
        name: "mysql-service",
        data: { service: "mysql" }
      },
      {
        type: "serviceActive",
        name: "postgres-service",
        data: { service: "postgresql" }
      },
      {
        type: "ufwRule",
        name: "mysql-firewall-rule",
        data: { from: "172.17.0.0/16", port: 3306, proto: "tcp" }
      },
      {
        type: "nftRule",
        name: "mysql-nft-rule",
        data: { family: "inet", table: "hostshift", chain: "input", contains: "tcp dport 3306 accept" }
      },
      {
        type: "fileExists",
        name: "health-file",
        data: { path: "/srv/hostshift-fixture/public/health" }
      },
      {
        type: "fileExists",
        name: "nginx-site",
        data: { path: "/etc/nginx/sites-available/hostshift-fixture.conf" }
      },
      {
        type: "fileContains",
        name: "health-content",
        data: { path: "/srv/hostshift-fixture/public/health", contains: "ok" }
      },
      {
        type: "fileContains",
        name: "sshd-keepalive",
        data: { path: "/etc/ssh/sshd_config.d/99-hostshift.conf", contains: "ClientAliveInterval 120" }
      },
      {
        type: "fileContains",
        name: "mysql-bind",
        data: { path: "/etc/mysql/mysql.conf.d/99-hostshift-bind.cnf", contains: "bind-address = 0.0.0.0" }
      },
      {
        type: "http",
        name: "health-http",
        data: { url: "http://127.0.0.1/health", timeoutSeconds: 10 }
      },
      {
        type: "mysqlScalar",
        name: "mysql-row-count",
        data: { database: "hostshiftvm", query: "SELECT COUNT(*) FROM pages", expected: "2" }
      },
      {
        type: "mysqlScalar",
        name: "mysql-checksum",
        data: { database: "hostshiftvm", query: "SELECT MD5(GROUP_CONCAT(CONCAT(id, ':', slug, ':', body) ORDER BY id SEPARATOR ',')) FROM pages", expected: "b56d589972734ead12a0069c3ebb4178" }
      },
      {
        type: "postgresScalar",
        name: "postgres-row-count",
        data: { database: "hostshiftpg", query: "SELECT COUNT(*) FROM metrics", expected: "2" }
      },
      {
        type: "postgresScalar",
        name: "postgres-checksum",
        data: { database: "hostshiftpg", query: "SELECT md5(string_agg(id::text || ':' || name, ',' ORDER BY id)) FROM metrics", expected: "e5926976ef869d2387a6e12b8bcc0cdd" }
      }
    ],
    approved: true
  };
}

function assertPhaseResult(body, phase, { expectStream }) {
  if (body.sourceWillBeModified !== false) {
    throw new Error(`${phase} must keep source immutable`);
  }
  if (Array.isArray(body.blockers) && body.blockers.length > 0) {
    throw new Error(`${phase} reported blockers: ${JSON.stringify(body.blockers)}`);
  }
  if (!Array.isArray(body.results) || body.results.length === 0) {
    throw new Error(`${phase} returned no execution results`);
  }
  for (const result of body.results) {
    if (result.error) {
      throw new Error(`${phase} failed ${result.actionId}: ${result.error}`);
    }
    if (result.dryRun || result.skipped) {
      throw new Error(`${phase} returned dry-run or skipped result for ${result.actionId}`);
    }
  }
  if (expectStream && !body.results.some((result) => result.stream === true)) {
    throw new Error(`${phase} did not execute any stream action`);
  }
}

function captureSourceSnapshot(sourceAlias, sshConfigPath) {
  const result = run(
    "ssh",
    [
      "-F",
      sshConfigPath,
      sourceAlias,
      "sha256sum",
      "/srv/hostshift-fixture/public/health",
      "/etc/nginx/sites-available/hostshift-fixture.conf"
    ],
    { capture: true }
  );
  return result.stdout.trim();
}

function runHostShift(hostShiftBin, args, env) {
  const command = hostShiftCommand(hostShiftBin, args);
  return run(command[0], command.slice(1), {
    cwd: repoRoot,
    env,
    capture: true
  });
}

function hostShiftCommand(hostShiftBin, args) {
  const isNodeScript = hostShiftBin.endsWith(".js") || hostShiftBin.endsWith(".mjs") || hostShiftBin.endsWith(".cjs");
  if (isNodeScript) {
    return [process.execPath, hostShiftBin, ...args];
  }
  return [hostShiftBin, ...args];
}

function runProviderPreflight(providerName, provider) {
  if (process.env.HOSTSHIFT_RUN_VM_E2E !== "1") {
    return;
  }

  const required = Array.isArray(provider.requiredBinaries) ? provider.requiredBinaries : [];
  for (const bin of required) {
    if (!commandExists(bin)) {
      throw new Error(`missing required binary for provider ${providerName}: ${bin}`);
    }
  }

  if (providerName === "lima") {
    const result = run("limactl", ["--version"], { capture: true });
    const version = result.stdout.trim();
    if (version) {
      console.log(`Lima preflight: ${version}`);
    }
  }
}

function commandExists(bin) {
  const result = spawnSync("sh", ["-lc", `command -v ${shellQuote(bin)}`], {
    stdio: "ignore"
  });
  return result.status === 0;
}

function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}

function yamlQuote(value) {
  return JSON.stringify(String(value));
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env ?? process.env,
    encoding: "utf8",
    stdio: options.capture ? "pipe" : "inherit"
  });

  if (result.status !== 0) {
    const stderr = result.stderr?.trim();
    const stdout = result.stdout?.trim();
    const detail = stderr || stdout || `${command} exited with status ${result.status}`;
    throw new Error(detail);
  }

  return {
    stdout: result.stdout ?? "",
    stderr: result.stderr ?? ""
  };
}

function runQuiet(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: options.env ?? process.env,
    encoding: "utf8",
    stdio: "pipe"
  });

  return {
    status: result.status ?? 1,
    stdout: result.stdout ?? "",
    stderr: result.stderr ?? ""
  };
}
