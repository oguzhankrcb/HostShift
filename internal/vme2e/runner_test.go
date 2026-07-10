package vme2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

func TestRunnerRendersLimaTemplatesAndSourceSafeManifests(t *testing.T) {
	emitDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--pair", "ubuntu22->debian12", "--emit-dir", emitDir}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}

	workspace := filepath.Join(emitDir, "ubuntu22-to-debian12")
	sourcePlan := readJSONMap(t, filepath.Join(workspace, "source.plan.json"))
	targetPlan := readJSONMap(t, filepath.Join(workspace, "target.plan.json"))
	pair := readJSONMap(t, filepath.Join(workspace, "pair.json"))
	commands := readJSONMap(t, filepath.Join(workspace, "commands.json"))
	sourceTemplate := readText(t, filepath.Join(workspace, "source.lima.yaml"))
	targetTemplate := readText(t, filepath.Join(workspace, "target.lima.yaml"))
	sourceFixture := readText(t, filepath.Join(workspace, "fixtures", "source-bootstrap.sh"))
	commonFixture := readText(t, filepath.Join(workspace, "fixtures", "common-bootstrap.sh"))
	targetFixture := readText(t, filepath.Join(workspace, "fixtures", "target-bootstrap.sh"))

	if sourcePlan["sourcePolicy"] != "strict-read-only" {
		t.Fatalf("unexpected source policy: %#v", sourcePlan["sourcePolicy"])
	}
	if targetPlan["sourcePolicy"] != "target-mutable" {
		t.Fatalf("unexpected target policy: %#v", targetPlan["sourcePolicy"])
	}
	if nestedString(sourcePlan, "platform", "key") != "ubuntu22" || nestedString(targetPlan, "platform", "key") != "debian12" {
		t.Fatalf("unexpected plan platforms: source=%#v target=%#v", sourcePlan["platform"], targetPlan["platform"])
	}
	if nestedString(sourcePlan, "platform", "templateUrl") != "template:ubuntu-22.04" || nestedString(targetPlan, "platform", "templateUrl") != "template:debian-12" {
		t.Fatalf("unexpected template urls: source=%#v target=%#v", sourcePlan["platform"], targetPlan["platform"])
	}
	if pair["sourcePolicy"] != "strict-read-only" {
		t.Fatalf("unexpected pair source policy: %#v", pair["sourcePolicy"])
	}
	for _, expected := range []string{
		`base: "template:ubuntu-22.04"`,
		"guestIP: 127.0.0.1\n    guestPortRange: [1, 65535]\n    proto: any\n    ignore: true",
		`url: "./fixtures/common-bootstrap.sh"`,
		`url: "./fixtures/source-bootstrap.sh"`,
	} {
		if !strings.Contains(sourceTemplate, expected) {
			t.Fatalf("expected source template to contain %q:\n%s", expected, sourceTemplate)
		}
	}
	if strings.Contains(sourceTemplate, "vmType:") {
		t.Fatalf("source template should not contain vmType without override:\n%s", sourceTemplate)
	}
	for _, expected := range []string{
		`base: "template:debian-12"`,
		"guestIP: 0.0.0.0\n    guestIPMustBeZero: false\n    guestPortRange: [1, 65535]\n    proto: any\n    ignore: true",
		`url: "./fixtures/target-bootstrap.sh"`,
	} {
		if !strings.Contains(targetTemplate, expected) {
			t.Fatalf("expected target template to contain %q:\n%s", expected, targetTemplate)
		}
	}
	for _, expected := range []string{"/srv/hostshift-fixture/public/health", "getent passwd 501", "$3 >= 1000 && $3 < 60000", `MYSQL_SERVER_PACKAGE="default-mysql-server"`, "hostshift-fixture-app.service", "<VirtualHost *:8080>", "systemctl restart mysql || systemctl restart mariadb"} {
		if !strings.Contains(sourceFixture, expected) {
			t.Fatalf("expected source fixture to contain %q", expected)
		}
	}
	if !strings.Contains(commonFixture, "nftables") {
		t.Fatalf("expected common fixture to mention nftables")
	}
	for _, expected := range []string{"apt-get install -y apache2", "Listen 8080", "nft add table inet hostshift", "nft list ruleset > /etc/nftables.conf"} {
		if !strings.Contains(targetFixture, expected) {
			t.Fatalf("expected target fixture to contain %q", expected)
		}
	}

	if commands["sourcePolicy"] != "strict-read-only" {
		t.Fatalf("unexpected command plan source policy: %#v", commands["sourcePolicy"])
	}
	commandList := commands["commands"].([]any)
	firstCommand := stringSlice(commandList[0])
	if len(firstCommand) < 2 || firstCommand[0] != "limactl" || firstCommand[1] != "validate" {
		t.Fatalf("unexpected first command: %#v", firstCommand)
	}
	if !containsString(stringSlice(commandList[8]), "plan") || !containsString(stringSlice(commandList[11]), "cutover") || !containsString(stringSlice(commandList[13]), "verify") {
		t.Fatalf("expected hostshift plan/cutover/verify commands: %#v", commandList)
	}
	if !containsString(stringSlice(commandList[12]), "--apply") || !containsString(stringSlice(commandList[12]), "<confirmationCode>") {
		t.Fatalf("expected confirmed cutover apply command: %#v", commandList[12])
	}
	if got := strings.Join(stringSlice(commandList[14]), " "); got != "limactl stop hostshift-ubuntu22-to-debian12-target" {
		t.Fatalf("unexpected target stop command: %s", got)
	}
	if got := strings.Join(stringSlice(commandList[15]), " "); got != "limactl start hostshift-ubuntu22-to-debian12-target" {
		t.Fatalf("unexpected target start command: %s", got)
	}
	if !containsString(stringSlice(commandList[18]), "verify") {
		t.Fatalf("expected post-reboot verify command: %#v", commandList[18])
	}
}

