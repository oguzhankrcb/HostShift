package profile

import (
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/safety"
	"gopkg.in/yaml.v3"
)

const CurrentSchemaVersion = 2

type Profile struct {
	SchemaVersion int        `json:"schemaVersion" yaml:"schemaVersion"`
	Name          string     `json:"name" yaml:"name"`
	Source        Host       `json:"source" yaml:"source"`
	Target        Host       `json:"target" yaml:"target"`
	CreatedAt     string     `json:"createdAt,omitempty" yaml:"createdAt,omitempty"`
	SourcePolicy  string     `json:"sourcePolicy" yaml:"sourcePolicy"`
	Platforms     Platforms  `json:"platforms,omitempty" yaml:"platforms,omitempty"`
	Firewall      Firewall   `json:"firewall,omitempty" yaml:"firewall,omitempty"`
	SSHD          SSHD       `json:"sshd,omitempty" yaml:"sshd,omitempty"`
	MySQL         MySQL      `json:"mysql,omitempty" yaml:"mysql,omitempty"`
	Workloads     []Workload `json:"workloads,omitempty" yaml:"workloads,omitempty"`
	Checks        []Check    `json:"checks,omitempty" yaml:"checks,omitempty"`
	Approved      bool       `json:"approved" yaml:"approved"`
}

type Host struct {
	SSH string `json:"ssh" yaml:"ssh"`
}

type Platforms struct {
	Source string `json:"source,omitempty" yaml:"source,omitempty"`
	Target string `json:"target,omitempty" yaml:"target,omitempty"`
}

type Firewall struct {
	Enabled *bool          `json:"enabled,omitempty" yaml:"enabled,omitempty"`
	Enable  bool           `json:"enable,omitempty" yaml:"enable,omitempty"`
	Rules   []FirewallRule `json:"rules,omitempty" yaml:"rules,omitempty"`
}

type FirewallRule struct {
	From  string `json:"from" yaml:"from"`
	Port  int    `json:"port" yaml:"port"`
	Proto string `json:"proto" yaml:"proto"`
}

type SSHD struct {
	Settings map[string]int `json:"settings,omitempty" yaml:"settings,omitempty"`
}

type MySQL struct {
	Settings MySQLSettings `json:"settings,omitempty" yaml:"settings,omitempty"`
}

type MySQLSettings struct {
	BindAddress       string `json:"bindAddress,omitempty" yaml:"bindAddress,omitempty"`
	MySQLXBindAddress string `json:"mysqlxBindAddress,omitempty" yaml:"mysqlxBindAddress,omitempty"`
}

type Workload struct {
	Type string `json:"type" yaml:"type"`
	Name string `json:"name" yaml:"name"`
	Data any    `json:"data,omitempty" yaml:"data,omitempty"`
}

type Check struct {
	Type string `json:"type" yaml:"type"`
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	Data any    `json:"data,omitempty" yaml:"data,omitempty"`
}

type v1Profile struct {
	SchemaVersion int           `json:"schemaVersion" yaml:"schemaVersion"`
	Name          string        `json:"name" yaml:"name"`
	Source        v1Source      `json:"source" yaml:"source"`
	Target        Host          `json:"target" yaml:"target"`
	Compose       []v1Compose   `json:"composeProjects" yaml:"composeProjects"`
	Standalone    []v1Container `json:"standaloneContainers" yaml:"standaloneContainers"`
	FileSets      []v1FileSet   `json:"fileSets" yaml:"fileSets"`
	Databases     []v1Database  `json:"databases" yaml:"databases"`
	HealthChecks  []v1Check     `json:"healthChecks" yaml:"healthChecks"`
	AppChecks     []v1Check     `json:"applicationChecks" yaml:"applicationChecks"`
	Approved      bool          `json:"approved" yaml:"approved"`
}

type v1Source struct {
	SSH    string `json:"ssh" yaml:"ssh"`
	Policy string `json:"policy" yaml:"policy"`
}

type v1Compose struct {
	Name       string `json:"name" yaml:"name"`
	WorkingDir string `json:"workingDir" yaml:"workingDir"`
	ConfigFile string `json:"configFile" yaml:"configFile"`
}

