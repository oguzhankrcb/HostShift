package ssh

import (
	"os"
	"reflect"
	"testing"
)

func TestSSHBaseArgsWithoutConfigOverride(t *testing.T) {
	t.Setenv("HOSTSHIFT_SSH_CONFIG", "")
	got := sshBaseArgs("target-alias")
	want := []string{"target-alias"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ssh args without override: want=%v got=%v", want, got)
	}
}

func TestSSHBaseArgsWithConfigOverride(t *testing.T) {
	configPath := "/tmp/hostshift-ssh-config"
	if err := os.Setenv("HOSTSHIFT_SSH_CONFIG", configPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("HOSTSHIFT_SSH_CONFIG")
	})
	got := sshBaseArgs("target-alias")
	want := []string{"-F", configPath, "target-alias"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected ssh args with override: want=%v got=%v", want, got)
	}
}

func TestTargetCommandUsesExplicitSudoMode(t *testing.T) {
	t.Setenv("HOSTSHIFT_TARGET_SUDO", "")
	plain := targetCommand([]string{"apt-get", "install", "-y", "rsync"})
	if !reflect.DeepEqual(plain, []string{"apt-get", "install", "-y", "rsync"}) {
		t.Fatalf("unexpected plain target command: %v", plain)
	}

	t.Setenv("HOSTSHIFT_TARGET_SUDO", "1")
	withSudo := targetCommand([]string{"apt-get", "install", "-y", "rsync"})
	want := []string{"sudo", "--non-interactive", "--", "apt-get", "install", "-y", "rsync"}
	if !reflect.DeepEqual(withSudo, want) {
		t.Fatalf("unexpected sudo target command: want=%v got=%v", want, withSudo)
	}
}

func TestJoinRemoteCommandPreservesShellScriptArgument(t *testing.T) {
	got := joinRemoteCommand([]string{"sh", "-lc", "install -d -m 755 /etc/ssh/sshd_config.d && printf '%s\\n' ok"})
	want := "'sh' '-lc' 'install -d -m 755 /etc/ssh/sshd_config.d && printf '\"'\"'%s\\n'\"'\"' ok'"
	if got != want {
		t.Fatalf("unexpected remote command:\nwant: %s\n got: %s", want, got)
	}
}
