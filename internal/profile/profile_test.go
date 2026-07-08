package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMigratesV1Profile(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "profile.json")
	data := `{
  "schemaVersion": 1,
  "name": "example",
  "source": {"ssh": "old", "policy": "strict-read-only"},
  "target": {"ssh": "new"},
  "composeProjects": [{"name": "web", "workingDir": "/srv/web", "configFile": "/srv/web/docker-compose.yml"}],
  "standaloneContainers": [{"name": "worker", "image": "example/worker:latest"}],
  "fileSets": [{"name": "app-files", "paths": ["/srv/app"], "targetPath": "/srv"}],
  "databases": [{"engine": "mysql", "name": "app"}],
  "healthChecks": [{"type": "http", "name": "homepage", "url": "http://127.0.0.1/", "hostHeader": "example.com", "timeoutSeconds": 10}],
  "applicationChecks": [{"type": "laravelDatabase", "name": "Laravel DB", "container": "app"}],
  "approved": false
}`
	if err := os.WriteFile(input, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	prof, err := Load(input)
	if err != nil {
		t.Fatal(err)
	}
	if prof.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("expected v2 profile, got %d", prof.SchemaVersion)
	}
	if len(prof.Workloads) != 4 {
		t.Fatalf("expected 4 workloads, got %d", len(prof.Workloads))
	}
	if len(prof.Checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(prof.Checks))
	}
}

func TestValidateRejectsUnsafeChecks(t *testing.T) {
	base := Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "example",
		Source:        Host{SSH: "old"},
		Target:        Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
	}
	tests := []Check{
		{Type: "http", Name: "bad-url", Data: map[string]any{"url": "file:///etc/passwd"}},
		{Type: "http", Name: "bad-host", Data: map[string]any{"url": "http://127.0.0.1", "hostHeader": "bad\nhost"}},
		{Type: "http", Name: "zero-timeout", Data: map[string]any{"url": "http://127.0.0.1", "timeoutSeconds": 0}},
		{Type: "http", Name: "bad-timeout", Data: map[string]any{"url": "http://127.0.0.1", "timeoutSeconds": 301}},
		{Type: "laravelDatabase", Name: "bad-container", Data: map[string]any{"container": "bad container"}},
		{Type: "fileExists", Name: "bad-path", Data: map[string]any{"path": "/etc"}},
		{Type: "fileContains", Name: "bad-contains-path", Data: map[string]any{"path": "/etc", "contains": "ok"}},
		{Type: "fileContains", Name: "bad-contains-value", Data: map[string]any{"path": "/srv/app/health", "contains": "bad\nvalue"}},
		{Type: "mysqlScalar", Name: "bad-db", Data: map[string]any{"database": "app;drop", "query": "SELECT COUNT(*) FROM pages", "expected": "2"}},
		{Type: "mysqlScalar", Name: "bad-query", Data: map[string]any{"database": "app", "query": "DELETE FROM pages", "expected": "2"}},
		{Type: "mysqlScalar", Name: "multi-query", Data: map[string]any{"database": "app", "query": "SELECT COUNT(*) FROM pages; DROP TABLE pages", "expected": "2"}},
		{Type: "mysqlScalar", Name: "bad-expected", Data: map[string]any{"database": "app", "query": "SELECT COUNT(*) FROM pages", "expected": "2\n3"}},
		{Type: "postgresScalar", Name: "bad-query", Data: map[string]any{"database": "app", "query": "UPDATE metrics SET name='x'", "expected": "2"}},
		{Type: "serviceActive", Name: "bad-service", Data: map[string]any{"service": "bad service"}},
		{Type: "ufwRule", Name: "bad-rule", Data: map[string]any{"from": "bad;source", "port": 3306, "proto": "tcp"}},
		{Type: "nftRule", Name: "bad-family", Data: map[string]any{"family": "bridge", "table": "hostshift", "chain": "input", "contains": "tcp dport 3306 accept"}},
		{Type: "nftRule", Name: "bad-table", Data: map[string]any{"family": "inet", "table": "bad;table", "chain": "input", "contains": "tcp dport 3306 accept"}},
		{Type: "nftRule", Name: "bad-contains", Data: map[string]any{"family": "inet", "table": "hostshift", "chain": "input", "contains": "bad\nrule"}},
		{Type: "unknown", Name: "unknown"},
	}
	for _, check := range tests {
		prof := base
		prof.Checks = []Check{check}
		if _, err := Validate(prof); err == nil {
			t.Fatalf("expected unsafe check to fail validation: %+v", check)
		}
	}
}