type v1Container struct {
	Name  string `json:"name" yaml:"name"`
	Image string `json:"image" yaml:"image"`
}

type v1Database struct {
	Engine string `json:"engine" yaml:"engine"`
	Name   string `json:"name" yaml:"name"`
}

type v1FileSet struct {
	Name       string   `json:"name" yaml:"name"`
	Paths      []string `json:"paths" yaml:"paths"`
	TargetPath string   `json:"targetPath" yaml:"targetPath"`
}

type v1Check struct {
	Type           string `json:"type" yaml:"type"`
	Name           string `json:"name" yaml:"name"`
	URL            string `json:"url" yaml:"url"`
	HostHeader     string `json:"hostHeader" yaml:"hostHeader"`
	TimeoutSeconds int    `json:"timeoutSeconds" yaml:"timeoutSeconds"`
	Container      string `json:"container" yaml:"container"`
}

func Load(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var raw struct {
		SchemaVersion int `json:"schemaVersion" yaml:"schemaVersion"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Profile{}, fmt.Errorf("profile parser failed; v2 schema is documented in schemas/profile.v2.schema.json: %w", err)
	}
	if raw.SchemaVersion == 1 {
		var old v1Profile
		if err := yaml.Unmarshal(data, &old); err != nil {
			return Profile{}, err
		}
		return Validate(MigrateV1(old))
	}
	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return Profile{}, err
	}
	return Validate(profile)
}

func Save(path string, profile Profile) error {
	body, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o600)
}

func MigrateV1(old v1Profile) Profile {
	workloads := []Workload{}
	checks := []Check{}
	for _, item := range old.Compose {
		workloads = append(workloads, Workload{Type: "docker-compose", Name: item.Name, Data: item})
	}
	for _, item := range old.Standalone {
		workloads = append(workloads, Workload{Type: "docker-standalone", Name: item.Name, Data: item})
	}
	for _, item := range old.FileSets {
		workloads = append(workloads, Workload{Type: "file-set", Name: item.Name, Data: item})
	}
	for _, item := range old.Databases {
		workloads = append(workloads, Workload{Type: item.Engine, Name: item.Name, Data: item})
	}
	for _, item := range old.HealthChecks {
		checks = append(checks, migrateV1Check(item))
	}
	for _, item := range old.AppChecks {
		checks = append(checks, migrateV1Check(item))
	}
	return Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          old.Name,
		Source:        Host{SSH: old.Source.SSH},
		Target:        old.Target,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		SourcePolicy:  "strict-read-only",
		Workloads:     workloads,
		Checks:        checks,
		Approved:      old.Approved,
	}
}

func migrateV1Check(old v1Check) Check {
	data := map[string]any{}
	if old.URL != "" {
		data["url"] = old.URL
	}
	if old.HostHeader != "" {
		data["hostHeader"] = old.HostHeader
	}
	if old.TimeoutSeconds != 0 {
		data["timeoutSeconds"] = old.TimeoutSeconds
	}
	if old.Container != "" {
		data["container"] = old.Container
	}
	return Check{Type: old.Type, Name: old.Name, Data: data}
}

func Validate(profile Profile) (Profile, error) {
	if profile.SchemaVersion != CurrentSchemaVersion {
		return Profile{}, fmt.Errorf("unsupported profile schema version: %d", profile.SchemaVersion)
	}
	if profile.Name == "" {
		return Profile{}, fmt.Errorf("profile name is required")
	}
	if profile.Source.SSH == "" {
		return Profile{}, fmt.Errorf("source ssh is required")
	}
	if profile.SourcePolicy != "strict-read-only" {
		return Profile{}, fmt.Errorf("sourcePolicy must be strict-read-only")
	}
	if err := validateFirewall(profile.Firewall); err != nil {
		return Profile{}, err
	}
	if err := validateSSHD(profile.SSHD); err != nil {
		return Profile{}, err
	}
	if err := validateMySQL(profile.MySQL); err != nil {
		return Profile{}, err
	}
	for _, workload := range profile.Workloads {
		if err := validateWorkload(workload); err != nil {
			return Profile{}, err
		}
	}
	for _, check := range profile.Checks {
		if err := validateCheck(check); err != nil {
			return Profile{}, err
		}
	}
	return profile, nil
}

var safeWorkloadType = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
var safeCheckName = regexp.MustCompile(`^[a-zA-Z0-9_. -]+$`)
var safeHostHeader = regexp.MustCompile(`^[a-zA-Z0-9.-]+(:[0-9]{1,5})?$`)
var safeNFTIdentifier = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
var safeRedisHost = regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`)

func validateFirewall(firewall Firewall) error {
	for _, rule := range firewall.Rules {
		if rule.From == "" {
			return fmt.Errorf("firewall rule source is required")
		}
		if _, err := netip.ParseAddr(rule.From); err != nil {
			if _, prefixErr := netip.ParsePrefix(rule.From); prefixErr != nil {
				return fmt.Errorf("invalid firewall source: %s", rule.From)
			}
		}
		if rule.Port < 1 || rule.Port > 65535 {
			return fmt.Errorf("invalid firewall port: %d", rule.Port)
		}
		if rule.Proto != "tcp" && rule.Proto != "udp" {
			return fmt.Errorf("invalid firewall protocol: %s", rule.Proto)
		}
	}
	return nil
}

func validateSSHD(sshd SSHD) error {
	allowed := map[string]bool{
		"ClientAliveInterval": true,
		"ClientAliveCountMax": true,
	}
	for key, value := range sshd.Settings {
		if !allowed[key] {
			return fmt.Errorf("unsupported sshd setting: %s", key)
		}
		if value < 0 || value > 86400 {
			return fmt.Errorf("invalid sshd setting %s: %d", key, value)
		}
	}
	return nil
}

func validateMySQL(mysql MySQL) error {
	for key, value := range map[string]string{
		"bindAddress":       mysql.Settings.BindAddress,
		"mysqlxBindAddress": mysql.Settings.MySQLXBindAddress,
	} {
		if value == "" {
			continue
		}
		if _, err := netip.ParseAddr(value); err != nil {
			return fmt.Errorf("invalid MySQL %s: %s", key, value)
		}
	}
	return nil
}

func validateWorkload(workload Workload) error {
	if !safeWorkloadType.MatchString(workload.Type) {
		return fmt.Errorf("workload type contains unsafe characters: %s", workload.Type)
	}
	if !safeWorkloadType.MatchString(workload.Name) {
		return fmt.Errorf("workload name contains unsafe characters: %s", workload.Name)
	}
	data, _ := workload.Data.(map[string]any)
	switch workload.Type {
	case "docker-compose":
		for _, key := range []string{"workingDir", "configFile"} {
			if value, ok := data[key].(string); ok && value != "" {
				if _, err := safety.TransferPath(value); err != nil {
					return fmt.Errorf("%s workload %s has unsafe %s: %w", workload.Type, workload.Name, key, err)
				}
			}
		}
	case "docker-standalone":
		if value, ok := data["image"].(string); ok && value != "" {
			if err := safety.DockerImage(value); err != nil {
				return fmt.Errorf("docker-standalone workload %s has unsafe image: %w", workload.Name, err)
			}
		}
	case "file-set":
		if err := validateFileSetData(workload.Data); err != nil {
			return fmt.Errorf("file-set workload %s is invalid: %w", workload.Name, err)
		}
	case "apache-vhost":
		for _, value := range dataStringSlice(workload.Data, "modules", "Modules") {
			if err := safety.ServiceName(value); err != nil {
				return fmt.Errorf("apache-vhost workload %s has unsafe modules value: %w", workload.Name, err)
			}
		}
		for _, value := range dataStringSlice(workload.Data, "sites", "Sites") {
			if err := safety.ServiceName(value); err != nil {
				return fmt.Errorf("apache-vhost workload %s has unsafe sites value: %w", workload.Name, err)
			}
		}
	case "systemd-service":
		service := dataString(workload.Data, "service", "Service")
		if service == "" {
			service = workload.Name
		}
		if err := safety.ServiceName(service); err != nil {
			return fmt.Errorf("systemd-service workload %s has unsafe service: %w", workload.Name, err)
		}
		if unitPath := dataString(workload.Data, "unitPath", "UnitPath"); unitPath != "" {
			if _, err := safety.TransferPath(unitPath); err != nil {
				return fmt.Errorf("systemd-service workload %s has unsafe unitPath: %w", workload.Name, err)
			}
			unitName := strings.TrimPrefix(unitPath, "/etc/systemd/system/")
			if unitName == unitPath || strings.Contains(unitName, "/") || !strings.HasSuffix(unitName, ".service") {
				return fmt.Errorf("systemd-service workload %s unitPath must be under /etc/systemd/system and end with .service", workload.Name)
			}
		}
	case "cron":
		service := dataString(workload.Data, "service", "Service")
		if service != "" {
			if err := safety.ServiceName(service); err != nil {
				return fmt.Errorf("cron workload %s has unsafe service: %w", workload.Name, err)
			}
		}
	case "php-fpm":
		service := dataString(workload.Data, "service", "Service")
		if service == "" {
			service = workload.Name
		}
		if err := safety.ServiceName(service); err != nil {
			return fmt.Errorf("php-fpm workload %s has unsafe service: %w", workload.Name, err)
		}
	case "supervisor":
		service := dataString(workload.Data, "service", "Service")
		if service == "" {
			service = "supervisor.service"
		}
		if err := safety.ServiceName(service); err != nil {
			return fmt.Errorf("supervisor workload %s has unsafe service: %w", workload.Name, err)
		}
	case "fail2ban":
		service := dataString(workload.Data, "service", "Service")
		if service == "" {
			service = "fail2ban.service"
		}
		if err := safety.ServiceName(service); err != nil {
			return fmt.Errorf("fail2ban workload %s has unsafe service: %w", workload.Name, err)
		}
	case "logrotate":
		config := dataString(workload.Data, "config", "Config")
		if config == "" {
			config = "/etc/logrotate.conf"
		}
		if _, err := safety.TransferPath(config); err != nil {
			return fmt.Errorf("logrotate workload %s has unsafe config: %w", workload.Name, err)
		}
	case "mysql", "mariadb", "postgresql":
		if err := safety.DatabaseName(workload.Name); err != nil {
			return err
		}
		for _, key := range []string{"sourcePasswordEnv", "targetPasswordEnv"} {
			if value, ok := data[key].(string); ok && value != "" {
				if err := safety.EnvName(value); err != nil {
					return fmt.Errorf("%s workload %s has unsafe %s: %w", workload.Type, workload.Name, key, err)
				}
			}
		}
	case "redis":
		for _, key := range []string{"snapshotPath", "targetPath"} {
			if value := dataString(workload.Data, key); value != "" {
				if _, err := safety.TransferPath(value); err != nil {
					return fmt.Errorf("redis workload %s has unsafe %s: %w", workload.Name, key, err)
				}
			}
		}
		if host := dataString(workload.Data, "replicaHost", "ReplicaHost"); host != "" && !safeRedisHost.MatchString(host) {
			return fmt.Errorf("redis workload %s has unsafe replicaHost", workload.Name)
		}
		port := dataInt(workload.Data, "replicaPort", "ReplicaPort")
		if dataHasKey(workload.Data, "replicaPort", "ReplicaPort") && (port < 1 || port > 65535) {
			return fmt.Errorf("redis workload %s has invalid replicaPort: %d", workload.Name, port)
		}
	}
	return nil
}

func validateFileSetData(data any) error {
	paths := dataStringSlice(data, "paths", "Paths")
	if len(paths) == 0 {
		return fmt.Errorf("paths are required")
	}
	for _, item := range paths {
		if _, err := safety.TransferPath(item); err != nil {
			return err
		}
	}
	targetPath := dataString(data, "targetPath", "TargetPath")
	if targetPath == "" {
		targetPath = "/"
	}
	if _, err := safety.TransferPath(targetPath); err != nil && targetPath != "/" {
		return err
	}
	return nil
}

func validateCheck(check Check) error {
	if !safeWorkloadType.MatchString(check.Type) {
		return fmt.Errorf("check type contains unsafe characters: %s", check.Type)
	}
	if check.Name != "" && !safeCheckName.MatchString(check.Name) {
		return fmt.Errorf("check name contains unsafe characters: %s", check.Name)
	}
	switch check.Type {
	case "http":
		rawURL := dataString(check.Data, "url", "URL")
		parsed, err := url.Parse(rawURL)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("http check %s has invalid url", check.Name)
		}
		if hostHeader := dataString(check.Data, "hostHeader", "HostHeader"); hostHeader != "" && !safeHostHeader.MatchString(hostHeader) {
			return fmt.Errorf("http check %s has unsafe hostHeader", check.Name)
		}
		timeout := dataInt(check.Data, "timeoutSeconds", "TimeoutSeconds")
		if dataHasKey(check.Data, "timeoutSeconds", "TimeoutSeconds") && (timeout < 1 || timeout > 300) {
			return fmt.Errorf("http check %s has invalid timeoutSeconds", check.Name)
		}
	case "laravelDatabase":
		container := dataString(check.Data, "container", "Container")
		if err := safety.DockerName(container); err != nil {
			return fmt.Errorf("laravelDatabase check %s has unsafe container: %w", check.Name, err)
		}
	case "fileExists":
		filePath := dataString(check.Data, "path", "Path")
		if _, err := safety.TransferPath(filePath); err != nil {
			return fmt.Errorf("fileExists check %s has unsafe path: %w", check.Name, err)
		}
	case "fileContains":
		filePath := dataString(check.Data, "path", "Path")
		if _, err := safety.TransferPath(filePath); err != nil {
			return fmt.Errorf("fileContains check %s has unsafe path: %w", check.Name, err)
		}
		needle := dataString(check.Data, "contains", "Contains")
		if needle == "" || strings.ContainsAny(needle, "\x00\n") {
			return fmt.Errorf("fileContains check %s has invalid contains value", check.Name)
		}
	case "mysqlScalar":
		database := dataString(check.Data, "database", "Database")
		if err := safety.DatabaseName(database); err != nil {
			return fmt.Errorf("mysqlScalar check %s has unsafe database: %w", check.Name, err)
		}
		query := dataString(check.Data, "query", "Query")
		if err := validateSQLReadQuery(query); err != nil {
			return fmt.Errorf("mysqlScalar check %s has unsafe query: %w", check.Name, err)
		}
		expected := dataString(check.Data, "expected", "Expected")
		if expected == "" || strings.ContainsAny(expected, "\x00\n") {
			return fmt.Errorf("mysqlScalar check %s has invalid expected value", check.Name)
		}
	case "postgresScalar":
		database := dataString(check.Data, "database", "Database")
		if err := safety.DatabaseName(database); err != nil {
			return fmt.Errorf("postgresScalar check %s has unsafe database: %w", check.Name, err)
		}
		query := dataString(check.Data, "query", "Query")
		if err := validateSQLReadQuery(query); err != nil {
			return fmt.Errorf("postgresScalar check %s has unsafe query: %w", check.Name, err)
		}
		expected := dataString(check.Data, "expected", "Expected")
		if expected == "" || strings.ContainsAny(expected, "\x00\n") {
			return fmt.Errorf("postgresScalar check %s has invalid expected value", check.Name)
		}
	case "serviceActive":
		service := dataString(check.Data, "service", "Service")
		if err := safety.ServiceName(service); err != nil {
			return fmt.Errorf("serviceActive check %s has unsafe service: %w", check.Name, err)
		}
	case "ufwRule":
		rule := FirewallRule{
			From:  dataString(check.Data, "from", "From"),
			Port:  dataInt(check.Data, "port", "Port"),
			Proto: dataString(check.Data, "proto", "Proto"),
		}
		if err := validateFirewall(Firewall{Rules: []FirewallRule{rule}}); err != nil {
			return fmt.Errorf("ufwRule check %s is invalid: %w", check.Name, err)
		}
	case "nftRule":
		if err := validateNFTRuleCheck(check); err != nil {
			return fmt.Errorf("nftRule check %s is invalid: %w", check.Name, err)
		}
	case "nginxConfig":
		return nil
	default:
		return fmt.Errorf("unsupported check type: %s", check.Type)
	}
	return nil
}

