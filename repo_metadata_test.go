package hostshift_test

import (
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestPluginManifestHasRequiredLocalPluginFields(t *testing.T) {
	manifest := readJSON(t, ".codex-plugin/plugin.json")
	requireEqual(t, manifest["name"], "hostshift")
	requireEqual(t, manifest["skills"], "./skills/")
	requireEqual(t, manifest["interface"].(map[string]any)["displayName"], "HostShift")
	if _, ok := manifest["interface"].(map[string]any)["defaultPrompt"].([]any); !ok {
		t.Fatal("expected defaultPrompt to be an array")
	}
}

func TestRepoMarketplaceExposesPackagedHostShiftPlugin(t *testing.T) {
	marketplace := readJSON(t, ".agents/plugins/marketplace.json")
	requireEqual(t, marketplace["name"], "hostshift")
	requireEqual(t, marketplace["interface"].(map[string]any)["displayName"], "HostShift")
	plugins := marketplace["plugins"].([]any)
	if len(plugins) != 1 {
		t.Fatalf("expected one marketplace plugin, got %d", len(plugins))
	}
	entry := plugins[0].(map[string]any)
	requireEqual(t, entry["name"], "hostshift")
	requireDeepEqual(t, entry["source"], map[string]any{"source": "local", "path": "./plugins/hostshift"})
	requireEqual(t, entry["policy"].(map[string]any)["installation"], "AVAILABLE")
	requireEqual(t, entry["policy"].(map[string]any)["authentication"], "ON_INSTALL")
	requireEqual(t, entry["category"], "Developer Tools")
}

func TestPackagedCodexPluginIncludesManifestSkillAndSafetyModel(t *testing.T) {
	manifest := readJSON(t, "plugins/hostshift/.codex-plugin/plugin.json")
	skill := readText(t, "plugins/hostshift/skills/migrate-server/SKILL.md")
	safety := readText(t, "plugins/hostshift/skills/migrate-server/references/safety-model.md")
	requireEqual(t, manifest["name"], "hostshift")
	requireEqual(t, manifest["version"], "0.3.0")
	requireEqual(t, manifest["license"], "Apache-2.0")
	requireEqual(t, manifest["repository"], "https://github.com/oguzhankrcb/HostShift")
	requireEqual(t, manifest["skills"], "./skills/")
	requireEqual(t, manifest["interface"].(map[string]any)["displayName"], "HostShift")
	requireMatch(t, skill, `(?m)^---\nname: migrate-server\n`)
	requireMatch(t, skill, `The skill is an operator layer`)
	requireMatch(t, skill, `Do not silently fall back to ad hoc shell commands`)
	requireMatch(t, safety, `immutable observation endpoint`)
}

func TestPackageNoLongerExposesNodeRuntimeBins(t *testing.T) {
	manifest := readJSON(t, "package.json")
	if _, ok := manifest["bin"]; ok {
		t.Fatalf("package.json must not expose Node runtime bins: %+v", manifest["bin"])
	}
	requireEqual(t, manifest["scripts"].(map[string]any)["check"], "make test-go")
}

func TestReadmeDocumentsHostshiftExecutionCommands(t *testing.T) {
	readme := readText(t, "README.md")
	for _, pattern := range []string{
		`hostshift discover`,
		`hostshift prepare`,
		`(?i)strictly read-only|read-only-source`,
		`codex plugin marketplace add`,
		`codex plugin add hostshift@hostshift`,
		`docs/install\.md`,
		`docs/validation\.md`,
		`examples/web-stack-v2\.yaml`,
	} {
		requireMatch(t, readme, pattern)
	}
}

func TestSkillFrontmatterDeclaresDistroNeutralMigrateSkill(t *testing.T) {
	skill := readText(t, "skills/migrate-server/SKILL.md")
	requireMatch(t, skill, `(?m)^---\nname: migrate-server\n`)
	requireMatch(t, skill, `strictly read-only source policy`)
}

func TestExampleProfileRemainsJSONCompatibleYAML(t *testing.T) {
	profile := readJSON(t, "examples/profile.yaml")
	requireEqual(t, profile["source"].(map[string]any)["policy"], "strict-read-only")
	requireEqual(t, profile["approved"], false)
}

func TestV2ExampleProfileDocumentsEnvSecretReferences(t *testing.T) {
	profile := readText(t, "examples/profile.v2.yaml")
	for _, pattern := range []string{
		`schemaVersion: 2`,
		`type: file-set`,
		`sourcePasswordEnv: SRC_MYSQL_PWD`,
		`targetPasswordEnv: DST_MYSQL_PWD`,
		`type: http`,
		`type: laravelDatabase`,
	} {
		requireMatch(t, profile, pattern)
	}
}

func TestPublicWebStackExampleCoversCrossDistroReleaseScenario(t *testing.T) {
	profile := readText(t, "examples/web-stack-v2.yaml")
	for _, pattern := range []string{
		`source: ubuntu:22\.04`,
		`target: debian:12`,
		`sourcePolicy: strict-read-only`,
		`type: docker-compose`,
		`type: mysql`,
		`type: postgresql`,
		`type: nginxConfig`,
		`approved: false`,
	} {
		requireMatch(t, profile, pattern)
	}
}

func TestReleaseValidationGatesAreDocumented(t *testing.T) {
	validation := readText(t, "docs/validation.md")
	for _, pattern := range []string{
		`HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker`,
		`HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e\.sh --apply`,
		`(?i)source checksum immutability`,
		`SPDX SBOM`,
		`checksums\.txt\.sig`,
		`checksums\.txt\.pem`,
		`(?i)artifact provenance attestation`,
		`vm-e2e-apply`,
	} {
		requireMatch(t, validation, pattern)
	}
}

func TestDocumentationWebsiteIsScaffoldedWithStarlightAndDockerCompose(t *testing.T) {
	manifest := readJSON(t, "docs-site/package.json")
	config := readText(t, "docs-site/astro.config.mjs")
	compose := readText(t, "docs-site/compose.yml")
	dockerfile := readText(t, "docs-site/Dockerfile")
	overview := readText(t, "docs-site/src/content/docs/overview.md")
	runner := readText(t, "docs-site/src/content/docs/operations/self-hosted-runner.md")

	dependencies := manifest["dependencies"].(map[string]any)
	requireEqual(t, dependencies["astro"], "7.0.6")
	requireEqual(t, dependencies["@astrojs/starlight"], "0.41.2")
	requireMatch(t, manifest["scripts"].(map[string]any)["build"].(string), `ASTRO_TELEMETRY_DISABLED=1`)
	requireMatch(t, config, `starlight\(`)
	requireMatch(t, config, `disable404Route: true`)
	requireMatch(t, compose, `4321:4321`)
	requireMatch(t, compose, `ASTRO_TELEMETRY_DISABLED`)
	requireMatch(t, dockerfile, `CMD \["npm", "run", "dev"\]`)
	requireMatch(t, overview, `(?i)source server exactly as it is`)
	requireMatch(t, runner, `hostshift-vm`)
}

func TestRootPackageAndWorkflowsValidateDocumentationWebsite(t *testing.T) {
	manifest := readJSON(t, "package.json")
	ci := readText(t, ".github/workflows/ci.yml")
	release := readText(t, ".github/workflows/release.yml")

	scripts := manifest["scripts"].(map[string]any)
	requireEqual(t, scripts["docs:build"], "npm --prefix docs-site run build")
	requireEqual(t, scripts["docs:compose:config"], "docker compose -f docs-site/compose.yml config")
	for _, body := range []string{ci, release} {
		requireMatch(t, body, `npm --prefix docs-site ci`)
		requireMatch(t, body, `npm run docs:build`)
		requireMatch(t, body, `npm run docs:compose:config`)
	}
}

func TestDocumentationWebsiteCoversProjectSurfaceArea(t *testing.T) {
	config := readText(t, "docs-site/astro.config.mjs")
	install := readText(t, "docs-site/src/content/docs/getting-started/install.md")
	ai := readText(t, "docs-site/src/content/docs/reference/ai-integrations.md")
	cli := readText(t, "docs-site/src/content/docs/reference/cli.md")
	profile := readText(t, "docs-site/src/content/docs/reference/profile-v2.md")
	discovery := readText(t, "docs-site/src/content/docs/reference/source-discovery.md")
	workloads := readText(t, "docs-site/src/content/docs/reference/workloads.md")
	checks := readText(t, "docs-site/src/content/docs/reference/checks.md")
	platforms := readText(t, "docs-site/src/content/docs/reference/platforms.md")
	state := readText(t, "docs-site/src/content/docs/reference/plans-state.md")
	matrix := readText(t, "docs-site/src/content/docs/reference/test-matrix.md")

	for _, slug := range []string{"reference/cli", "reference/ai-integrations", "reference/profile-v2", "reference/source-discovery", "reference/workloads", "reference/checks", "reference/platforms", "reference/plans-state", "reference/test-matrix"} {
		requireMatch(t, config, regexp.QuoteMeta(slug))
	}
	for _, pattern := range []string{`codex plugin marketplace add`, `codex plugin add hostshift@hostshift`, `hostshift mcp stdio`, `hostshift mcp doctor --json`} {
		requireMatch(t, install, pattern)
	}
	requireMatch(t, ai, `Claude Desktop`)
	requireMatch(t, ai, `hostshift mcp doctor --json`)
	requireMatch(t, ai, "No MCP tool exposes `--apply`")
	for _, command := range []string{"doctor", "discover", "plan", "explain", "review", "prepare", "sync", "verify", "cutover", "rollback", "mcp stdio", "mcp doctor", "profile migrate", "status", "resume", "policy source", "sbom", "matrix docker", "docker-e2e", "matrix vm", "vm-e2e"} {
		requireMatch(t, cli, strings.ReplaceAll(regexp.QuoteMeta(command), " ", `\s+`))
	}
	for _, field := range []string{"schemaVersion", "sourcePolicy", "firewall", "sshd", "mysql", "workloads", "checks", "approved"} {
		requireMatch(t, profile, field)
	}
	for _, fact := range []string{"osRelease", "packages", "ufwStatus", "nftRuleset", "supervisorConfigPaths", "fail2banConfigPaths", "customSystemdUnits", "dockerComposeProjects", "dockerContainers"} {
		requireMatch(t, discovery, fact)
	}
	for _, workload := range []string{"docker-compose", "docker-standalone", "file-set", "cron", "php-fpm", "supervisor", "fail2ban", "apache-vhost", "systemd-service", "mysql", "mariadb", "postgresql", "redis"} {
		requireMatch(t, workloads, workload)
	}
	for _, check := range []string{"http", "laravelDatabase", "fileExists", "fileContains", "mysqlScalar", "postgresScalar", "serviceActive", "ufwRule", "nftRule", "nginxConfig"} {
		requireMatch(t, checks, check)
	}
	requireMatch(t, platforms, `Ubuntu[\s\S]*22\.04[\s\S]*24\.04[\s\S]*25\.10[\s\S]*26\.04`)
	requireMatch(t, platforms, `Debian[\s\S]*12[\s\S]*13`)
	requireMatch(t, platforms, `docker-compose-plugin`)
	requireMatch(t, platforms, "`cron`[\\s\\S]*`cron`[\\s\\S]*`cron`")
	requireMatch(t, platforms, "`php-fpm`[\\s\\S]*`php-fpm`[\\s\\S]*`php-fpm`")
	requireMatch(t, platforms, "`supervisor`[\\s\\S]*`supervisor`[\\s\\S]*`supervisor`")
	requireMatch(t, platforms, "`fail2ban`[\\s\\S]*`fail2ban`[\\s\\S]*`fail2ban`")
	requireMatch(t, state, `Action\{id, phase, hostRole, impact, command, preconditions, rollback\}`)
	requireMatch(t, state, `audit\.jsonl`)
	requireMatch(t, matrix, `HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker`)
	requireMatch(t, matrix, `HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e\.sh --apply`)
	requireMatch(t, matrix, `self-hosted, macOS, hostshift-vm`)
}

func TestReleaseWorkflowPackagesOnlyAfterHostedReleaseGatesPass(t *testing.T) {
	workflow := readText(t, ".github/workflows/release.yml")
	for _, pattern := range []string{
		`docker-matrix:`,
		`HOSTSHIFT_RUN_DOCKER_MATRIX=1 make test-integration-docker`,
		`vm-e2e-preflight:`,
		`HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm`,
		`needs:\n\s+- quick-gates\n\s+- docker-matrix\n\s+- vm-e2e-preflight`,
		`sigstore/cosign-installer@v3`,
		`cosign sign-blob --yes`,
		`gh release upload "\$GITHUB_REF_NAME" dist/checksums\.txt\.sig dist/checksums\.txt\.pem --clobber`,
		`actions/attest-build-provenance@v2`,
	} {
		requireMatch(t, workflow, pattern)
	}
}

func TestCIWorkflowUsesHostedVMPreflightAndSelfHostedApplyGate(t *testing.T) {
	workflow := readText(t, ".github/workflows/ci.yml")
	for _, pattern := range []string{
		`vm-e2e-preflight:`,
		`HOSTSHIFT_RUN_VM_E2E=1 make test-e2e-vm`,
		`vm-e2e-apply-self-hosted:`,
		`runs-on: \[self-hosted, macOS, hostshift-vm\]`,
		`HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e\.sh --apply`,
		`Upload Lima logs on failure`,
		`~/\.lima/\*\*/ha\.stderr\.log`,
	} {
		requireMatch(t, workflow, pattern)
	}
}

func TestSelfHostedVMApplyWorkflowPreservesRealVMReleaseGate(t *testing.T) {
	workflow := readText(t, ".github/workflows/vm-e2e-apply.yml")
	requireMatch(t, workflow, `runs-on: \[self-hosted, macOS, hostshift-vm\]`)
	requireMatch(t, workflow, `HOSTSHIFT_RUN_VM_E2E=1 bash tests/e2e/vm/run-vm-e2e\.sh --apply`)
}

func TestGitignoreExcludesProductionSecretsAndGeneratedArtifacts(t *testing.T) {
	gitignore := readText(t, ".gitignore")
	for _, pattern := range []string{
		`(?m)^runs/$`,
		`(?m)^\.env$`,
		`(?m)^\.env\.\*$`,
		`(?m)^!\.env\.example$`,
		`(?m)^\*\.pem$`,
		`(?m)^id_ed25519$`,
		`(?m)^ssh_config$`,
		`(?m)^dist/$`,
	} {
		requireMatch(t, gitignore, pattern)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(readText(t, path)), &out); err != nil {
		t.Fatalf("failed to parse %s: %v", path, err)
	}
	return out
}

func requireEqual(t *testing.T, got, want any) {
	t.Helper()
	if got != want {
		t.Fatalf("unexpected value:\nwant: %#v\n got: %#v", want, got)
	}
}

func requireDeepEqual(t *testing.T, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected value:\nwant: %#v\n got: %#v", want, got)
	}
}

func requireMatch(t *testing.T, body, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(body) {
		t.Fatalf("expected pattern %q in:\n%s", pattern, body)
	}
}
