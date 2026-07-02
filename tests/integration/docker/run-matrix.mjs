#!/usr/bin/env node
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";

const matrix = {
  ubuntu22: ["ubuntu22", "ubuntu24", "ubuntu25", "debian12"],
  debian12: ["ubuntu22", "ubuntu24", "ubuntu25", "debian12", "debian13"]
};

const images = {
  ubuntu22: "ubuntu:22.04",
  ubuntu24: "ubuntu:24.04",
  ubuntu25: "ubuntu:25.10",
  debian12: "debian:12",
  debian13: "debian:13"
};

const platforms = {
  ubuntu22: "ubuntu:22.04",
  ubuntu24: "ubuntu:24.04",
  ubuntu25: "ubuntu:25.10",
  debian12: "debian:12",
  debian13: "debian:13"
};

const pairs = Object.entries(matrix).flatMap(([source, targets]) =>
  targets.map((target) => ({ source, target, sourceImage: images[source], targetImage: images[target] }))
);

const composeDir = fileURLToPath(new URL(".", import.meta.url));
const repoRoot = fileURLToPath(new URL("../../../", import.meta.url));
const hostshiftNodeCli = fileURLToPath(new URL("../../../bin/hostshift.js", import.meta.url));
const hostshiftGoCli = fileURLToPath(new URL("../../../dist/hostshift", import.meta.url));
const hostshiftCli = fs.existsSync(hostshiftGoCli) ? hostshiftGoCli : hostshiftNodeCli;

const pairFilter = readPairFilter(process.argv.slice(2));
const selectedPairs = pairFilter ? pairs.filter((pair) => `${pair.source}->${pair.target}` === pairFilter) : pairs;
if (pairFilter && selectedPairs.length === 0) {
  console.error(`unknown matrix pair: ${pairFilter}`);
  process.exit(2);
}

if (process.argv.includes("--list")) {
  for (const pair of selectedPairs) {
    console.log(`${pair.source} -> ${pair.target}`);
  }
  process.exit(0);
}

if (process.argv.includes("--list-images")) {
  for (const image of uniqueBaseImages(selectedPairs)) {
    console.log(image);
  }
  process.exit(0);
}

const runRealMatrix = process.env.HOSTSHIFT_RUN_DOCKER_MATRIX === "1";
const commandTimeoutMs = readTimeoutMs("HOSTSHIFT_DOCKER_COMMAND_TIMEOUT_MS", 10 * 60 * 1000);
const buildTimeoutMs = readTimeoutMs("HOSTSHIFT_DOCKER_BUILD_TIMEOUT_MS", 20 * 60 * 1000);
const pullTimeoutMs = readTimeoutMs("HOSTSHIFT_DOCKER_PULL_TIMEOUT_MS", buildTimeoutMs);
console.log(`HostShift Docker migration matrix: ${selectedPairs.length} pairs`);
console.log(`HostShift CLI: ${hostshiftCli}`);

const compose = spawnSync("docker", ["compose", "version"], { stdio: "inherit" });
if (compose.status !== 0) {
  process.exit(compose.status ?? 1);
}
if (process.argv.includes("--pull-images")) {
  ensureDockerDaemon();
  prePullBaseImages(selectedPairs);
  process.exit(0);
}
if (runRealMatrix) {
  ensureDockerDaemon();
  if (process.env.HOSTSHIFT_DOCKER_SKIP_PREPULL !== "1") {
    prePullBaseImages(selectedPairs);
  }
}

try {
  for (const pair of selectedPairs) {
    console.log(`${pair.source} -> ${pair.target}`);
    if (!runRealMatrix) {
      continue;
    }
    runPair(pair);
  }

  if (!runRealMatrix) {
    console.log("Dry-run only. Set HOSTSHIFT_RUN_DOCKER_MATRIX=1 to execute Docker compose config/build and source immutability checks for each pair.");
  }
} catch (error) {
  if (error instanceof Error) {
    console.error(error.message);
  } else {
    console.error(String(error));
  }
  process.exit(1);
}

function readPairFilter(args) {
  const index = args.indexOf("--pair");
  if (index === -1) {
    return "";
  }
  return args[index + 1] ?? "";
}