func validateNFTRuleCheck(check Check) error {
	family := dataString(check.Data, "family", "Family")
	switch family {
	case "inet", "ip", "ip6":
	default:
		return fmt.Errorf("family must be inet, ip, or ip6")
	}
	for _, item := range []struct {
		label string
		value string
	}{
		{label: "table", value: dataString(check.Data, "table", "Table")},
		{label: "chain", value: dataString(check.Data, "chain", "Chain")},
	} {
		if item.value == "" || !safeNFTIdentifier.MatchString(item.value) {
			return fmt.Errorf("%s must match %s", item.label, safeNFTIdentifier.String())
		}
	}
	contains := dataString(check.Data, "contains", "Contains")
	if contains == "" || strings.ContainsAny(contains, "\x00\n") {
		return fmt.Errorf("contains must be a non-empty single-line literal")
	}
	return nil
}

func validateSQLReadQuery(query string) error {
	trimmed := strings.TrimSpace(query)
	lower := strings.ToLower(trimmed)
	if trimmed == "" || strings.ContainsAny(trimmed, "\x00\n") {
		return fmt.Errorf("query must be a single line")
	}
	if strings.Contains(trimmed, ";") || strings.Contains(lower, "--") || strings.Contains(lower, "/*") || strings.Contains(lower, "*/") {
		return fmt.Errorf("query must be a single statement")
	}
	if !strings.HasPrefix(lower, "select ") {
		return fmt.Errorf("query must start with SELECT")
	}
	for _, token := range []string{" insert ", " update ", " delete ", " drop ", " alter ", " create ", " truncate ", " replace ", " grant ", " revoke ", " load_file", " into outfile "} {
		if strings.Contains(" "+lower+" ", token) {
			return fmt.Errorf("query contains forbidden token %s", strings.TrimSpace(token))
		}
	}
	return nil
}