func TestFixtureProfileCoversApacheAndStandaloneSystemdApplication(t *testing.T) {
	workspace := pairWorkspace{
		Pair:       matrixPair{Source: "ubuntu22", Target: "debian12"},
		SourcePlan: instancePlan{Platform: platformPlan{Family: "ubuntu", Release: "22.04"}, SSH: sshPlan{Alias: "source"}},
		TargetPlan: instancePlan{Platform: platformPlan{Family: "debian", Release: "12"}, SSH: sshPlan{Alias: "target"}},
	}
	body, err := json.Marshal(buildFixtureProfile(workspace))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"type":"apache-vhost"`,
		`"type":"systemd-service"`,
		`"service":"hostshift-fixture-app.service"`,
		`"url":"http://127.0.0.1:8080/health"`,
		`"path":"/etc/systemd/system/hostshift-fixture-app.service"`,
	} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("expected fixture profile to contain %s: %s", expected, body)
		}
	}

	profilePath := filepath.Join(t.TempDir(), "fixture.profile.json")
	if err := os.WriteFile(profilePath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	prof, err := profile.Load(profilePath)
	if err != nil {
		t.Fatalf("fixture profile must validate: %v", err)
	}
	plan, err := planner.Build(prof, time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture profile must plan: %v", err)
	}
	if len(plan.Blockers) != 0 {
		t.Fatalf("fixture profile must not have blockers: %v", plan.Blockers)
	}
	wantedActions := map[string]core.Phase{
		"target.workload.apache-vhost.hostshift-fixture.activate":     core.PhaseVerify,
		"target.workload.systemd-service.hostshift-fixture-app.start": core.PhaseCutover,
		"target.check.serviceActive.fixture-app-service":              core.PhaseVerify,
		"target.check.http.apache-health-http":                        core.PhaseVerify,
	}
	for _, action := range plan.Actions {
		if expectedPhase, ok := wantedActions[action.ID]; ok {
			if action.Phase != expectedPhase {
				t.Fatalf("action %s has phase %s, expected %s", action.ID, action.Phase, expectedPhase)
			}
			delete(wantedActions, action.ID)
		}
	}
	if len(wantedActions) != 0 {
		t.Fatalf("fixture plan is missing actions: %v", wantedActions)
	}
}

func TestCutoverConfirmationCodeRequiresValue(t *testing.T) {
	code, err := cutoverConfirmationCode(`{"confirmationCode":"CUTOVER-123"}`)
	if err != nil || code != "CUTOVER-123" {
		t.Fatalf("unexpected confirmation result: code=%q err=%v", code, err)
	}
	if _, err := cutoverConfirmationCode(`{"confirmationCode":""}`); err == nil {
		t.Fatal("expected missing confirmation code to fail")
	}
}

func TestRunnerCanForceLimaQEMUDriver(t *testing.T) {
	t.Setenv("HOSTSHIFT_VM_LIMA_VM_TYPE", "qemu")
	emitDir := t.TempDir()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--pair", "ubuntu22->ubuntu22", "--emit-dir", emitDir}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	workspace := filepath.Join(emitDir, "ubuntu22-to-ubuntu22")
	sourceTemplate := readText(t, filepath.Join(workspace, "source.lima.yaml"))
	sourcePlan := readJSONMap(t, filepath.Join(workspace, "source.plan.json"))
	if !strings.Contains(sourceTemplate, `vmType: "qemu"`) {
		t.Fatalf("expected qemu vmType in template:\n%s", sourceTemplate)
	}
	if nestedString(sourcePlan, "lima", "vmType") != "qemu" {
		t.Fatalf("expected qemu vmType in source plan: %#v", sourcePlan["lima"])
	}
}

func TestRunnerApplyPathExecutesLifecycleSnapshotAndHostShiftPhases(t *testing.T) {
	tempDir := t.TempDir()
	binDir := filepath.Join(tempDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(tempDir, "commands.log")
	fakeIdentity := filepath.Join(tempDir, "id_ed25519")
	if err := os.WriteFile(fakeIdentity, []byte("not-a-real-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	writeExecutable(t, filepath.Join(binDir, "limactl"), `#!/usr/bin/env bash
set -euo pipefail
echo "limactl $*" >> "${HOSTSHIFT_VM_TEST_LOG}"
command="${1:-}"
case "$command" in
  --version)
    echo "limactl version 1.0.0-test"
    ;;
  validate|start|stop|delete)
    ;;
  show-ssh)
    name="${@: -1}"
    port="60023"
    if [[ "$name" == *source* ]]; then port="60022"; fi
    echo 'Hostname="127.0.0.1"'
    echo "Port=\"${port}\""
    echo 'User="root"'
    echo "IdentityFile=\"${HOSTSHIFT_VM_TEST_IDENTITY}\""
    ;;
  *)
    echo "unexpected limactl invocation: $*" >&2
    exit 1
    ;;
