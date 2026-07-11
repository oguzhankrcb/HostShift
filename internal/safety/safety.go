package safety

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

var (
	safeSSHAlias            = regexp.MustCompile(`^[a-zA-Z0-9_.@:-]+$`)
	safeServiceName         = regexp.MustCompile(`^[a-zA-Z0-9_.@-]+$`)
	safeDockerName          = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
	safeDockerImage         = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_./:@-]*$`)
	safeDatabaseName        = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	safeEnvName             = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	safeSourceFindPattern   = regexp.MustCompile(`^[a-zA-Z0-9_./*?-]+$`)
	safeRedisHost           = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)
	passwordReference       = regexp.MustCompile(`^PGPASSWORD=\$\{([A-Z_][A-Z0-9_]*)\}$`)
	mysqlDumpScript         = regexp.MustCompile(`^if mysqldump --help \| grep -q -- '--no-tablespaces'; then exec mysqldump --single-transaction --quick --skip-lock-tables --no-tablespaces (?:--password=\$\{[A-Z_][A-Z0-9_]*\} )?--databases '[a-zA-Z0-9_.-]+'; else exec mysqldump --single-transaction --quick --skip-lock-tables (?:--password=\$\{[A-Z_][A-Z0-9_]*\} )?--databases '[a-zA-Z0-9_.-]+'; fi$`)
	mysqlDumpDatabase       = regexp.MustCompile(`--databases '([a-zA-Z0-9_.-]+)'`)
	mysqlDumpPasswordEnv    = regexp.MustCompile(`--password=\$\{([A-Z_][A-Z0-9_]*)\}`)
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
	switch command[0] {
	case "cat":
		return sourceCatCommand(command)
	case "uname":
		return exactSourceCommand(command, []string{"uname", "-m"})
	case "hostname":
		return exactSourceCommand(command, []string{"hostname"})
	case "df":
		return exactSourceCommand(command, []string{"df", "-Pk"})
	case "dpkg-query":
		return exactSourceCommand(command, []string{"dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\n"})
	case "systemctl":
		return sourceSystemctlCommand(command)
	case "findmnt":
		return exactSourceCommand(command, []string{"findmnt", "--json", "--real"})
	case "ss":
		return exactSourceCommand(command, []string{"ss", "-lntupH"})
	case "ufw":
		return exactSourceCommand(command, []string{"ufw", "status", "verbose"})
	case "nft":
		return exactSourceCommand(command, []string{"nft", "list", "ruleset"})
	case "sshd":
		return exactSourceCommand(command, []string{"sshd", "-T"})
	case "mysql":
		return exactSourceCommand(command, []string{"mysql", "--batch", "--skip-column-names", "--execute=SHOW DATABASES"})
	case "psql":
		return exactSourceCommand(command, []string{"psql", "--tuples-only", "--no-align", "--command=SELECT datname FROM pg_database WHERE datistemplate = false"})
	case "nginx":
		return exactSourceCommand(command, []string{"nginx", "-T"})
	case "apache2ctl":
		return exactSourceCommand(command, []string{"apache2ctl", "-S"})
	case "find":
		return sourceFindCommand(command)
	case "getent":
		return sourceGetentCommand(command)
	case "docker":
		return sourceDockerCommand(command)
	case "tar":
		return sourceTarCreateCommand(command)
	case "sh":
		return sourceMySQLDumpCommand(command)
	case "pg_dump":
		return sourcePostgreSQLDumpCommand(command)
	case "env":
		return sourceEnvPostgreSQLDumpCommand(command)
	case "redis-cli":
		return sourceRedisRDBCommand(command)
	default:
		return fmt.Errorf("source executable is not allowlisted: %s", command[0])
	}
}

func exactSourceCommand(command, expected []string) error {
	if len(command) != len(expected) {
		return fmt.Errorf("source command shape is not allowlisted: %s", command[0])
	}
	for index := range expected {
		if command[index] != expected[index] {
			return fmt.Errorf("source command argument is not allowlisted: %s", command[index])
		}
	}
	return nil
}

func sourceCatCommand(command []string) error {
	if len(command) < 2 {
		return fmt.Errorf("source cat command requires an absolute path")
	}
	for _, value := range command[1:] {
		if !strings.HasPrefix(value, "/") || path.Clean(value) != value {
			return fmt.Errorf("source cat command has unsafe path: %s", value)
		}
	}
	return nil
}

func sourceSystemctlCommand(command []string) error {
	if len(command) < 2 {
		return fmt.Errorf("source systemctl command requires a read-only operation")
	}
	switch command[1] {
	case "list-unit-files":
		return exactSourceCommand(command, []string{"systemctl", "list-unit-files", "--state=enabled", "--type=service", "--no-pager", "--no-legend"})
	case "list-units":
		return exactSourceCommand(command, []string{"systemctl", "list-units", "--state=running", "--type=service", "--no-pager", "--no-legend"})
	case "show":
		return sourceSystemctlShowCommand(command)
	default:
		return fmt.Errorf("source systemctl operation is not allowlisted: %s", command[1])
	}
}

func sourceSystemctlShowCommand(command []string) error {
	allowedProperties := map[string]bool{
		"--property=Id":                            true,
		"--property=ActiveState":                   true,
		"--property=SubState":                      true,
		"--property=MainPID":                       true,
		"--property=ActiveEnterTimestampMonotonic": true,
	}
	services := 0
	for _, value := range command[2:] {
		if value == "--no-pager" || allowedProperties[value] {
			continue
		}
		if !strings.HasSuffix(value, ".service") || ServiceName(value) != nil {
			return fmt.Errorf("source systemctl show argument is not allowlisted: %s", value)
		}
		services++
	}
	if services == 0 {
		return fmt.Errorf("source systemctl show requires a service")
	}
	return nil
}

func sourceFindCommand(command []string) error {
	if len(command) < 3 || command[len(command)-1] != "-print" {
		return fmt.Errorf("source find command must end with -print")
	}
	index := 1
	roots := 0
	for index < len(command) && strings.HasPrefix(command[index], "/") {
		if path.Clean(command[index]) != command[index] {
			return fmt.Errorf("source find command has unsafe root: %s", command[index])
		}
		roots++
		index++
	}
	if roots == 0 {
		return fmt.Errorf("source find command requires an absolute root")
	}
	for index < len(command) {
		switch command[index] {
		case "-maxdepth":
			index++
			if index >= len(command) {
				return fmt.Errorf("source find -maxdepth requires a value")
			}
			depth, err := strconv.Atoi(command[index])
			if err != nil || depth < 0 || depth > 10 {
				return fmt.Errorf("source find has invalid maxdepth: %s", command[index])
			}
		case "-type":
			index++
			if index >= len(command) || command[index] != "f" {
				return fmt.Errorf("source find may inspect only regular files")
			}
		case "-name", "-path":
			index++
			if index >= len(command) || !safeSourceFindPattern.MatchString(command[index]) {
				return fmt.Errorf("source find has unsafe pattern")
			}
			if command[index-1] == "-path" && !strings.HasPrefix(command[index], "/") {
				return fmt.Errorf("source find -path requires an absolute pattern")
			}
		case "(", ")", "-o", "-print":
		default:
			return fmt.Errorf("source find expression is not allowlisted: %s", command[index])
		}
		index++
	}
	return nil
}

func sourceGetentCommand(command []string) error {
	if len(command) != 2 || (command[1] != "passwd" && command[1] != "group") {
		return fmt.Errorf("source getent database is not allowlisted")
	}
	return nil
}

func sourceDockerCommand(command []string) error {
	allowed := [][]string{
		{"docker", "version", "--format", "{{json .Server.Version}}"},
		{"docker", "compose", "ls", "--format", "json"},
		{"docker", "ps", "--format", "{{json .}}"},
		{"docker", "volume", "ls", "--format", "{{json .}}"},
		{"docker", "network", "ls", "--format", "{{json .}}"},
	}
	for _, expected := range allowed {
		if exactSourceCommand(command, expected) == nil {
			return nil
		}
	}
	if len(command) == 4 && command[1] == "image" && command[2] == "save" {
		if err := DockerImage(command[3]); err != nil {
			return err
		}
		return nil
	}
	return fmt.Errorf("source docker operation is not allowlisted")
}

func sourceMySQLDumpCommand(command []string) error {
	if len(command) != 3 || command[1] != "-lc" || !mysqlDumpScript.MatchString(command[2]) {
		return fmt.Errorf("source shell command is not an allowlisted MySQL dump")
	}
	databases := mysqlDumpDatabase.FindAllStringSubmatch(command[2], -1)
	if len(databases) != 2 || databases[0][1] != databases[1][1] {
		return fmt.Errorf("source MySQL dump databases do not match")
	}
	passwords := mysqlDumpPasswordEnv.FindAllStringSubmatch(command[2], -1)
	if len(passwords) != 0 && (len(passwords) != 2 || passwords[0][1] != passwords[1][1]) {
		return fmt.Errorf("source MySQL dump password references do not match")
	}
	return nil
}

func sourcePostgreSQLDumpCommand(command []string) error {
	if len(command) != 4 || command[1] != "--format=custom" || command[2] != "--dbname" {
		return fmt.Errorf("source pg_dump command shape is not allowlisted")
	}
	return DatabaseName(command[3])
}

func sourceEnvPostgreSQLDumpCommand(command []string) error {
	if len(command) != 6 || !passwordReference.MatchString(command[1]) || command[2] != "pg_dump" {
		return fmt.Errorf("source env command is not an allowlisted PostgreSQL dump")
	}
	return sourcePostgreSQLDumpCommand(command[2:])
}

func sourceRedisRDBCommand(command []string) error {
	if len(command) != 7 || command[1] != "-h" || command[3] != "-p" || command[5] != "--rdb" || command[6] != "-" {
		return fmt.Errorf("source redis-cli command is not an allowlisted RDB export")
	}
	if !safeRedisHost.MatchString(command[2]) {
		return fmt.Errorf("source redis-cli has unsafe host")
	}
	port, err := strconv.Atoi(command[4])
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("source redis-cli has invalid port")
	}
	return nil
}

func sourceTarCreateCommand(command []string) error {
	prefix := []string{"tar", "--create", "--file=-", "--one-file-system", "--warning=no-file-changed", "-C", "/"}
	if len(command) < len(prefix) {
		return fmt.Errorf("source tar command is incomplete")
	}
	for index, expected := range prefix {
		if command[index] != expected {
			return fmt.Errorf("source tar command has unexpected argument: %s", command[index])
		}
	}
	for _, operand := range command[len(prefix):] {
		cleaned := path.Clean(operand)
		if strings.HasPrefix(operand, "-") || strings.HasPrefix(operand, "/") || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return fmt.Errorf("source tar command has unsafe transfer path: %s", operand)
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
