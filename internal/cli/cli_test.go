package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlanReadsYAMLProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: yaml-example
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
approved: false
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"plan", "--profile", path, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"sourceWillBeModified": false`) {
		t.Fatalf("plan output did not preserve source safety: %s", stdout.String())
	}
}

func TestExplainSummarizesPlanForAIReview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: explain-app
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: debian:13
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: redis
    name: cache
    data:
      snapshotPath: /var/lib/redis/dump.rdb
checks:
  - type: http
    name: homepage
    data:
      url: http://127.0.0.1/
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"explain", "--profile", path, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{
		`"sourceWillBeModified": false`,
		`"readyForApply": true`,
		`"streamCount": 1`,
		`redis:cache`,
		`Run prepare, sync, and verify as dry-runs before any apply command.`,
		`MCP and AI integrations do not expose apply commands.`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected explain output to contain %q: %s", expected, out)
		}
	}
}

func TestReviewReportsStructuredFindingsForAIClients(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: review-app
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: debian:13
sourcePolicy: strict-read-only
approved: false
workloads:
  - type: docker-compose
    name: web
    data:
      workingDir: /srv/web
      configFile: /srv/web/docker-compose.yml
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"review", "--profile", path, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{
		`"status": "blocked"`,
		`"safeForAI": true`,
		`"Profile is not approved"`,
		`"Profile has no verification checks."`,
		`"Container workload has no HTTP or application database check."`,
		`"suggestedProfilePatch"`,
		`url: http://127.0.0.1/health`,
		`"Cross-distribution migration ubuntu:24.04`,
		`debian:13 requires workload compatibility checks"`,
		`"operatorChecklist"`,
		`"Do not suggest MCP apply operations; MCP exposes planning, review, and dry-run tools only."`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected review output to contain %q: %s", expected, out)
		}
	}
}

func TestCapabilitiesReportsAISafeCatalog(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"capabilities", "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{
		`"sourceWillBeModified": false`,
		`"applyToolsExposed": false`,
		`"id": "ubuntu"`,
		`"versionId": "24.04"`,
		`"type": "docker-compose"`,
		`"type": "docker-volume"`,
		`"type": "memcached"`,
		`"type": "certbot"`,
		`"type": "serviceActive"`,
		`"memcachedConfigPaths"`,
		`"name": "mysql-server"`,
		`"debianPackage": "default-mysql-server"`,
		`MCP tools do not expose apply operations.`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected capabilities output to contain %q: %s", expected, out)
		}
	}
}

func TestReviewReportsWorkloadSpecificMissingEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: review-workloads
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: mysql
    name: app
    data:
      sourcePasswordEnv: SRC_MYSQL_PWD
  - type: postgresql
    name: analytics
    data:
      sourcePasswordEnv: SRC_PG_PWD
      targetPasswordEnv: DST_PG_PWD
  - type: systemd-service
    name: queue
    data:
      service: queue.service
  - type: caddy
    name: caddy
    data:
      service: caddy.service
      config: /etc/caddy/Caddyfile
  - type: php-fpm
    name: php8.3-fpm
    data:
      service: php8.3-fpm.service
  - type: supervisor
    name: supervisor
    data:
      service: supervisor.service
  - type: fail2ban
    name: fail2ban
    data:
      service: fail2ban.service
  - type: memcached
    name: memcached
    data:
      service: memcached.service
      config: /etc/memcached.conf
  - type: rabbitmq
    name: rabbitmq
    data:
      service: rabbitmq-server.service
      configDir: /etc/rabbitmq
  - type: certbot
    name: certbot
    data:
      configDir: /etc/letsencrypt
  - type: logrotate
    name: logrotate
    data:
      config: /etc/logrotate.conf
  - type: file-set
    name: nginx-config
    data:
      paths:
        - /etc/nginx/sites-available/app.conf
      targetPath: /
  - type: cron
    name: cron
  - type: docker-volume
    name: uploads
    data:
      strategy: snapshot
      snapshotPath: /srv/hostshift-snapshots/uploads.tar
      targetPath: /srv/hostshift/volumes/uploads
  - type: docker-volume
    name: cache
    data:
      strategy: disposable
  - type: docker-volume
    name: database
    data:
      strategy: database-backed
  - type: docker-volume
    name: shared-media
    data:
      strategy: external
