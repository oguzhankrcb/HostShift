import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";

test("vm e2e runner lists required cross-distro pairs", () => {
  const result = spawnSync(process.execPath, ["tests/e2e/vm/run-vm-e2e.mjs", "--list"], {
    cwd: process.cwd(),
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /ubuntu22 -> ubuntu24/);
  assert.match(result.stdout, /ubuntu22 -> debian12/);
  assert.match(result.stdout, /debian12 -> ubuntu22/);
  assert.match(result.stdout, /debian12 -> debian13/);
});

test("vm e2e dry-run documents provider boot and apply behavior", () => {
  const result = spawnSync(process.execPath, ["tests/e2e/vm/run-vm-e2e.mjs"], {
    cwd: process.cwd(),
    encoding: "utf8"
  });
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /provider preflight and VM boot/i);
  assert.match(result.stdout, /Add --apply to execute the real provider workflow/i);
});

test("vm e2e runner renders Lima templates and source-safe manifests", () => {
  const emitDir = fs.mkdtempSync(path.join(os.tmpdir(), "hostshift-vm-test-"));
  const result = spawnSync(
    process.execPath,
    ["tests/e2e/vm/run-vm-e2e.mjs", "--pair", "ubuntu22->debian12", "--emit-dir", emitDir],
    {
      cwd: process.cwd(),
      encoding: "utf8"
    }
  );
  assert.equal(result.status, 0, result.stderr);

  const workspace = path.join(emitDir, "ubuntu22-to-debian12");
  const sourcePlan = JSON.parse(fs.readFileSync(path.join(workspace, "source.plan.json"), "utf8"));
  const targetPlan = JSON.parse(fs.readFileSync(path.join(workspace, "target.plan.json"), "utf8"));
  const pair = JSON.parse(fs.readFileSync(path.join(workspace, "pair.json"), "utf8"));
  const commands = JSON.parse(fs.readFileSync(path.join(workspace, "commands.json"), "utf8"));
  const sourceTemplate = fs.readFileSync(path.join(workspace, "source.lima.yaml"), "utf8");
  const targetTemplate = fs.readFileSync(path.join(workspace, "target.lima.yaml"), "utf8");
  const sourceFixture = fs.readFileSync(path.join(workspace, "fixtures/source-bootstrap.sh"), "utf8");
  const commonFixture = fs.readFileSync(path.join(workspace, "fixtures/common-bootstrap.sh"), "utf8");
  const targetFixture = fs.readFileSync(path.join(workspace, "fixtures/target-bootstrap.sh"), "utf8");
  const vmRunner = fs.readFileSync("tests/e2e/vm/run-vm-e2e.mjs", "utf8");

  assert.equal(sourcePlan.sourcePolicy, "strict-read-only");
  assert.equal(targetPlan.sourcePolicy, "target-mutable");
  assert.equal(sourcePlan.platform.key, "ubuntu22");
  assert.equal(targetPlan.platform.key, "debian12");
  assert.equal(sourcePlan.platform.templateUrl, "template:ubuntu-22.04");
  assert.equal(targetPlan.platform.templateUrl, "template:debian-12");
  assert.equal(pair.sourcePolicy, "strict-read-only");
  assert.match(sourceTemplate, /base: "template:ubuntu-22\.04"/);
  assert.match(targetTemplate, /base: "template:debian-12"/);
  assert.match(sourceTemplate, /url: "\.\/fixtures\/common-bootstrap\.sh"/);
  assert.match(sourceTemplate, /url: "\.\/fixtures\/source-bootstrap\.sh"/);
  assert.match(targetTemplate, /url: "\.\/fixtures\/target-bootstrap\.sh"/);
  assert.match(sourceFixture, /\/srv\/hostshift-fixture\/public\/health/);
  assert.match(sourceFixture, /getent passwd 501/);
  assert.match(sourceFixture, /\$3 >= 1000 && \$3 < 60000/);
  assert.match(sourceFixture, /MYSQL_SERVER_PACKAGE="default-mysql-server"/);
  assert.match(sourceFixture, /systemctl restart mysql \|\| systemctl restart mariadb/);
  assert.match(commonFixture, /nftables/);
  assert.match(targetFixture, /nft add table inet hostshift/);
  assert.match(targetFixture, /nft list ruleset > \/etc\/nftables\.conf/);
  assert.equal(commands.sourcePolicy, "strict-read-only");
  assert.deepEqual(commands.commands[0].slice(0, 2), ["limactl", "validate"]);
  assert.deepEqual(commands.commands[2].slice(0, 4), ["limactl", "start", "--tty=false", "--name"]);
  assert(commands.commands[8].includes("plan"));
  assert(commands.commands[11].includes("verify"));
  assert.deepEqual(commands.commands[12], ["limactl", "stop", "hostshift-ubuntu22-to-debian12-target"]);
  assert.deepEqual(commands.commands[13], ["limactl", "start", "hostshift-ubuntu22-to-debian12-target"]);
  assert(commands.commands[16].includes("verify"));
  assert.match(vmRunner, /result\.error/);
});

