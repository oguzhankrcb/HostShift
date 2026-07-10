package safety

import "testing"

func TestSourceCommandRejectsMutations(t *testing.T) {
	for _, command := range [][]string{
		{"sudo", "cat", "/etc/passwd"},
		{"systemctl", "restart", "nginx"},
		{"service", "nginx", "restart"},
		{"docker", "exec", "app", "sh"},
		{"touch", "/tmp/hostshift"},
	} {
		if err := SourceCommand(command); err == nil {
			t.Fatalf("expected %v to be rejected", command)
		}
	}
}

func TestSourceCommandAllowsReadOnlySystemdServiceInventory(t *testing.T) {
	command := []string{"systemctl", "list-unit-files", "--state=enabled", "--type=service", "--no-pager", "--no-legend"}
	if err := SourceCommand(command); err != nil {
		t.Fatalf("expected read-only systemd inventory command to be allowed: %v", err)
	}
}

func TestTargetCommandAllowsTargetMutationsButRejectsControlChars(t *testing.T) {
	if err := TargetCommand([]string{"apt-get", "install", "-y", "rsync"}); err != nil {
		t.Fatalf("expected target package install command to be allowed: %v", err)
	}
	if err := TargetCommand([]string{"echo", "bad\narg"}); err == nil {
		t.Fatal("expected target command with control chars to be rejected")
	}
}

func TestRedactArgsMasksSecrets(t *testing.T) {
	got := RedactArgs([]string{"mysql", "--password=secret", "--user", "app", "--password", "another"})
	if got[1] != "[redacted]" {
		t.Fatalf("expected inline password redaction, got %+v", got)
	}
	if got[5] != "[redacted]" {
		t.Fatalf("expected following password value redaction, got %+v", got)
	}
}

func TestDatabaseAndEnvValidation(t *testing.T) {
	if err := DatabaseName("app_db-1"); err != nil {
		t.Fatalf("expected db name to be valid: %v", err)
	}
	if err := DatabaseName("app;drop"); err == nil {
		t.Fatal("expected unsafe db name to fail")
	}
	if err := EnvName("MYSQL_PASSWORD"); err != nil {
		t.Fatalf("expected env name to be valid: %v", err)
	}
	if err := EnvName("bad;name"); err == nil {
		t.Fatal("expected unsafe env name to fail")
	}
}

func TestTransferPathRejectsMachineSpecificPaths(t *testing.T) {
	if _, err := TransferPath("/etc/machine-id"); err == nil {
		t.Fatal("expected machine-id path to be rejected")
	}
	if _, err := TransferPath("/srv/app"); err != nil {
		t.Fatalf("expected /srv/app to be accepted: %v", err)
	}
}