function ensureDockerDaemon() {
  const result = spawnSync("docker", ["info"], { stdio: "pipe", encoding: "utf8" });
  if (result.status === 0) {
    return;
  }
  process.stderr.write("Docker daemon is required for HOSTSHIFT_RUN_DOCKER_MATRIX=1.\n");
  if (result.stderr) {
    process.stderr.write(result.stderr);
  }
  throw new Error("docker daemon unavailable");
}

function uniqueBaseImages(selected) {
  return [...new Set(selected.flatMap((pair) => [pair.sourceImage, pair.targetImage]))].sort();
}

function prePullBaseImages(selected) {
  for (const image of uniqueBaseImages(selected)) {
    if (dockerImageExists(image)) {
      console.log(`[docker] base image cached: ${image}`);
      continue;
    }
    console.log(`[docker] pulling base image: ${image}`);
    run("docker", ["pull", image], { timeout: pullTimeoutMs });
  }
}

function dockerImageExists(image) {
  const result = spawnSync("docker", ["image", "inspect", image], {
    stdio: "ignore",
    timeout: commandTimeoutMs
  });
  return result.status === 0;
}

function runPair(pair) {
  logStage(pair, "starting");
  const project = `hostshift-${pair.source}-${pair.target}`.replace(/[^a-zA-Z0-9_-]/g, "-");
  const sshHome = fs.mkdtempSync(path.join(os.tmpdir(), `${project}-ssh-`));
  const keyPath = path.join(sshHome, "id_ed25519");
  generateKeypair(keyPath);
  const env = {
    ...process.env,
    SOURCE_IMAGE: pair.sourceImage,
    TARGET_IMAGE: pair.targetImage,
    SSH_PUBLIC_KEY: fs.readFileSync(`${keyPath}.pub`, "utf8").trim()
  };
  logStage(pair, "rendering compose config");
  run("docker", ["compose", "-p", project, "-f", "compose.yaml", "config"], { cwd: composeDir, env });
  logStage(pair, "building fixture images");
  run("docker", ["compose", "-p", project, "-f", "compose.yaml", "build", "--no-cache"], { cwd: composeDir, env, timeout: buildTimeoutMs });
  try {
    logStage(pair, "booting fixtures");
    run("docker", ["compose", "-p", project, "-f", "compose.yaml", "up", "-d"], { cwd: composeDir, env });
    logStage(pair, "verifying source fixture baseline");
    verifySourceFixture(project, env);
    const sshConfig = writeSshConfig(project, sshHome, keyPath, {
      source: lookupPort(project, "source"),
      target: lookupPort(project, "target")
    });
    logStage(pair, "checking SSH connectivity");
    verifySshConnectivity(sshConfig);
    logStage(pair, "running discover");
    runHostShiftDiscover(pair, sshConfig);
    logStage(pair, "running dry-run plan/prepare/sync/verify");
    runHostShiftDryRuns(pair, sshConfig);
    logStage(pair, "running sync --apply smoke");
    runHostShiftSyncApplySmoke(pair, sshConfig);
    logStage(pair, "running verify --apply smoke");
    runHostShiftVerifyApplySmoke(pair, sshConfig);
    logStage(pair, "completed successfully");
  } finally {
    logStage(pair, "cleaning up fixtures");
    run("docker", ["compose", "-p", project, "-f", "compose.yaml", "down", "--volumes", "--remove-orphans"], { cwd: composeDir, env });
  }
}

function generateKeypair(keyPath) {
  run("ssh-keygen", ["-q", "-t", "ed25519", "-N", "", "-f", keyPath], { stdio: "inherit" });
}

function writeSshConfig(project, sshHome, keyPath, ports) {
  const aliases = {
    source: `${project}-source`,
    target: `${project}-target`
  };
  const sshDir = path.join(sshHome, ".ssh");
  fs.mkdirSync(sshDir, { recursive: true });
  const configPath = path.join(sshDir, "config");
  const config = [
    sshHostConfig(aliases.source, ports.source, keyPath),
    sshHostConfig(aliases.target, ports.target, keyPath)
  ].join("\n\n") + "\n";
  fs.writeFileSync(configPath, config, { mode: 0o600 });
  return { aliases, configPath, sshHome };
}

function sshHostConfig(alias, port, keyPath) {
  return [
    `Host ${alias}`,
    "  HostName 127.0.0.1",
    `  Port ${port}`,
    "  User root",
    `  IdentityFile ${keyPath}`,
    "  BatchMode yes",
    "  IdentitiesOnly yes",
    "  StrictHostKeyChecking no",
    "  UserKnownHostsFile /dev/null",
    "  LogLevel ERROR"
  ].join("\n");
}

