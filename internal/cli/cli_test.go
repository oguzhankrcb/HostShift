package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