test("vm e2e apply path executes limactl lifecycle, source snapshot, and hostshift phases", () => {
  const tempDir = fs.mkdtempSync(path.join(os.tmpdir(), "hostshift-vm-apply-"));
  const binDir = path.join(tempDir, "bin");
  fs.mkdirSync(binDir, { recursive: true });
  const logPath = path.join(tempDir, "commands.log");
  const fakeIdentity = path.join(tempDir, "id_ed25519");
  fs.writeFileSync(fakeIdentity, "not-a-real-key\n");

  const limactlPath = path.join(binDir, "limactl");
  fs.writeFileSync(
    limactlPath,
    `#!/usr/bin/env node
import fs from "node:fs";
import path from "node:path";

const args = process.argv.slice(2);
const logPath = process.env.HOSTSHIFT_VM_TEST_LOG;
if (logPath) {
  fs.appendFileSync(logPath, \`limactl \${args.join(" ")}\\n\`);
}

const command = args[0];
if (command === "--version") {
  console.log("limactl version 1.0.0-test");
  process.exit(0);
}
if (command === "validate" || command === "start" || command === "stop" || command === "delete") {
  process.exit(0);
}
if (command === "show-ssh") {
  const name = args.at(-1);
  const port = name.includes("source") ? "60022" : "60023";
  console.log('Hostname="127.0.0.1"');
  console.log(\`Port="\${port}"\`);
  console.log('User="root"');
  console.log(\`IdentityFile="\${process.env.HOSTSHIFT_VM_TEST_IDENTITY}"\`);
  process.exit(0);
}

console.error(\`unexpected limactl invocation: \${args.join(" ")}\`);
process.exit(1);
`,
    { mode: 0o755 }
  );

  const sshPath = path.join(binDir, "ssh");
  fs.writeFileSync(
    sshPath,
    `#!/usr/bin/env node
import fs from "node:fs";

const args = process.argv.slice(2);
const logPath = process.env.HOSTSHIFT_VM_TEST_LOG;
if (logPath) {
  fs.appendFileSync(logPath, \`ssh \${args.join(" ")}\\n\`);
}

const commandArgs = args[0] === "-F" ? args.slice(3) : args.slice(1);
if (commandArgs[0] === "sha256sum") {
  process.stdout.write("abc123  /srv/hostshift-fixture/public/health\\ndef456  /etc/nginx/sites-available/hostshift-fixture.conf\\n");
  process.exit(0);
}

console.error(\`unexpected ssh invocation: \${args.join(" ")}\`);
process.exit(1);
`,
    { mode: 0o755 }
  );

  const hostshiftPath = path.join(binDir, "hostshift-stub.mjs");
  fs.writeFileSync(
    hostshiftPath,
    `#!/usr/bin/env node
import fs from "node:fs";

const args = process.argv.slice(2);
const logPath = process.env.HOSTSHIFT_VM_TEST_LOG;
if (logPath) {
  fs.appendFileSync(logPath, \`hostshift \${args.join(" ")}\\n\`);
}

const command = args[0];
if (command === "discover") {
  const profileIndex = args.indexOf("--profile");
  const profilePath = args[profileIndex + 1];
  fs.writeFileSync(profilePath, "sourcePolicy: strict-read-only\\n");
  process.stdout.write(JSON.stringify({
    sourceWillBeModified: false,
    requiredFailures: [],
    facts: { osRelease: { ok: true } }
  }));
  process.exit(0);
}
if (command === "plan") {
  process.stdout.write(JSON.stringify({
    sourceWillBeModified: false,
    blockers: [],
    actions: []
  }));
  process.exit(0);
}
if (command === "prepare" || command === "verify") {
  process.stdout.write(JSON.stringify({
    sourceWillBeModified: false,
    blockers: [],
    results: [{ actionId: \`\${command}-action\`, dryRun: false, skipped: false }]
  }));
  process.exit(0);
}
if (command === "sync") {
  process.stdout.write(JSON.stringify({
    sourceWillBeModified: false,
    blockers: [],
    results: [{ actionId: "sync-stream", dryRun: false, skipped: false, stream: true }]
  }));
  process.exit(0);
}

console.error(\`unexpected hostshift invocation: \${args.join(" ")}\`);
process.exit(1);
`,
    { mode: 0o755 }
  );

  const emitDir = path.join(tempDir, "emit");
  const result = spawnSync(
    process.execPath,
    ["tests/e2e/vm/run-vm-e2e.mjs", "--pair", "ubuntu22->debian12", "--emit-dir", emitDir, "--apply"],
    {
      cwd: process.cwd(),
      encoding: "utf8",
      env: {
        ...process.env,
        PATH: `${binDir}${path.delimiter}${process.env.PATH}`,
        HOSTSHIFT_RUN_VM_E2E: "1",
        HOSTSHIFT_VM_HOSTSHIFT_BIN: hostshiftPath,
        HOSTSHIFT_VM_TEST_LOG: logPath,
        HOSTSHIFT_VM_TEST_IDENTITY: fakeIdentity
      }
    }
  );
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /Lima preflight: limactl version 1\.0\.0-test/);
  assert.match(result.stdout, /VM apply executor completed successfully/);

  const workspace = path.join(emitDir, "ubuntu22-to-debian12");
  const sshConfig = fs.readFileSync(path.join(workspace, "ssh_config"), "utf8");
  const discoveredProfile = fs.readFileSync(path.join(workspace, "discovered.profile.yaml"), "utf8");
  const fixtureProfile = JSON.parse(fs.readFileSync(path.join(workspace, "fixture.profile.json"), "utf8"));
  const log = fs.readFileSync(logPath, "utf8");

  assert.match(sshConfig, /Host hostshift-ubuntu22-to-debian12-source-ssh/);
  assert.match(sshConfig, /IdentityFile .*id_ed25519/);
  assert.match(discoveredProfile, /sourcePolicy: strict-read-only/);
  assert.equal(fixtureProfile.sourcePolicy, "strict-read-only");
  assert.equal(fixtureProfile.workloads[0].type, "file-set");
  assert.equal(fixtureProfile.workloads[1].type, "mysql");
  assert.equal(fixtureProfile.workloads[2].type, "postgresql");
  assert.equal(fixtureProfile.checks[0].type, "nginxConfig");
  assert(fixtureProfile.checks.some((check) => check.type === "http"));
  assert(fixtureProfile.checks.some((check) => check.type === "mysqlScalar" && check.name === "mysql-checksum"));
  assert(fixtureProfile.checks.some((check) => check.type === "postgresScalar" && check.name === "postgres-checksum"));
  assert(fixtureProfile.checks.some((check) => check.type === "serviceActive" && check.name === "nginx-service"));
  assert(fixtureProfile.checks.some((check) => check.type === "ufwRule" && check.name === "mysql-firewall-rule"));
  assert(fixtureProfile.checks.some((check) => check.type === "nftRule" && check.name === "mysql-nft-rule"));
  assert.match(log, /limactl validate .*source\.lima\.yaml/);
  assert.match(log, /limactl start --tty=false --name hostshift-ubuntu22-to-debian12-source/);
  assert.match(log, /limactl show-ssh --format=options hostshift-ubuntu22-to-debian12-target/);
  assert.match(log, /ssh -F .* hostshift-ubuntu22-to-debian12-source-ssh sha256sum \/srv\/hostshift-fixture\/public\/health/);
  assert.match(log, /hostshift discover --source hostshift-ubuntu22-to-debian12-source-ssh/);
  assert.match(log, /hostshift plan --profile .*discovered\.profile\.yaml --target hostshift-ubuntu22-to-debian12-target-ssh --json/);
  assert.match(log, /hostshift plan --profile .*fixture\.profile\.json --target hostshift-ubuntu22-to-debian12-target-ssh --json/);
  assert.match(log, /hostshift prepare --profile .*fixture\.profile\.json --target hostshift-ubuntu22-to-debian12-target-ssh --apply --json --state-dir .* --run-id ubuntu22-debian12-prepare/);
  assert.match(log, /hostshift sync --profile .*fixture\.profile\.json --target hostshift-ubuntu22-to-debian12-target-ssh --apply --json --state-dir .* --run-id ubuntu22-debian12-sync/);
  assert.match(log, /hostshift verify --profile .*fixture\.profile\.json --target hostshift-ubuntu22-to-debian12-target-ssh --apply --json --state-dir .* --run-id ubuntu22-debian12-verify/);
  assert.match(log, /limactl stop hostshift-ubuntu22-to-debian12-target/);
  assert.match(log, /limactl start hostshift-ubuntu22-to-debian12-target/);
  assert.match(log, /hostshift verify --profile .*fixture\.profile\.json --target hostshift-ubuntu22-to-debian12-target-ssh --apply --json --state-dir .* --run-id ubuntu22-debian12-post-reboot-verify/);
  assert.match(log, /limactl delete --force hostshift-ubuntu22-to-debian12-source/);
});