function lookupPort(project, service) {
  const result = run("docker", ["compose", "-p", project, "-f", "compose.yaml", "port", service, "2222"], {
    cwd: composeDir,
    capture: true
  });
  const value = result.stdout.trim();
  const parts = value.split(":");
  const port = parts.at(-1);
  if (!port || !/^\d+$/.test(port)) {
    throw new Error(`could not parse docker compose port for ${service}: ${value}`);
  }
  return port;
}

function verifySshConnectivity(sshConfig) {
  const env = sshEnv(sshConfig);
  waitForSSH(sshConfig, sshConfig.aliases.source, env);
  waitForSSH(sshConfig, sshConfig.aliases.target, env);
}

function verifySourceFixture(project, env) {
  run("docker", ["compose", "-p", project, "-f", "compose.yaml", "exec", "-T", "source", "sha256sum", "-c", "/fixture/hostshift/source.sha256"], { cwd: composeDir, env });
}

function runHostShiftDiscover(pair, sshConfig) {
  const env = sshEnv(sshConfig);
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `hostshift-discover-${pair.source}-${pair.target}-`));
  const profilePath = path.join(tempDir, "discovered.yaml");
  const result = runHostShift(["discover", "--source", sshConfig.aliases.source, "--name", `${pair.source}-fixture`, "--profile", profilePath, "--json"], {
    cwd: repoRoot,
    env,
    capture: true
  });
  const body = JSON.parse(result.stdout);
  if (!Array.isArray(body.requiredFailures) || body.requiredFailures.length !== 0) {
    throw new Error(`discover reported required failures for ${pair.source}: ${JSON.stringify(body.requiredFailures)}`);
  }
  if (!body.facts?.osRelease?.ok) {
    throw new Error(`discover did not read osRelease for ${pair.source}`);
  }
  const discovered = fs.readFileSync(profilePath, "utf8");
  if (!discovered.includes("sourcePolicy: strict-read-only")) {
    throw new Error(`discover did not write the expected strict source policy profile for ${pair.source}`);
  }
  if (!discovered.includes(platforms[pair.source])) {
    throw new Error(`discover did not capture expected source platform ${platforms[pair.source]}`);
  }
}

function runHostShiftDryRuns(pair, sshConfig) {
  const env = sshEnv(sshConfig);
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `hostshift-plan-${pair.source}-${pair.target}-`));
  const profilePath = path.join(tempDir, "matrix-profile.yaml");
  fs.writeFileSync(profilePath, JSON.stringify(buildMatrixProfile(pair, sshConfig.aliases), null, 2));
  const plan = runJSON(["plan", "--profile", profilePath, "--json"], env);
  if (plan.sourceWillBeModified !== false) {
    throw new Error(`plan must keep source immutable for ${pair.source} -> ${pair.target}`);
  }
  assertExpectedBlockers(pair, plan.blockers, "plan");
  for (const phase of ["prepare", "sync", "verify"]) {
    const body = runJSON([phase, "--profile", profilePath, "--json", "--state-dir", tempDir, "--run-id", `${pair.source}-${pair.target}-${phase}`], env);
    if (body.sourceWillBeModified !== false) {
      throw new Error(`${phase} must keep source immutable for ${pair.source} -> ${pair.target}`);
    }
    assertExpectedBlockers(pair, body.blockers, phase);
  }
}

function runHostShiftSyncApplySmoke(pair, sshConfig) {
  const env = sshEnv(sshConfig);
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `hostshift-apply-${pair.source}-${pair.target}-`));
  const profilePath = path.join(tempDir, "matrix-apply-profile.yaml");
  fs.writeFileSync(profilePath, JSON.stringify(buildApplySmokeProfile(pair, sshConfig.aliases), null, 2));
  const body = runJSON(["sync", "--profile", profilePath, "--apply", "--json", "--state-dir", tempDir, "--run-id", `${pair.source}-${pair.target}-sync-apply`], env);
  if (body.sourceWillBeModified !== false) {
    throw new Error(`sync apply must keep source immutable for ${pair.source} -> ${pair.target}`);
  }
  if (!Array.isArray(body.results) || body.results.length === 0 || body.results.some((result) => result.stream !== true)) {
    throw new Error(`sync apply did not execute the expected stream actions for ${pair.source} -> ${pair.target}`);
  }
  verifyApplyArtifacts(sshConfig);
  verifyMySQLReplication(sshConfig);
  verifyPostgreSQLReplication(sshConfig);
}