checks:
  - type: postgresScalar
    name: analytics-count
    data:
      database: analytics
      query: SELECT COUNT(*) FROM metrics
      expected: "2"
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"review", "--profile", path, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{
		`"status": "ready-for-dry-run"`,
		`"MySQL/MariaDB workload has no scalar data verification check."`,
		`"MySQL/MariaDB workload does not declare targetPasswordEnv."`,
		`type: mysqlScalar`,
		`targetPasswordEnv: DST_MYSQL_APP_PWD`,
		`"systemd-service workload has no matching serviceActive check."`,
		`service: queue.service`,
		`"Caddy workload has no matching serviceActive check."`,
		`service: caddy.service`,
		`"Caddy workload has no fileExists check for its config."`,
		`path: /etc/caddy/Caddyfile`,
		`"PHP-FPM workload has no matching serviceActive check."`,
		`service: php8.3-fpm.service`,
		`"Supervisor workload has no matching serviceActive check."`,
		`service: supervisor.service`,
		`"Fail2ban workload has no matching serviceActive check."`,
		`service: fail2ban.service`,
		`"Memcached workload has no matching serviceActive check."`,
		`service: memcached.service`,
		`"Memcached workload has no fileExists check for its config."`,
		`path: /etc/memcached.conf`,
		`"RabbitMQ workload has no matching serviceActive check."`,
		`service: rabbitmq-server.service`,
		`"RabbitMQ workload has no fileExists check for its config directory."`,
		`path: /etc/rabbitmq`,
		`"RabbitMQ workload preserves configuration only; live queues and messages are not migrated."`,
		`"Certbot workload has no fileExists check for its config directory."`,
		`path: /etc/letsencrypt`,
		`"Certbot workload preserves existing Let's Encrypt files only; DNS, ACME challenges, and future renewal behavior must be reviewed separately."`,
		`"Logrotate workload has no fileExists check for its main config."`,
		`path: /etc/logrotate.conf`,
		`"Nginx file-set has no nginxConfig validation check."`,
		`type: nginxConfig`,
		`"cron workload has no target serviceActive check."`,
		`service: cron`,
		`"Docker named volume uses an existing source-side snapshot tar; HostShift does not create the snapshot."`,
		`data under /srv/hostshift/volumes/uploads`,
		`"Docker named volume is marked disposable and its data will not be migrated."`,
		`"Docker named volume is marked database-backed and HostShift will not copy its filesystem contents."`,
		`"Docker named volume is marked external and HostShift will not migrate its data."`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected review output to contain %q: %s", expected, out)
		}
	}
	if strings.Contains(out, "PostgreSQL workload has no scalar data verification check.") {
		t.Fatalf("postgresScalar check should satisfy PostgreSQL workload evidence: %s", out)
	}
}

func TestProfileMigrateWritesV2YAML(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "v1.yaml")
	output := filepath.Join(dir, "v2.yaml")
	body := []byte(`schemaVersion: 1
name: legacy
source:
  ssh: old
  policy: strict-read-only
target:
  ssh: new
composeProjects:
  - name: web
    workingDir: /srv/web
    configFile: /srv/web/docker-compose.yml
approved: false
`)
	if err := os.WriteFile(input, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"profile", "migrate", "--input", input, "--output", output}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	migrated, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migrated), "schemaVersion: 2") {
		t.Fatalf("expected v2 yaml output, got: %s", string(migrated))
	}
	if !strings.Contains(string(migrated), "strict-read-only") {
		t.Fatalf("expected source policy in migrated profile, got: %s", string(migrated))
	}
}