func TestValidateAcceptsFileChecks(t *testing.T) {
	prof := Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "example",
		Source:        Host{SSH: "old"},
		Target:        Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
	}
	prof.Checks = []Check{
		{Type: "fileExists", Name: "health-file", Data: map[string]any{"path": "/srv/app/public/health"}},
		{Type: "fileContains", Name: "health-content", Data: map[string]any{"path": "/srv/app/public/health", "contains": "ok"}},
		{Type: "mysqlScalar", Name: "mysql-count", Data: map[string]any{"database": "app", "query": "SELECT COUNT(*) FROM pages", "expected": "2"}},
		{Type: "postgresScalar", Name: "postgres-count", Data: map[string]any{"database": "analytics", "query": "SELECT COUNT(*) FROM metrics", "expected": "2"}},
		{Type: "serviceActive", Name: "nginx", Data: map[string]any{"service": "nginx"}},
		{Type: "ufwRule", Name: "mysql-rule", Data: map[string]any{"from": "172.17.0.0/16", "port": 3306, "proto": "tcp"}},
		{Type: "nftRule", Name: "mysql-nft-rule", Data: map[string]any{"family": "inet", "table": "hostshift", "chain": "input", "contains": "tcp dport 3306 accept"}},
		{Type: "nginxConfig", Name: "reload"},
	}
	if _, err := Validate(prof); err != nil {
		t.Fatalf("expected file checks to validate: %v", err)
	}
}

func TestValidateRejectsUnsafeWorkloadMetadata(t *testing.T) {
	base := Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "example",
		Source:        Host{SSH: "old"},
		Target:        Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      false,
	}
	tests := []Workload{
		{Type: "mysql", Name: "app;drop"},
		{Type: "mysql", Name: "app", Data: map[string]any{"sourcePasswordEnv": "bad;env"}},
		{Type: "docker-compose", Name: "web", Data: map[string]any{"workingDir": "/etc"}},
		{Type: "docker-standalone", Name: "worker", Data: map[string]any{"image": "bad image"}},
		{Type: "file-set", Name: "files", Data: map[string]any{"paths": []any{"/etc"}}},
		{Type: "apache-vhost", Name: "apache", Data: map[string]any{"sites": []any{"bad site.conf"}}},
		{Type: "caddy", Name: "caddy", Data: map[string]any{"service": "bad service"}},
		{Type: "caddy", Name: "caddy", Data: map[string]any{"config": "/etc"}},
		{Type: "systemd-service", Name: "app", Data: map[string]any{"service": "bad service"}},
		{Type: "systemd-service", Name: "app", Data: map[string]any{"unitPath": "/etc/systemd"}},
		{Type: "cron", Name: "cron", Data: map[string]any{"service": "bad service"}},
		{Type: "php-fpm", Name: "php", Data: map[string]any{"service": "bad service"}},
		{Type: "supervisor", Name: "supervisor", Data: map[string]any{"service": "bad service"}},
		{Type: "fail2ban", Name: "fail2ban", Data: map[string]any{"service": "bad service"}},
		{Type: "logrotate", Name: "logrotate", Data: map[string]any{"config": "/etc"}},
		{Type: "redis", Name: "cache", Data: map[string]any{"snapshotPath": "/etc"}},
		{Type: "redis", Name: "cache", Data: map[string]any{"targetPath": "/var"}},
		{Type: "redis", Name: "cache", Data: map[string]any{"replicaHost": "bad host"}},
		{Type: "redis", Name: "cache", Data: map[string]any{"replicaHost": "127.0.0.1", "replicaPort": 70000}},
	}
	for _, workload := range tests {
		prof := base
		prof.Workloads = []Workload{workload}
		if _, err := Validate(prof); err == nil {
			t.Fatalf("expected unsafe workload to fail validation: %+v", workload)
		}
	}
}