function runHostShiftVerifyApplySmoke(pair, sshConfig) {
  const env = sshEnv(sshConfig);
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), `hostshift-verify-apply-${pair.source}-${pair.target}-`));
  const profilePath = path.join(tempDir, "matrix-verify-profile.yaml");
  fs.writeFileSync(profilePath, JSON.stringify(buildVerifySmokeProfile(pair, sshConfig.aliases), null, 2));
  const body = runJSON(["verify", "--profile", profilePath, "--apply", "--json", "--state-dir", tempDir, "--run-id", `${pair.source}-${pair.target}-verify-apply`], env);
  if (body.sourceWillBeModified !== false) {
    throw new Error(`verify apply must keep source immutable for ${pair.source} -> ${pair.target}`);
  }
  if (!Array.isArray(body.results) || body.results.length === 0 || body.results.some((result) => result.dryRun || result.skipped)) {
    throw new Error(`verify apply did not execute expected target checks for ${pair.source} -> ${pair.target}`);
  }
  assertResultAction(body.results, "target.check.http.fixture-health");
  assertResultAction(body.results, "target.check.laravelDatabase.fixture-db");
  verifyTargetHTTP(sshConfig);
}

function runJSON(args, env) {
  const result = runHostShift(args, {
    cwd: repoRoot,
    env,
    capture: true
  });
  return JSON.parse(result.stdout);
}

function runHostShift(args, options = {}) {
  if (hostshiftCli === hostshiftGoCli) {
    return run(hostshiftCli, args, options);
  }
  return run(process.execPath, [hostshiftNodeCli, ...args], options);
}

function verifyApplyArtifacts(sshConfig) {
  const env = sshEnv(sshConfig);
  const { source, target } = sshConfig.aliases;
  verifyRemoteFile(sshConfig, source, "/fixture/hostshift/source.sha256", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/.env", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/artisan", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/docker-compose.yml", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/fixtures/mysql/fixturedb.sql", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/fixtures/postgresql/fixturedb.sql", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/config/standalone.json", env);
  verifyRemoteFile(sshConfig, target, "/srv/app/public/index.html", env);
  verifyRemoteFile(sshConfig, target, "/etc/nginx/sites-available/example.conf", env);
  compareRemoteSha(sshConfig, source, target, "/srv/app/.env", env);
  compareRemoteSha(sshConfig, source, target, "/srv/app/artisan", env);
  compareRemoteSha(sshConfig, source, target, "/srv/app/docker-compose.yml", env);
  compareRemoteSha(sshConfig, source, target, "/srv/app/fixtures/mysql/fixturedb.sql", env);
  compareRemoteSha(sshConfig, source, target, "/etc/nginx/sites-available/example.conf", env);
  run("ssh", ["-F", sshConfig.configPath, source, "sha256sum", "-c", "/fixture/hostshift/source.sha256"], { env });
}

function verifyMySQLReplication(sshConfig) {
  const sourceRows = remoteMySQLScalar(sshConfig, sshConfig.aliases.source, "fixturedb", "SELECT COUNT(*) FROM pages");
  const targetRows = remoteMySQLScalar(sshConfig, sshConfig.aliases.target, "fixturedb", "SELECT COUNT(*) FROM pages");
  const sourceChecksum = remoteMySQLScalar(sshConfig, sshConfig.aliases.source, "fixturedb", "CHECKSUM TABLE pages");
  const targetChecksum = remoteMySQLScalar(sshConfig, sshConfig.aliases.target, "fixturedb", "CHECKSUM TABLE pages");
  if (sourceRows !== "2") {
    throw new Error(`unexpected source mysql row count: ${sourceRows}`);
  }
  if (targetRows !== sourceRows) {
    throw new Error(`mysql row count mismatch: ${sourceRows} != ${targetRows}`);
  }
  if (sourceChecksum !== targetChecksum) {
    throw new Error(`mysql checksum mismatch: ${sourceChecksum} != ${targetChecksum}`);
  }
}