func TestSBOMCommandWritesSPDXDocument(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "hostshift.sbom.spdx.json")
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"sbom", "--output", output, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"format": "SPDX-2.3"`) {
		t.Fatalf("expected sbom summary JSON, got: %s", stdout.String())
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	if document["spdxVersion"] != "SPDX-2.3" || document["SPDXID"] != "SPDXRef-DOCUMENT" {
		t.Fatalf("unexpected SBOM document header: %+v", document)
	}
	packages := document["packages"].([]any)
	if len(packages) == 0 {
		t.Fatal("expected SBOM packages")
	}
	first := packages[0].(map[string]any)
	if first["filesAnalyzed"] != false || first["licenseDeclared"] != "NOASSERTION" {
		t.Fatalf("unexpected SBOM package metadata: %+v", first)
	}
}

func TestDockerMatrixCommandListsRequiredPairs(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "docker", "--list"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{"ubuntu22 -> ubuntu24", "ubuntu22 -> debian12", "debian12 -> ubuntu22", "debian12 -> debian13"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected matrix list to contain %q: %s", expected, out)
		}
	}
}

func TestDockerMatrixCommandDryRunDocumentsImmutabilityChecks(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "docker"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "source immutability checks") || !strings.Contains(out, `"sourceWillBeModified": false`) {
		t.Fatalf("expected docker matrix dry-run safety output: %s", out)
	}
}

func TestDockerMatrixCommandFiltersSinglePair(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "docker", "--list", "--pair", "ubuntu22->debian12"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "ubuntu22 -> debian12" {
		t.Fatalf("unexpected filtered pair list: %s", stdout.String())
	}
}

func TestDockerMatrixCommandListsUniqueFixtureBaseImages(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "docker", "--list-images"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	want := []string{"debian:12", "debian:13", "ubuntu:22.04", "ubuntu:24.04", "ubuntu:25.10"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected image list:\nwant: %+v\n got: %+v", want, got)
	}
}

func TestVMMatrixCommandListsRequiredPairs(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "vm", "--list"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{"ubuntu22 -> ubuntu24", "ubuntu22 -> debian12", "debian12 -> ubuntu22", "debian12 -> debian13"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected VM matrix list to contain %q: %s", expected, out)
		}
	}
}

func TestVMMatrixCommandDryRunDocumentsProviderBootAndApplyBehavior(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "vm"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "provider preflight and VM boot") || !strings.Contains(out, "Add --apply") || !strings.Contains(out, `"sourceWillBeModified": false`) {
		t.Fatalf("expected VM matrix dry-run guidance: %s", out)
	}
}

func TestVMMatrixCommandFiltersSinglePair(t *testing.T) {
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"matrix", "vm", "--list", "--pair", "ubuntu22->debian12"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "ubuntu22 -> debian12" {
		t.Fatalf("unexpected VM filtered pair list: %s", stdout.String())
	}
}

func TestBuildSBOMDocumentUsesGoPURLs(t *testing.T) {
	doc := buildSBOMDocument([]goModule{{Name: "github.com/example/mod", Version: "v1.2.3"}, {Name: "github.com/example/root"}}, time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC))
	if doc.DocumentNamespace != "https://github.com/oguzhankaracabay/hostshift/sbom/1783425600000" {
		t.Fatalf("unexpected namespace: %s", doc.DocumentNamespace)
	}
	if doc.CreationInfo.Creators[0] != "Tool: hostshift sbom" {
		t.Fatalf("unexpected creator: %+v", doc.CreationInfo.Creators)
	}
	if got := doc.Packages[0].ExternalReference[0].ReferenceLocator; got != "pkg:golang/github.com%2Fexample%2Fmod@v1.2.3" {
		t.Fatalf("unexpected purl: %s", got)
	}
	if doc.Packages[1].VersionInfo != "main" || doc.Packages[1].DownloadLocation != "NOASSERTION" {
		t.Fatalf("unexpected root module package: %+v", doc.Packages[1])
	}
	if len(doc.Relationships) != 1 || doc.Relationships[0].RelationshipType != "DESCRIBES" {
		t.Fatalf("unexpected relationships: %+v", doc.Relationships)
	}
}

func TestPrepareDryRunReportsSkippedActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: dry-run
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"prepare", "--profile", path, "--json", "--state-dir", dir, "--run-id", "prepare-test"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"sourceWillBeModified": false`) {
		t.Fatalf("prepare output did not preserve source safety: %s", out)
	}
	if !strings.Contains(out, `"skipped": true`) {
		t.Fatalf("expected dry-run skipped result: %s", out)
	}
}

func TestPhaseDryRunReturnsGeneratedRunID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: generated-run-id
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"prepare", "--profile", path, "--json", "--state-dir", dir}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	runID, _ := result["runId"].(string)
	if !strings.HasPrefix(runID, "prepare-") {
		t.Fatalf("expected generated prepare run id, got %q", runID)
	}
	if _, err := os.Stat(filepath.Join(dir, "runs", runID, "state.json")); err != nil {
		t.Fatalf("generated run id must address saved state: %v", err)
	}
}