esac
`)

	writeExecutable(t, filepath.Join(binDir, "ssh"), `#!/usr/bin/env bash
set -euo pipefail
echo "ssh $*" >> "${HOSTSHIFT_VM_TEST_LOG}"
if [[ "${1:-}" == "-F" ]]; then
  shift 3
else
  shift 1
fi
if [[ "${1:-}" == "sha256sum" ]]; then
  echo "abc123  /srv/hostshift-fixture/public/health"
  echo "def456  /srv/hostshift-fixture/systemd-marker"
  echo "ghi789  /etc/nginx/sites-available/hostshift-fixture.conf"
  echo "jkl012  /etc/apache2/ports.conf"
  echo "mno345  /etc/apache2/sites-available/hostshift-fixture.conf"
  echo "pqr678  /etc/systemd/system/hostshift-fixture-app.service"
  exit 0
fi
echo "unexpected ssh invocation: $*" >&2
exit 1
`)

	hostshiftPath := filepath.Join(binDir, "hostshift-stub")
	writeExecutable(t, hostshiftPath, `#!/usr/bin/env bash
set -euo pipefail
echo "hostshift $*" >> "${HOSTSHIFT_VM_TEST_LOG}"
command="${1:-}"
shift || true
case "$command" in
  discover)
    profile=""
    while [[ $# -gt 0 ]]; do
      if [[ "$1" == "--profile" ]]; then
        profile="$2"
        shift 2
      else
        shift
      fi
    done
    printf 'sourcePolicy: strict-read-only\n' > "$profile"
    printf '{"sourceWillBeModified":false,"requiredFailures":[],"facts":{"osRelease":{"ok":true}}}\n'
    ;;
  plan)
    if [[ "$*" == *"discovered.profile.yaml"* ]]; then
      printf '{"sourceWillBeModified":false,"blockers":["Profile is not approved","Target platform is unknown; package capabilities could not be mapped to distribution packages"],"actions":[]}\n'
    else
      printf '{"sourceWillBeModified":false,"blockers":[],"actions":[]}\n'
    fi
    ;;
  prepare|verify)
    printf '{"sourceWillBeModified":false,"blockers":[],"results":[{"actionId":"%s-action","dryRun":false,"skipped":false}]}\n' "$command"
    ;;
  sync)
    printf '{"sourceWillBeModified":false,"blockers":[],"results":[{"actionId":"sync-stream","dryRun":false,"skipped":false,"stream":true}]}\n'
    ;;
  cutover)
    apply=0
    for arg in "$@"; do
      if [[ "$arg" == "--apply" ]]; then apply=1; fi
    done
    if [[ "$apply" == "1" ]]; then
      printf '{"sourceWillBeModified":false,"blockers":[],"results":[{"actionId":"cutover-action","dryRun":false,"skipped":false}]}\n'
    else
      printf '{"sourceWillBeModified":false,"blockers":[],"confirmationCode":"CUTOVER-TEST","actions":[{"id":"cutover-action"}]}\n'
    fi
    ;;
  *)
    echo "unexpected hostshift invocation: $command $*" >&2
    exit 1
    ;;