function verifyTargetHTTP(sshConfig) {
  const env = sshEnv(sshConfig);
  const result = run("ssh", ["-F", sshConfig.configPath, sshConfig.aliases.target, "curl", "--fail", "--silent", "http://127.0.0.1/health"], { env, capture: true });
  if (result.stdout.trim() !== "ok") {
    throw new Error(`unexpected health response: ${result.stdout.trim()}`);
  }
}

function verifyPostgreSQLReplication(sshConfig) {
  const sourceRows = remotePostgresScalar(sshConfig, sshConfig.aliases.source, "fixturepg", "SELECT COUNT(*) FROM metrics");
  const targetRows = remotePostgresScalar(sshConfig, sshConfig.aliases.target, "fixturepg", "SELECT COUNT(*) FROM metrics");
  const sourceChecksum = remotePostgresScalar(
    sshConfig,
    sshConfig.aliases.source,
    "fixturepg",
    "SELECT md5(string_agg(id::text || ':' || name, ',' ORDER BY id)) FROM metrics"
  );
  const targetChecksum = remotePostgresScalar(
    sshConfig,
    sshConfig.aliases.target,
    "fixturepg",
    "SELECT md5(string_agg(id::text || ':' || name, ',' ORDER BY id)) FROM metrics"
  );
  if (sourceRows !== "2") {
    throw new Error(`unexpected source postgresql row count: ${sourceRows}`);
  }
  if (targetRows !== sourceRows) {
    throw new Error(`postgresql row count mismatch: ${sourceRows} != ${targetRows}`);
  }
  if (sourceChecksum !== targetChecksum) {
    throw new Error(`postgresql checksum mismatch: ${sourceChecksum} != ${targetChecksum}`);
  }
}

function assertExpectedBlockers(pair, blockers, stage) {
  if (!Array.isArray(blockers) || blockers.length !== 0) {
    throw new Error(`${stage} unexpectedly blocked for ${pair.source} -> ${pair.target}: ${JSON.stringify(blockers)}`);
  }
}

function assertResultAction(results, actionID) {
  if (!results.some((result) => result.actionId === actionID && !result.error)) {
    throw new Error(`expected result action ${actionID}`);
  }
}

function verifyRemoteFile(sshConfig, alias, remotePath, env) {
  run("ssh", ["-F", sshConfig.configPath, alias, "test", "-f", remotePath], { env });
}

function compareRemoteSha(sshConfig, sourceAlias, targetAlias, remotePath, env) {
  const source = remoteSha(sshConfig, sourceAlias, remotePath, env);
  const target = remoteSha(sshConfig, targetAlias, remotePath, env);
  if (source !== target) {
    throw new Error(`checksum mismatch for ${remotePath}: ${source} != ${target}`);
  }
}

function logStage(pair, stage) {
  console.log(`[${pair.source}->${pair.target}] ${stage}`);
}

function remoteSha(sshConfig, alias, remotePath, env) {
  const result = run("ssh", ["-F", sshConfig.configPath, alias, "sha256sum", remotePath], { env, capture: true });
  return result.stdout.trim().split(/\s+/)[0];
}

function remoteMySQLScalar(sshConfig, alias, database, query) {
  const env = sshEnv(sshConfig);
  const script = `mysql --batch --skip-column-names ${shellQuote(database)} --execute=${shellQuote(query)}`;
  const result = run("ssh", ["-F", sshConfig.configPath, alias, `sh -lc ${shellQuote(script)}`], { env, capture: true });
  const lines = result.stdout.trim().split(/\r?\n/).filter(Boolean);
  const tail = lines.at(-1) ?? "";
  return tail.trim().split(/\s+/).at(-1) ?? "";
}

function remotePostgresScalar(sshConfig, alias, database, query) {
  const env = sshEnv(sshConfig);
  const script = `psql --username root --dbname ${shellQuote(database)} --tuples-only --no-align --command ${shellQuote(query)}`;
  const result = run("ssh", ["-F", sshConfig.configPath, alias, `sh -lc ${shellQuote(script)}`], { env, capture: true });
  return result.stdout.trim();
}