func TestResumeDryRunReportsPendingActionsWithoutChangingState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: resume-preview
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: file-set
    name: app-files
    data:
      paths:
        - /srv/app
      targetPath: /srv
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"prepare", "--profile", path, "--state-dir", dir, "--run-id", "resume-preview"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "runs", "resume-preview", "state.json")
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"resume", "--profile", path, "--state-dir", dir, "--run-id", "resume-preview", "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	for _, expected := range []string{
		`"resumed": true`,
		`"apply": false`,
		`"previousStatus": "dry-run"`,
		`"sourceWillBeModified": false`,
		`"dryRun": true`,
		`"skipped": true`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected resume preview to contain %q: %s", expected, out)
		}
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("resume preview must not mutate existing state\nbefore: %s\nafter: %s", before, after)
	}
}

func TestResumeRejectsProfileChangesAfterRunStarted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: resume-changed
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"prepare", "--profile", path, "--state-dir", dir, "--run-id", "resume-changed"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	changed := append(body, []byte("workloads:\n  - type: file-set\n    name: app\n    data:\n      paths: [/srv/app]\n      targetPath: /srv\n")...)
	if err := os.WriteFile(path, changed, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"resume", "--profile", path, "--state-dir", dir, "--run-id", "resume-changed", "--json"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("expected changed profile to block resume, got %v", err)
	}
}

func TestResumeCutoverApplyRequiresConfirmation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: resume-cutover
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: docker-compose
    name: web
    data:
      workingDir: /srv/web
      configFile: /srv/web/docker-compose.yml
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"cutover", "--profile", path, "--state-dir", dir, "--run-id", "resume-cutover"}, &bytes.Buffer{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"resume", "--profile", path, "--state-dir", dir, "--run-id", "resume-cutover", "--apply", "--confirm", "WRONG"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "invalid confirmation code") {
		t.Fatalf("expected resume cutover confirmation failure, got %v", err)
	}
}

func TestResumeApplyContinuesPendingTargetActionsThroughSSHRunner(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatal(err)
	}
	countPath := filepath.Join(dir, "ssh-count")
	logPath := filepath.Join(dir, "ssh.log")
	sshScript := `#!/bin/sh
set -eu
count=0
if [ -f "$HOSTSHIFT_FAKE_SSH_COUNT" ]; then
  count="$(cat "$HOSTSHIFT_FAKE_SSH_COUNT")"
fi
count=$((count + 1))
printf '%s\n' "$count" > "$HOSTSHIFT_FAKE_SSH_COUNT"
printf '%s\n' "$*" >> "$HOSTSHIFT_FAKE_SSH_LOG"
if [ "$count" -eq 2 ]; then
  echo "intentional target failure" >&2
  exit 42
fi
printf 'ok\n'
`
	sshPath := filepath.Join(binDir, "ssh")
	if err := os.WriteFile(sshPath, []byte(sshScript), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOSTSHIFT_FAKE_SSH_COUNT", countPath)
	t.Setenv("HOSTSHIFT_FAKE_SSH_LOG", logPath)
	profilePath := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: resume-ssh
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
firewall:
  enabled: true
  enable: true
  rules:
    - from: 10.0.0.0/8
      port: 443
      proto: tcp
approved: true
`)
	if err := os.WriteFile(profilePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"prepare", "--profile", profilePath, "--state-dir", dir, "--run-id", "resume-ssh", "--apply", "--json"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "intentional target failure") {
		t.Fatalf("expected initial target failure, got %v", err)
	}
	statePath := filepath.Join(dir, "runs", "resume-ssh", "state.json")
	stateBody, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var failedState map[string]any
	if err := json.Unmarshal(stateBody, &failedState); err != nil {
		t.Fatal(err)
	}
	failedAction, _ := failedState["failedAction"].(string)
	if failedState["status"] != "failed" || failedAction == "" || failedState["uncertainAction"] != failedAction {
		t.Fatalf("unexpected failed CLI state: %+v", failedState)
	}
	countBefore, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	err = Run(context.Background(), []string{"resume", "--profile", profilePath, "--state-dir", dir, "--run-id", "resume-ssh", "--apply", "--json"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--retry-failed "+failedAction) {
		t.Fatalf("expected explicit retry requirement, got %v", err)
	}
	countAfter, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(countBefore, countAfter) {
		t.Fatalf("unconfirmed resume must not invoke SSH: before=%s after=%s", countBefore, countAfter)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"resume", "--profile", profilePath, "--state-dir", dir, "--run-id", "resume-ssh", "--apply", "--retry-failed", failedAction, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), `"previouslyCompleted": true`) || !strings.Contains(stdout.String(), `"sourceWillBeModified": false`) {
		t.Fatalf("expected completed action skip and source safety in resume output: %s", stdout.String())
	}
	finishedBody, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var finished map[string]any
	if err := json.Unmarshal(finishedBody, &finished); err != nil {
		t.Fatal(err)
	}
	if finished["status"] != "completed" || finished["failedAction"] != nil || finished["uncertainAction"] != nil {
		t.Fatalf("unexpected completed CLI resume state: %+v", finished)
	}
	logBody, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(logBody), "old-server") {
		t.Fatalf("prepare/resume must not execute source SSH commands: %s", logBody)
	}
}

func TestSyncDryRunReportsStreamActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: stream-dry-run
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: mysql
    name: app
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"sync", "--profile", path, "--json", "--state-dir", dir, "--run-id", "sync-test"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"stream": true`) {
		t.Fatalf("expected stream result in sync dry-run: %s", out)
	}
	if !strings.Contains(out, `"sourceWillBeModified": false`) {
		t.Fatalf("sync output did not preserve source safety: %s", out)
	}
}