func TestValidateAcceptsApacheSystemdAndRedisWorkloads(t *testing.T) {
	prof := Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "example",
		Source:        Host{SSH: "old"},
		Target:        Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      false,
		Workloads: []Workload{
			{Type: "apache-vhost", Name: "apache", Data: map[string]any{"modules": []any{"rewrite", "ssl"}, "sites": []any{"example.conf"}}},
			{Type: "caddy", Name: "caddy", Data: map[string]any{"service": "caddy.service", "config": "/etc/caddy/Caddyfile"}},
			{Type: "systemd-service", Name: "portfolio", Data: map[string]any{"service": "portfolio.service", "unitPath": "/etc/systemd/system/portfolio.service"}},
			{Type: "cron", Name: "cron", Data: map[string]any{"service": "cron.service"}},
			{Type: "php-fpm", Name: "php8.3-fpm", Data: map[string]any{"service": "php8.3-fpm.service"}},
			{Type: "supervisor", Name: "supervisor", Data: map[string]any{"service": "supervisor.service"}},
			{Type: "fail2ban", Name: "fail2ban", Data: map[string]any{"service": "fail2ban.service"}},
			{Type: "logrotate", Name: "logrotate", Data: map[string]any{"config": "/etc/logrotate.conf"}},
			{Type: "redis", Name: "cache-snapshot", Data: map[string]any{"snapshotPath": "/var/lib/redis/dump.rdb", "targetPath": "/var/lib/redis/dump.rdb"}},
			{Type: "redis", Name: "cache-replica", Data: map[string]any{"replicaHost": "127.0.0.1", "replicaPort": 6380}},
		},
	}
	if _, err := Validate(prof); err != nil {
		t.Fatalf("expected apache, systemd, and redis workloads to validate: %v", err)
	}
}

func TestValidateAcceptsFirstInstallTargetSettings(t *testing.T) {
	enabled := true
	prof := Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "example",
		Source:        Host{SSH: "old"},
		Target:        Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      false,
		Firewall: Firewall{
			Enabled: &enabled,
			Enable:  true,
			Rules: []FirewallRule{
				{From: "172.17.0.0/16", Port: 3306, Proto: "tcp"},
				{From: "2001:db8::/32", Port: 443, Proto: "tcp"},
			},
		},
		SSHD: SSHD{Settings: map[string]int{
			"ClientAliveInterval": 120,
			"ClientAliveCountMax": 720,
		}},
		MySQL: MySQL{Settings: MySQLSettings{
			BindAddress:       "0.0.0.0",
			MySQLXBindAddress: "127.0.0.1",
		}},
	}
	if _, err := Validate(prof); err != nil {
		t.Fatalf("expected target settings to validate: %v", err)
	}
}

func TestValidateRejectsUnsafeFirstInstallTargetSettings(t *testing.T) {
	base := Profile{
		SchemaVersion: CurrentSchemaVersion,
		Name:          "example",
		Source:        Host{SSH: "old"},
		Target:        Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      false,
	}
	tests := []Profile{
		func() Profile {
			prof := base
			prof.Firewall.Rules = []FirewallRule{{From: "bad;source", Port: 3306, Proto: "tcp"}}
			return prof
		}(),
		func() Profile {
			prof := base
			prof.Firewall.Rules = []FirewallRule{{From: "127.0.0.1", Port: 70000, Proto: "tcp"}}
			return prof
		}(),
		func() Profile {
			prof := base
			prof.Firewall.Rules = []FirewallRule{{From: "127.0.0.1", Port: 3306, Proto: "icmp"}}
			return prof
		}(),
		func() Profile {
			prof := base
			prof.SSHD.Settings = map[string]int{"PermitRootLogin": 1}
			return prof
		}(),
		func() Profile {
			prof := base
			prof.SSHD.Settings = map[string]int{"ClientAliveInterval": 90000}
			return prof
		}(),
		func() Profile {
			prof := base
			prof.MySQL.Settings.BindAddress = "127.0.0.1;touch"
			return prof
		}(),
	}
	for _, prof := range tests {
		if _, err := Validate(prof); err == nil {
			t.Fatalf("expected unsafe target settings to fail validation: %+v", prof)
		}
	}
}

func TestLoadRejectsUnsafeLegacyProfileAfterMigration(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "profile.yaml")
	data := `schemaVersion: 1
name: example
source:
  ssh: old
  policy: strict-read-only
target:
  ssh: new
fileSets:
  - name: unsafe
    paths:
      - /etc
    targetPath: /
approved: false
`
	if err := os.WriteFile(input, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(input); err == nil {
		t.Fatal("expected unsafe migrated v1 file-set to fail validation")
	}
}
