import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";

test("plugin manifest has required local plugin fields", async () => {
  const manifest = JSON.parse(await fs.readFile(".codex-plugin/plugin.json", "utf8"));
  assert.equal(manifest.name, "hostshift");
  assert.equal(manifest.skills, "./skills/");
  assert.equal(manifest.interface.displayName, "HostShift");
  assert(Array.isArray(manifest.interface.defaultPrompt));
});

test("repo marketplace exposes the packaged HostShift plugin", async () => {
  const marketplace = JSON.parse(await fs.readFile(".agents/plugins/marketplace.json", "utf8"));
  assert.equal(marketplace.name, "hostshift");
  assert.equal(marketplace.interface.displayName, "HostShift");
  assert.equal(marketplace.plugins.length, 1);

  const [entry] = marketplace.plugins;
  assert.equal(entry.name, "hostshift");
  assert.deepEqual(entry.source, {
    source: "local",
    path: "./plugins/hostshift"
  });
  assert.equal(entry.policy.installation, "AVAILABLE");
  assert.equal(entry.policy.authentication, "ON_INSTALL");
  assert.equal(entry.category, "Developer Tools");
});

test("packaged Codex plugin includes manifest, skill, and safety model", async () => {
  const manifest = JSON.parse(await fs.readFile("plugins/hostshift/.codex-plugin/plugin.json", "utf8"));
  const skill = await fs.readFile("plugins/hostshift/skills/migrate-server/SKILL.md", "utf8");
  const safety = await fs.readFile("plugins/hostshift/skills/migrate-server/references/safety-model.md", "utf8");

  assert.equal(manifest.name, "hostshift");
  assert.equal(manifest.version, "0.3.0");
  assert.equal(manifest.license, "Apache-2.0");
  assert.equal(manifest.repository, "https://github.com/oguzhankrcb/HostShift");
  assert.equal(manifest.skills, "./skills/");
  assert.equal(manifest.interface.displayName, "HostShift");
  assert.match(skill, /^---\nname: migrate-server\n/m);
  assert.match(skill, /The skill is an operator layer/);
  assert.match(skill, /Do not silently fall back to ad hoc shell commands/);
  assert.match(safety, /immutable observation endpoint/);
});

test("package no longer exposes Node runtime bins", async () => {
  const manifest = JSON.parse(await fs.readFile("package.json", "utf8"));
  assert.equal(manifest.bin, undefined);
  assert.equal(manifest.scripts.check, "node --test");
});

test("readme documents hostshift execution commands", async () => {
  const readme = await fs.readFile("README.md", "utf8");
  assert.match(readme, /hostshift discover/);
  assert.match(readme, /hostshift prepare/);
  assert.match(readme, /strictly read-only|read-only-source/i);
  assert.match(readme, /codex plugin marketplace add/);
  assert.match(readme, /codex plugin add hostshift@hostshift/);
  assert.match(readme, /docs\/install\.md/);
  assert.match(readme, /docs\/validation\.md/);
  assert.match(readme, /examples\/web-stack-v2\.yaml/);
});

test("skill frontmatter declares distro-neutral migrate skill", async () => {
  const skill = await fs.readFile("skills/migrate-server/SKILL.md", "utf8");
  assert.match(skill, /^---\nname: migrate-server\n/m);
  assert.match(skill, /strictly read-only source policy/);
});

test("example profile remains JSON-compatible YAML", async () => {
  const profile = JSON.parse(await fs.readFile("examples/profile.yaml", "utf8"));
  assert.equal(profile.source.policy, "strict-read-only");
  assert.equal(profile.approved, false);
});

test("v2 example profile documents env secret references", async () => {
  const profile = await fs.readFile("examples/profile.v2.yaml", "utf8");
  assert.match(profile, /schemaVersion: 2/);
  assert.match(profile, /type: file-set/);
  assert.match(profile, /sourcePasswordEnv: SRC_MYSQL_PWD/);
  assert.match(profile, /targetPasswordEnv: DST_MYSQL_PWD/);
  assert.match(profile, /type: http/);
  assert.match(profile, /type: laravelDatabase/);
});

test("public web stack example covers cross-distro release scenario", async () => {
  const profile = await fs.readFile("examples/web-stack-v2.yaml", "utf8");
  assert.match(profile, /source: ubuntu:22\.04/);
  assert.match(profile, /target: debian:12/);
  assert.match(profile, /sourcePolicy: strict-read-only/);
  assert.match(profile, /type: docker-compose/);
  assert.match(profile, /type: mysql/);
  assert.match(profile, /type: postgresql/);
  assert.match(profile, /type: nginxConfig/);
  assert.match(profile, /approved: false/);
});

