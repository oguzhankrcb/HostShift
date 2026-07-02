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

test("package exposes hostshift and legacy server-migrate bins", async () => {
  const manifest = JSON.parse(await fs.readFile("package.json", "utf8"));
  assert.equal(manifest.bin.hostshift, "./bin/hostshift.js");
  assert.equal(manifest.bin["server-migrate"], "./bin/server-migrate.js");
});

test("readme documents hostshift execution commands", async () => {
  const readme = await fs.readFile("README.md", "utf8");
  assert.match(readme, /hostshift discover/);
  assert.match(readme, /hostshift prepare/);
  assert.match(readme, /strictly read-only|read-only-source/i);
  assert.match(readme, /docs\/validation\.md/);
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

test("release workflow cannot publish before real migration gates pass", async () => {
  const workflow = await fs.readFile(".github/workflows/release.yml", "utf8");
  assert.match(workflow, /docker-matrix:/);
  assert.match(workflow, /HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker/);
  assert.match(workflow, /vm-e2e-apply:/);
  assert.match(workflow, /HOSTSHIFT_RUN_VM_E2E=1 bash tests\/e2e\/vm\/run-vm-e2e\.sh --apply/);
  assert.match(workflow, /needs:\n\s+- quick-gates\n\s+- docker-matrix\n\s+- vm-e2e-apply/);
  assert.match(workflow, /sigstore\/cosign-installer@v3/);
  assert.match(workflow, /cosign sign-blob --yes/);
  assert.match(workflow, /gh release upload "\$GITHUB_REF_NAME" dist\/checksums\.txt\.sig dist\/checksums\.txt\.pem --clobber/);
  assert.match(workflow, /actions\/attest-build-provenance@v2/);
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
