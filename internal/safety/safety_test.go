package safety

import "testing"

func TestSourceCommandRejectsMutations(t *testing.T) {
	for _, command := range [][]string{
		{"sudo", "cat", "/etc/passwd"},
		{"systemctl", "restart", "nginx"},
		{"service", "nginx", "restart"},
		{"dd", "if=/dev/zero", "of=/tmp/hostshift"},
		{"reboot"},
		{"shutdown", "-h", "now"},
		{"useradd", "hostshift"},
		{"mount", "/dev/sdb1", "/mnt"},
		{"iptables", "-F"},
		{"nft", "add", "table", "inet", "hostshift"},
		{"ufw", "enable"},
		{"docker", "run", "alpine"},
		{"docker", "exec", "app", "sh"},
		{"mysql", "--execute=DROP DATABASE app"},
		{"psql", "--command=DELETE FROM users"},
		{"find", "/srv", "-delete", "-print"},
		{"find", "/srv", "-exec", "rm", "{}", ";", "-print"},
		{"sh", "-lc", "touch /tmp/hostshift"},
		{"python3", "-c", "open('/tmp/hostshift', 'w').close()"},
		{"redis-cli", "-h", "127.0.0.1", "-p", "6379", "SET", "key", "value"},
		{"touch", "/tmp/hostshift"},
		{"unknown-read-tool", "--version"},
	} {
		if err := SourceCommand(command); err == nil {
			t.Fatalf("expected %v to be rejected", command)
		}
	}
}

func TestSourceCommandAllowsTypedReadOnlyExports(t *testing.T) {
	mysqlWithoutPassword := "if mysqldump --help | grep -q -- '--no-tablespaces'; then exec mysqldump --single-transaction --quick --skip-lock-tables --no-tablespaces --databases 'app'; else exec mysqldump --single-transaction --quick --skip-lock-tables --databases 'app'; fi"
	mysqlWithPassword := "if mysqldump --help | grep -q -- '--no-tablespaces'; then exec mysqldump --single-transaction --quick --skip-lock-tables --no-tablespaces --password=${MYSQL_PASSWORD} --databases 'app'; else exec mysqldump --single-transaction --quick --skip-lock-tables --password=${MYSQL_PASSWORD} --databases 'app'; fi"
	commands := [][]string{
		{"sh", "-lc", mysqlWithoutPassword},
		{"sh", "-lc", mysqlWithPassword},
		{"pg_dump", "--format=custom", "--dbname", "app"},
		{"env", "PGPASSWORD=${POSTGRES_PASSWORD}", "pg_dump", "--format=custom", "--dbname", "app"},
		{"redis-cli", "-h", "127.0.0.1", "-p", "6380", "--rdb", "-"},
		{"docker", "image", "save", "registry.example.com/team/app:1.2.3"},
		{"systemctl", "show", "--property=Id", "--property=ActiveState", "--property=SubState", "--property=MainPID", "--property=ActiveEnterTimestampMonotonic", "--no-pager", "nginx.service"},
	}
	for _, command := range commands {
		if err := SourceCommand(command); err != nil {
			t.Fatalf("expected typed read-only command %v to be allowed: %v", command, err)
		}
	}
}

func TestSourceCommandRejectsMalformedTypedExports(t *testing.T) {
	commands := [][]string{
		{"sh", "-lc", "mysqldump --databases app"},
		{"pg_dump", "--format=plain", "--dbname", "app"},
		{"env", "PGPASSWORD=literal-secret", "pg_dump", "--format=custom", "--dbname", "app"},
		{"redis-cli", "-h", "127.0.0.1", "-p", "0", "--rdb", "-"},
		{"docker", "image", "save", "app;touch"},
		{"systemctl", "show", "--property=Environment", "nginx.service"},
	}
	for _, command := range commands {
		if err := SourceCommand(command); err == nil {
			t.Fatalf("expected malformed typed command %v to be rejected", command)
		}
	}
}

func TestSourceCommandAllowsReadOnlySystemdServiceInventory(t *testing.T) {
	command := []string{"systemctl", "list-unit-files", "--state=enabled", "--type=service", "--no-pager", "--no-legend"}
	if err := SourceCommand(command); err != nil {
		t.Fatalf("expected read-only systemd inventory command to be allowed: %v", err)
	}
}

func TestSourceCommandAllowsTypedTarPathsThatMatchForbiddenCommandNames(t *testing.T) {
	command := []string{"tar", "--create", "--file=-", "--one-file-system", "--warning=no-file-changed", "-C", "/", "etc/logrotate.d/apt", "srv/app"}
	if err := SourceCommand(command); err != nil {
		t.Fatalf("expected typed read-only tar command to be allowed: %v", err)
	}
}

func TestSourceCommandRejectsTarOptionInjectionAndTraversal(t *testing.T) {
	for _, operand := range []string{"--checkpoint-action=exec=touch /tmp/changed", "../etc/shadow", "/etc/passwd"} {
		command := []string{"tar", "--create", "--file=-", "--one-file-system", "--warning=no-file-changed", "-C", "/", operand}
		if err := SourceCommand(command); err == nil {
			t.Fatalf("expected unsafe tar operand %q to be rejected", operand)
		}
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
