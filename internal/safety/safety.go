package safety

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

var (
	safeSSHAlias      = regexp.MustCompile(`^[a-zA-Z0-9_.@:-]+$`)
	safeServiceName   = regexp.MustCompile(`^[a-zA-Z0-9_.@-]+$`)
	safeDockerName    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	safeDockerImage   = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$`)
	safeDatabaseName  = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	safeEnvName       = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	forbiddenOnSource = []string{
		"sudo", " su ", "doas", "systemctl start", "systemctl stop",
		"systemctl restart", "systemctl reload", " service ", "kill", "pkill",
		"apt ", "apt-get", "dpkg -i", "snap install", "docker stop",
		"docker restart", "docker rm", "docker exec", "tee", "touch", "mkdir",
		"rm ", "mv ", "cp ", "chmod", "chown", "truncate", "sed -i",
		"mysql -e", "psql -c", "redis-cli set", ">", ">>",
	}
	machineSpecificExcludes = []string{
		"/etc/machine-id",
		"/var/lib/dbus/machine-id",
		"/etc/ssh/ssh_host_",
		"/etc/netplan",
		"/var/lib/cloud",
		"/etc/fstab",
		"/boot",
		"/proc",
		"/sys",
		"/dev",
		"/run",
		"/tmp",
		"/var/tmp",
		"/var/lib/docker",
	}
)

func SSHAlias(value string) error {
	if value == "" || !safeSSHAlias.MatchString(value) {
		return fmt.Errorf("ssh alias contains unsafe characters")
	}
	return nil
}

func ServiceName(value string) error {
	if value == "" || !safeServiceName.MatchString(value) {
		return fmt.Errorf("unsafe service name: %s", value)
	}
	return nil
}

func DockerName(value string) error {
	if value == "" || !safeDockerName.MatchString(value) {
		return fmt.Errorf("docker name contains unsafe characters: %s", value)
	}
	return nil
}

func DockerImage(value string) error {
	if value == "" || !safeDockerImage.MatchString(value) {
		return fmt.Errorf("docker image contains unsafe characters: %s", value)
	}
	return nil
}

func DatabaseName(value string) error {
	if value == "" || !safeDatabaseName.MatchString(value) {
		return fmt.Errorf("database name contains unsafe characters: %s", value)
	}
	return nil
}

func EnvName(value string) error {
	if value == "" || !safeEnvName.MatchString(value) {
		return fmt.Errorf("environment variable name contains unsafe characters: %s", value)
	}
	return nil
}

func TransferPath(value string) (string, error) {
	if value == "" || !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\n\x00") {
		return "", fmt.Errorf("transfer path must be a safe absolute path: %s", value)
	}
	normalized := path.Clean(value)
	if normalized == "/" || normalized == "/etc" || normalized == "/var" {
		return "", fmt.Errorf("transfer path is too broad: %s", normalized)
	}
	for _, entry := range machineSpecificExcludes {
		if strings.HasSuffix(entry, "_") {
			if strings.HasPrefix(normalized, entry) {
				return "", fmt.Errorf("transfer path is machine-specific: %s", normalized)
			}
			continue
		}
		if normalized == entry || strings.HasPrefix(normalized, entry+"/") {
			return "", fmt.Errorf("transfer path is machine-specific: %s", normalized)
		}
	}
	return normalized, nil
}

func SourceCommand(command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("source command is empty")
	}
	for _, arg := range command {
		if arg == "" || strings.ContainsAny(arg, "\n\x00") {
			return fmt.Errorf("source command contains unsafe argument")
		}
	}
	joined := " " + strings.ToLower(strings.Join(command, " ")) + " "
	for _, token := range forbiddenOnSource {
		if strings.Contains(joined, strings.ToLower(token)) {
			return fmt.Errorf("source command contains forbidden token: %s", strings.TrimSpace(token))
		}
	}
	return nil
}

func TargetCommand(command []string) error {
	if len(command) == 0 {
		return fmt.Errorf("target command is empty")
	}
	for _, arg := range command {
		if arg == "" || strings.ContainsAny(arg, "\n\x00") {
			return fmt.Errorf("target command contains unsafe argument")
		}
	}
	return nil
}

func Redact(value string) string {
	sensitiveKeys := []string{"password", "passwd", "secret", "token", "key", "credential"}
	lower := strings.ToLower(value)
	for _, key := range sensitiveKeys {
		if strings.Contains(lower, key+"=") || strings.Contains(lower, key+":") {
			return "[redacted]"
		}
	}
	return value
}

func RedactArgs(args []string) []string {
	redacted := make([]string, len(args))
	maskNext := false
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if maskNext {
			redacted[index] = "[redacted]"
			maskNext = false
			continue
		}
		if lower == "-p" || lower == "--password" || lower == "--password-file" || strings.HasSuffix(lower, "password") {
			redacted[index] = arg
			maskNext = true
			continue
		}
		if strings.Contains(lower, "password=") || strings.Contains(lower, "passwd=") || strings.Contains(lower, "secret=") || strings.Contains(lower, "token=") {
			redacted[index] = "[redacted]"
			continue
		}
		redacted[index] = Redact(arg)
	}
	return redacted
}
