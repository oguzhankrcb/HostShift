package dockere2e

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerListsRequiredDockerPairs(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--list"}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{"ubuntu22 -> ubuntu24", "ubuntu22 -> debian12", "debian12 -> ubuntu22", "debian12 -> debian13"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected list to contain %q:\n%s", expected, out)
		}
	}
}

func TestRunnerListsUniqueDockerBaseImages(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--list-images"}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v\nstderr: %s", err, stderr.String())
	}
	got := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	want := []string{"debian:12", "debian:13", "ubuntu:22.04", "ubuntu:24.04", "ubuntu:25.10"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected image list:\nwant: %+v\n got: %+v", want, got)
	}
}

func TestRunnerDryRunDocumentsImmutabilityChecks(t *testing.T) {
	binDir := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "commands.log")
	writeExecutable(t, filepath.Join(binDir, "docker"), `#!/usr/bin/env bash
set -euo pipefail
echo "docker $*" >> "${HOSTSHIFT_DOCKER_TEST_LOG}"
if [[ "${1:-}" == "compose" && "${2:-}" == "version" ]]; then
  echo "Docker Compose version test"
  exit 0
fi
echo "unexpected docker invocation: $*" >&2
exit 1
`)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOSTSHIFT_DOCKER_TEST_LOG", logPath)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := Run(context.Background(), []string{"--pair", "ubuntu22->debian12"}, &stdout, &stderr); err != nil {
		t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	for _, expected := range []string{
		"HostShift Docker migration matrix: 1 pairs",
		"ubuntu22 -> debian12",
		"Dry-run only. Set HOSTSHIFT_RUN_DOCKER_MATRIX=1",
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("expected dry-run output to contain %q:\n%s", expected, out)
		}
	}
	log := readText(t, logPath)
	if !strings.Contains(log, "docker compose version") {
		t.Fatalf("expected compose version preflight:\n%s", log)
	}
}

func TestBuildMatrixProfileCoversExtendedConfigWorkloads(t *testing.T) {
	profile := buildMatrixProfile(matrixPair{Source: "ubuntu22", Target: "debian12"}, map[string]string{"source": "source", "target": "target"})
	workloads := profile["workloads"].([]map[string]any)
	types := map[string]bool{}
	var fileSetPaths []string
	for _, workload := range workloads {
		types[workload["type"].(string)] = true
		if workload["type"] == "file-set" {
			data := workload["data"].(map[string]any)
			fileSetPaths = data["paths"].([]string)
		}
	}
	for _, expected := range []string{"docker-compose", "docker-standalone", "docker-volume", "caddy", "php-fpm", "supervisor", "fail2ban", "memcached", "rabbitmq", "certbot", "logrotate", "mysql", "postgresql", "redis"} {
		if !types[expected] {
			t.Fatalf("expected matrix profile workload %s in %+v", expected, workloads)
		}
	}
	for _, expected := range fixtureConfigTransferPaths {
		if !containsString(fileSetPaths, expected) {
			t.Fatalf("expected file-set transfer path %s in %+v", expected, fileSetPaths)
		}
	}
}

func TestBuildApplySmokeProfileTransfersExtendedConfigFiles(t *testing.T) {
	profile := buildApplySmokeProfile(matrixPair{Source: "ubuntu22", Target: "debian12"}, map[string]string{"source": "source", "target": "target"})
	workloads := profile["workloads"].([]map[string]any)
	types := map[string]bool{}
	var fileSetPaths []string
	for _, workload := range workloads {
		types[workload["type"].(string)] = true
		if workload["type"] == "file-set" {
			data := workload["data"].(map[string]any)
			fileSetPaths = data["paths"].([]string)
		}
	}
	for _, expected := range fixtureConfigTransferPaths {
		if !containsString(fileSetPaths, expected) {
			t.Fatalf("expected apply smoke transfer path %s in %+v", expected, fileSetPaths)
		}
	}
	if len(fixtureConfigFiles) < 10 {
		t.Fatalf("expected extended fixture config file coverage, got %+v", fixtureConfigFiles)
	}
	if !types["redis"] {
		t.Fatalf("expected Redis snapshot workload in apply smoke profile: %+v", workloads)
	}
	if !types["docker-volume"] {
		t.Fatalf("expected Docker volume snapshot workload in apply smoke profile: %+v", workloads)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
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

func readText(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