func TestPrepareApplyRefusesBlockers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: blocked
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
approved: false
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"prepare", "--profile", path, "--apply", "--state-dir", dir, "--run-id", "blocked-test"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected apply to refuse blockers")
	}
	if !strings.Contains(err.Error(), "Cannot apply") && !strings.Contains(err.Error(), "cannot apply") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCutoverDryRunReportsConfirmationCodeAndTargetActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: cutover-app
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: docker-compose
    name: web
    data:
      workingDir: /srv/web
      configFile: /srv/web/docker-compose.yml
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"cutover", "--profile", path, "--json", "--state-dir", dir, "--run-id", "cutover-test"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"confirmationCode": "START-CUTOVER-APP"`) {
		t.Fatalf("expected confirmation code in cutover dry-run: %s", out)
	}
	if !strings.Contains(out, `"sourceWillBeModified": false`) {
		t.Fatalf("cutover output did not preserve source safety: %s", out)
	}
	if !strings.Contains(out, "target.workload.docker-compose.web.up") {
		t.Fatalf("expected docker compose cutover action: %s", out)
	}
}

func TestCutoverApplyRequiresExactConfirmationCode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: cutover-app
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: docker-compose
    name: web
    data:
      workingDir: /srv/web
      configFile: /srv/web/docker-compose.yml
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), []string{"cutover", "--profile", path, "--apply", "--confirm", "WRONG", "--state-dir", dir, "--run-id", "cutover-test"}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected invalid confirmation code")
	}
	if !strings.Contains(err.Error(), "invalid confirmation code") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRollbackStatesThatSourceWasNotChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: rollback-app
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
approved: false
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"rollback", "--profile", path, "--json"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, `"sourceChanged": false`) {
		t.Fatalf("rollback should state source was unchanged: %s", out)
	}
	if !strings.Contains(out, `"automatic": false`) {
		t.Fatalf("rollback should be manual by default: %s", out)
	}
}

func TestVerifyDryRunIncludesTypedChecks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: verify-checks
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
checks:
  - type: http
    name: homepage
    data:
      url: http://127.0.0.1/
      hostHeader: example.com
      timeoutSeconds: 10
  - type: laravelDatabase
    name: Laravel DB
    data:
      container: app
approved: true
`)
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"verify", "--profile", path, "--json", "--state-dir", dir, "--run-id", "verify-test"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	out := stdout.String()
	if !strings.Contains(out, "target.check.http.homepage") {
		t.Fatalf("expected HTTP check in verify output: %s", out)
	}
	if !strings.Contains(out, "target.check.laravelDatabase.Laravel-DB") {
		t.Fatalf("expected Laravel check in verify output: %s", out)
	}
	if !strings.Contains(out, `"sourceWillBeModified": false`) {
		t.Fatalf("verify output did not preserve source safety: %s", out)
	}
}