test("release validation gates are documented", async () => {
  const validation = await fs.readFile("docs/validation.md", "utf8");
  assert.match(validation, /HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker/);
  assert.match(validation, /HOSTSHIFT_RUN_VM_E2E=1 bash tests\/e2e\/vm\/run-vm-e2e\.sh --apply/);
  assert.match(validation, /source checksum immutability/i);
  assert.match(validation, /SPDX SBOM/);
  assert.match(validation, /checksums\.txt\.sig/);
  assert.match(validation, /checksums\.txt\.pem/);
  assert.match(validation, /artifact provenance attestation/i);
  assert.match(validation, /vm-e2e-apply/);
});

test("documentation website is scaffolded with Starlight and Docker Compose", async () => {
  const manifest = JSON.parse(await fs.readFile("docs-site/package.json", "utf8"));
  const config = await fs.readFile("docs-site/astro.config.mjs", "utf8");
  const compose = await fs.readFile("docs-site/compose.yml", "utf8");
  const dockerfile = await fs.readFile("docs-site/Dockerfile", "utf8");
  const overview = await fs.readFile("docs-site/src/content/docs/overview.md", "utf8");
  const runner = await fs.readFile("docs-site/src/content/docs/operations/self-hosted-runner.md", "utf8");

  assert.equal(manifest.dependencies.astro, "7.0.6");
  assert.equal(manifest.dependencies["@astrojs/starlight"], "0.41.2");
  assert.match(manifest.scripts.build, /ASTRO_TELEMETRY_DISABLED=1/);
  assert.match(config, /starlight\(/);
  assert.match(config, /disable404Route: true/);
  assert.match(compose, /4321:4321/);
  assert.match(compose, /ASTRO_TELEMETRY_DISABLED/);
  assert.match(dockerfile, /CMD \["npm", "run", "dev"\]/);
  assert.match(overview, /source server exactly as it is/i);
  assert.match(runner, /hostshift-vm/);
});

test("root package and workflows validate the documentation website", async () => {
  const manifest = JSON.parse(await fs.readFile("package.json", "utf8"));
  const ci = await fs.readFile(".github/workflows/ci.yml", "utf8");
  const release = await fs.readFile(".github/workflows/release.yml", "utf8");

  assert.equal(manifest.scripts["docs:build"], "npm --prefix docs-site run build");
  assert.equal(manifest.scripts["docs:compose:config"], "docker compose -f docs-site/compose.yml config");
  assert.match(ci, /npm --prefix docs-site ci/);
  assert.match(ci, /npm run docs:build/);
  assert.match(ci, /npm run docs:compose:config/);
  assert.match(release, /npm --prefix docs-site ci/);
  assert.match(release, /npm run docs:build/);
  assert.match(release, /npm run docs:compose:config/);
});

test("documentation website covers the project surface area", async () => {
  const config = await fs.readFile("docs-site/astro.config.mjs", "utf8");
  const install = await fs.readFile("docs-site/src/content/docs/getting-started/install.md", "utf8");
  const cli = await fs.readFile("docs-site/src/content/docs/reference/cli.md", "utf8");
  const profile = await fs.readFile("docs-site/src/content/docs/reference/profile-v2.md", "utf8");
  const discovery = await fs.readFile("docs-site/src/content/docs/reference/source-discovery.md", "utf8");
  const workloads = await fs.readFile("docs-site/src/content/docs/reference/workloads.md", "utf8");
  const checks = await fs.readFile("docs-site/src/content/docs/reference/checks.md", "utf8");
  const platforms = await fs.readFile("docs-site/src/content/docs/reference/platforms.md", "utf8");
  const state = await fs.readFile("docs-site/src/content/docs/reference/plans-state.md", "utf8");
  const matrix = await fs.readFile("docs-site/src/content/docs/reference/test-matrix.md", "utf8");

  for (const slug of [
    "reference/cli",
    "reference/profile-v2",
    "reference/source-discovery",
    "reference/workloads",
    "reference/checks",
    "reference/platforms",
    "reference/plans-state",
    "reference/test-matrix"
  ]) {
    assert.match(config, new RegExp(slug.replace("/", "\\/")));
  }

  assert.match(install, /codex plugin marketplace add/);
  assert.match(install, /codex plugin add hostshift@hostshift/);
  for (const command of ["doctor", "discover", "plan", "prepare", "sync", "verify", "cutover", "rollback", "profile migrate", "status", "resume", "policy source"]) {
    assert.match(cli, new RegExp(command.replace(" ", "\\s+")));
  }
  for (const field of ["schemaVersion", "sourcePolicy", "firewall", "sshd", "mysql", "workloads", "checks", "approved"]) {
    assert.match(profile, new RegExp(field));
  }
  for (const fact of ["osRelease", "packages", "ufwStatus", "nftRuleset", "dockerComposeProjects", "dockerContainers"]) {
    assert.match(discovery, new RegExp(fact));
  }
  for (const type of ["docker-compose", "docker-standalone", "file-set", "mysql", "mariadb", "postgresql"]) {
    assert.match(workloads, new RegExp(type));
  }
  for (const type of ["http", "laravelDatabase", "fileExists", "fileContains", "mysqlScalar", "postgresScalar", "serviceActive", "ufwRule", "nftRule", "nginxConfig"]) {
    assert.match(checks, new RegExp(type));
  }
  assert.match(platforms, /Ubuntu[\s\S]*22\.04[\s\S]*24\.04[\s\S]*25\.10[\s\S]*26\.04/);
  assert.match(platforms, /Debian[\s\S]*12[\s\S]*13/);
  assert.match(platforms, /docker-compose-plugin/);
  assert.match(state, /Action\{id, phase, hostRole, impact, command, preconditions, rollback\}/);
  assert.match(state, /audit\.jsonl/);
  assert.match(matrix, /HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker/);
  assert.match(matrix, /HOSTSHIFT_RUN_VM_E2E=1 bash tests\/e2e\/vm\/run-vm-e2e\.sh --apply/);
  assert.match(matrix, /self-hosted, macOS, hostshift-vm/);
});

test("release workflow packages only after hosted release gates pass", async () => {
  const workflow = await fs.readFile(".github/workflows/release.yml", "utf8");
  assert.match(workflow, /docker-matrix:/);
  assert.match(workflow, /HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker/);
  assert.match(workflow, /vm-e2e-preflight:/);
  assert.match(workflow, /HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm/);
  assert.match(workflow, /needs:\n\s+- quick-gates\n\s+- docker-matrix\n\s+- vm-e2e-preflight/);
  assert.match(workflow, /sigstore\/cosign-installer@v3/);
  assert.match(workflow, /cosign sign-blob --yes/);
  assert.match(workflow, /gh release upload "\$GITHUB_REF_NAME" dist\/checksums\.txt\.sig dist\/checksums\.txt\.pem --clobber/);
  assert.match(workflow, /actions\/attest-build-provenance@v2/);
});

test("ci workflow uses hosted VM preflight and self-hosted apply gate", async () => {
  const workflow = await fs.readFile(".github/workflows/ci.yml", "utf8");
  assert.match(workflow, /vm-e2e-preflight:/);
  assert.match(workflow, /HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm/);
  assert.match(workflow, /vm-e2e-apply-self-hosted:/);
  assert.match(workflow, /runs-on: \[self-hosted, macOS, hostshift-vm\]/);
  assert.match(workflow, /HOSTSHIFT_RUN_VM_E2E=1 bash tests\/e2e\/vm\/run-vm-e2e\.sh --apply/);
  assert.match(workflow, /Upload Lima logs on failure/);
  assert.match(workflow, /~\/\.lima\/\*\*\/ha\.stderr\.log/);
});

test("self-hosted VM apply workflow preserves the real VM release gate", async () => {
  const workflow = await fs.readFile(".github/workflows/vm-e2e-apply.yml", "utf8");
  assert.match(workflow, /runs-on: \[self-hosted, macOS, hostshift-vm\]/);
  assert.match(workflow, /HOSTSHIFT_RUN_VM_E2E=1 bash tests\/e2e\/vm\/run-vm-e2e\.sh --apply/);
});

test("gitignore excludes production secrets and generated artifacts", async () => {
  const gitignore = await fs.readFile(".gitignore", "utf8");
  assert.match(gitignore, /^runs\/$/m);
  assert.match(gitignore, /^\.env$/m);
  assert.match(gitignore, /^\.env\.\*$/m);
  assert.match(gitignore, /^!\.env\.example$/m);
  assert.match(gitignore, /^\*\.pem$/m);
  assert.match(gitignore, /^id_ed25519$/m);
  assert.match(gitignore, /^ssh_config$/m);
  assert.match(gitignore, /^dist\/$/m);
});