function shellQuote(value) {
  return `'${String(value).replace(/'/g, `'\\''`)}'`;
}

function waitForSSH(sshConfig, alias, env) {
  let lastError = "";
  for (let attempt = 1; attempt <= 60; attempt += 1) {
    const result = spawnSync("ssh", ["-F", sshConfig.configPath, alias, "true"], {
      stdio: "pipe",
      encoding: "utf8",
      env
    });
    if (result.status === 0) {
      return;
    }
    lastError = (result.stderr || result.stdout || "").trim();
    sleep(1000);
  }
  throw new Error(`ssh did not become ready for ${alias}: ${lastError}`);
}

function sleep(ms) {
  const end = Date.now() + ms;
  while (Date.now() < end) {
    // busy wait is acceptable here because the matrix runner is a short-lived test process.
  }
}

function sshEnv(sshConfig) {
  return {
    ...process.env,
    HOME: sshConfig.sshHome,
    HOSTSHIFT_SSH_CONFIG: sshConfig.configPath
  };
}

function buildMatrixProfile(pair, aliases) {
  return {
    schemaVersion: 2,
    name: `matrix-${pair.source}-to-${pair.target}`,
    source: { ssh: aliases.source },
    target: { ssh: aliases.target },
    sourcePolicy: "strict-read-only",
    platforms: {
      source: platforms[pair.source],
      target: platforms[pair.target]
    },
    firewall: {
      enabled: true,
      enable: true,
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
        type: "docker-compose",
        name: "fixture-compose",
        data: {
          workingDir: "/srv/app",
          configFile: "/srv/app/docker-compose.yml"
        }
      },
      {
        type: "file-set",
        name: "fixture-files",
        data: {
          paths: ["/srv/app", "/etc/nginx/sites-available"],
          targetPath: "/"
        }
      },
      {
        type: "docker-standalone",
        name: "fixture-standalone",
        data: {
          image: "fixture/standalone:latest"
        }
      },
      {
        type: "mysql",
        name: "fixturedb"
      },
      {
        type: "postgresql",
        name: "fixturepg"
      }
    ],
    checks: [
      {
        type: "http",
        name: "fixture-health",
        data: {
          url: "http://127.0.0.1/health",
          timeoutSeconds: 5
        }
      },
      {
        type: "laravelDatabase",
        name: "fixture-db",
        data: {
          container: "fixture-app"
        }
      }
    ],
    approved: true
  };
}

function buildApplySmokeProfile(pair, aliases) {
  return {
    schemaVersion: 2,
    name: `matrix-apply-${pair.source}-to-${pair.target}`,
    source: { ssh: aliases.source },
    target: { ssh: aliases.target },
    sourcePolicy: "strict-read-only",
    platforms: {
      source: platforms[pair.source],
      target: platforms[pair.target]
    },
    workloads: [
      {
        type: "file-set",
        name: "fixture-files",
        data: {
          paths: ["/srv/app", "/etc/nginx/sites-available"],
          targetPath: "/"
        }
      },
      {
        type: "mysql",
        name: "fixturedb"
      },
      {
        type: "postgresql",
        name: "fixturepg"
      }
    ],
    approved: true
  };
}

function buildVerifySmokeProfile(pair, aliases) {
  return {
    schemaVersion: 2,
    name: `matrix-verify-${pair.source}-to-${pair.target}`,
    source: { ssh: aliases.source },
    target: { ssh: aliases.target },
    sourcePolicy: "strict-read-only",
    platforms: {
      source: platforms[pair.source],
      target: platforms[pair.target]
    },
    checks: [
      {
        type: "http",
        name: "fixture-health",
        data: {
          url: "http://127.0.0.1/health",
          timeoutSeconds: 5
        }
      },
      {
        type: "laravelDatabase",
        name: "fixture-db",
        data: {
          container: "fixture-app"
        }
      }
    ],
    approved: true
  };
}

function run(command, args, options = {}) {
  const { capture = false, timeout = commandTimeoutMs, ...rest } = options;
  const result = spawnSync(command, args, {
    stdio: capture ? "pipe" : "inherit",
    encoding: capture ? "utf8" : undefined,
    timeout,
    ...rest
  });
  if (result.error) {
    if (result.error.code === "ETIMEDOUT") {
      throw new Error(`command timed out after ${timeout}ms: ${command} ${args.join(" ")}`);
    }
    throw result.error;
  }
  if (result.status !== 0) {
    if (capture && result.stderr) {
      process.stderr.write(result.stderr);
    }
    throw new Error(`command failed: ${command} ${args.join(" ")}`);
  }
  return result;
}

function readTimeoutMs(name, fallback) {
  const raw = process.env[name];
  if (!raw) {
    return fallback;
  }
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value) || value <= 0) {
    throw new Error(`${name} must be a positive integer in milliseconds`);
  }
  return value;
}