esac
`)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOSTSHIFT_RUN_VM_E2E", "1")
	t.Setenv("HOSTSHIFT_VM_TEST_LOG", logPath)
	t.Setenv("HOSTSHIFT_VM_TEST_IDENTITY", fakeIdentity)
	t.Setenv("HOSTSHIFT_VM_HOSTSHIFT_BIN", hostshiftPath)

	emitDir := filepath.Join(tempDir, "emit")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--pair", "ubuntu22->debian12", "--emit-dir", emitDir, "--apply"}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}

	log := readText(t, logPath)
	for _, expected := range []string{
		"limactl --version",
		"limactl validate",
		"limactl start --tty=false --name hostshift-ubuntu22-to-debian12-source",
		"limactl show-ssh --format=options hostshift-ubuntu22-to-debian12-source",
		"ssh -F",
		"hostshift discover",
		"hostshift plan",
		"hostshift prepare",
		"hostshift sync",
		"hostshift cutover",
		"--confirm CUTOVER-TEST",
		"hostshift verify",
		"limactl stop hostshift-ubuntu22-to-debian12-target",
		"limactl delete --force hostshift-ubuntu22-to-debian12-source",
	} {
		if !strings.Contains(log, expected) {
			t.Fatalf("expected command log to contain %q:\n%s", expected, log)
		}
	}
	if !strings.Contains(stdout.String(), "VM apply executor completed successfully.") {
		t.Fatalf("expected success output, got: %s", stdout.String())
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	body := readText(t, path)
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func nestedString(document map[string]any, key, nestedKey string) string {
	nested, ok := document[key].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := nested[nestedKey].(string)
	return value
}

func stringSlice(value any) []string {
	items := value.([]any)
	out := make([]string, len(items))
	for index, item := range items {
		out[index], _ = item.(string)
	}
	return out
}

func containsString(items []string, expected string) bool {
	for _, item := range items {
		if item == expected {
			return true
		}
	}
	return false
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