func dataString(data any, keys ...string) string {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			if value, ok := item[key].(string); ok {
				return value
			}
		}
	}
	switch item := data.(type) {
	case v1FileSet:
		for _, key := range keys {
			if key == "targetPath" || key == "TargetPath" {
				return item.TargetPath
			}
		}
	case v1Check:
		for _, key := range keys {
			switch key {
			case "url", "URL":
				return item.URL
			case "hostHeader", "HostHeader":
				return item.HostHeader
			case "container", "Container":
				return item.Container
			}
		}
	}
	return ""
}

func dataInt(data any, keys ...string) int {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			if raw, ok := item[key]; ok {
				switch value := raw.(type) {
				case int:
					return value
				case int64:
					return int(value)
				case float64:
					return int(value)
				}
			}
		}
	}
	if item, ok := data.(v1Check); ok {
		for _, key := range keys {
			if key == "timeoutSeconds" || key == "TimeoutSeconds" {
				return item.TimeoutSeconds
			}
		}
	}
	return 0
}

func dataHasKey(data any, keys ...string) bool {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			if _, ok := item[key]; ok {
				return true
			}
		}
	}
	if item, ok := data.(v1Check); ok {
		for _, key := range keys {
			if (key == "timeoutSeconds" || key == "TimeoutSeconds") && item.TimeoutSeconds != 0 {
				return true
			}
		}
	}
	return false
}

func dataStringSlice(data any, keys ...string) []string {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			if raw, ok := item[key]; ok {
				switch values := raw.(type) {
				case []string:
					return values
				case []any:
					out := make([]string, 0, len(values))
					for _, value := range values {
						if str, ok := value.(string); ok {
							out = append(out, str)
						}
					}
					return out
				}
			}
		}
	}
	switch item := data.(type) {
	case v1FileSet:
		return item.Paths
	}
	return nil
}
