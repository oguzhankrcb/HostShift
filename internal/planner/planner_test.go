package planner

import (
	"strings"
	"testing"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

func TestBuildAllowsDebian12LTSTargets(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "debian:12"},
		Approved:      true,
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) != 0 {
		t.Fatalf("expected debian 12 lts target to be allowed, got blockers: %+v", plan.Blockers)
	}
}

func TestBuildBlocksExpiredTargets(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "debian:12"},
		Approved:      true,
	}
	plan, err := Build(prof, time.Date(2028, 7, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) == 0 {
		t.Fatal("expected expired debian 12 target blocker")
	}
}

func TestBuildWarnsForCrossDistroAndPreservesSourceSafety(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "debian:13"},
		Approved:      true,
		Workloads:     []profile.Workload{{Type: "mysql", Name: "app"}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if plan.SourceWillBeModified {
		t.Fatal("source must not be modified")
	}
	if len(plan.Warnings) == 0 {
		t.Fatal("expected cross-distro warning")
	}
	if len(plan.Streams) == 0 {
		t.Fatalf("expected workload stream, got %+v", plan.Streams)
	}
}

func TestTargetPackagePreparationUsesPlatformCapabilities(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "debian:13"},
		Approved:      true,
		Workloads: []profile.Workload{
			{Type: "file-set", Name: "files"},
			{Type: "file-set", Name: "nginx-files", Data: map[string]any{"paths": []any{"/etc/nginx/sites-available/example.conf"}, "targetPath": "/"}},
			{Type: "docker-compose", Name: "web"},
			{Type: "mysql", Name: "app"},
		},
		Checks: []profile.Check{
			{Type: "http", Name: "homepage", Data: map[string]any{"url": "http://127.0.0.1/"}},
			{Type: "laravelDatabase", Name: "app", Data: map[string]any{"container": "app"}},
		},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var actionCommand []string
	for _, action := range plan.Actions {
		if action.ID == "target.prepare.packages" {
			actionCommand = action.Command
			break
		}
	}
	if len(actionCommand) == 0 {
		t.Fatalf("expected package prepare action, got %+v", plan.Actions)
	}
	expected := []string{"apt-get", "install", "-y", "rsync", "tar", "curl", "nginx", "docker.io", "docker-compose-plugin", "default-mysql-client"}
	if strings.Join(actionCommand, " ") != strings.Join(expected, " ") {
		t.Fatalf("unexpected package command:\nwant: %+v\n got: %+v", expected, actionCommand)
	}
	if strings.Count(strings.Join(actionCommand, " "), "docker.io") != 1 {
		t.Fatalf("expected docker runtime package to be deduplicated, got %+v", actionCommand)
	}
}

func TestUnknownTargetPlatformBlocksPackagePreparation(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "unknown:1"},
		Approved:      true,
		Workloads:     []profile.Workload{{Type: "docker-compose", Name: "web"}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blockers) == 0 {
		t.Fatal("expected unknown platform blocker")
	}
	for _, action := range plan.Actions {
		if action.ID == "target.prepare.packages" {
			t.Fatalf("unknown target platform must not produce package install action: %+v", action)
		}
	}
}

func TestTargetConfigurationActionsAreTargetOnly(t *testing.T) {
	enabled := true
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Firewall: profile.Firewall{
			Enabled: &enabled,
			Enable:  true,
			Rules:   []profile.FirewallRule{{From: "172.17.0.0/16", Port: 3306, Proto: "tcp"}},
		},
		SSHD: profile.SSHD{Settings: map[string]int{
			"ClientAliveInterval": 120,
			"ClientAliveCountMax": 720,
		}},
		MySQL: profile.MySQL{Settings: profile.MySQLSettings{
			BindAddress:       "0.0.0.0",
			MySQLXBindAddress: "127.0.0.1",
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, action := range plan.Actions {
		ids[action.ID] = true
		if action.ID != "source.inventory" && strings.HasPrefix(action.ID, "target.") && action.HostRole != "target" {
			t.Fatalf("target action has wrong host role: %+v", action)
		}
		if strings.Contains(strings.Join(action.Command, "\n"), "\x00") {
			t.Fatalf("command contains NUL: %+v", action.Command)
		}
		for _, arg := range action.Command {
			if strings.Contains(arg, "\n") {
				t.Fatalf("command argument contains literal newline: %+v", action.Command)
			}
		}
	}
	for _, id := range []string{
		"target.firewall.ufw.allow.1",
		"target.firewall.ufw.enable",
		"target.sshd.settings",
		"target.mysql.bind-settings",
	} {
		if !ids[id] {
			t.Fatalf("expected action %s in plan: %+v", id, plan.Actions)
		}
	}
	var packageCommand []string
	for _, action := range plan.Actions {
		if action.ID == "target.prepare.packages" {
			packageCommand = action.Command
			break
		}
	}
	if !strings.Contains(strings.Join(packageCommand, " "), "ufw") {
		t.Fatalf("expected ufw package capability in prepare command, got %+v", packageCommand)
	}
	if !strings.Contains(strings.Join(packageCommand, " "), "openssh-server") {
		t.Fatalf("expected openssh-server package capability in prepare command, got %+v", packageCommand)
	}
	if !strings.Contains(strings.Join(packageCommand, " "), "mysql-server") {
		t.Fatalf("expected mysql-server package capability in prepare command, got %+v", packageCommand)
	}
}

func TestDisabledFirewallDoesNotPlanUFWActions(t *testing.T) {
	enabled := false
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Firewall: profile.Firewall{
			Enabled: &enabled,
			Rules:   []profile.FirewallRule{{From: "172.17.0.0/16", Port: 3306, Proto: "tcp"}},
		},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		if strings.HasPrefix(action.ID, "target.firewall.") {
			t.Fatalf("disabled firewall must not emit firewall action: %+v", action)
		}
	}
}

func TestStandaloneContainerUsesImageInReadOnlyStream(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "docker-standalone",
			Name: "portfolio",
			Data: map[string]any{"image": "portfolio:latest"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Streams) != 1 {
		t.Fatalf("expected one stream, got %+v", plan.Streams)
	}
	stream := plan.Streams[0]
	if got := stream.SourceCommand[len(stream.SourceCommand)-1]; got != "portfolio:latest" {
		t.Fatalf("expected image name, got %s", got)
	}
	if got := stream.TargetCommand[0]; got != "docker" {
		t.Fatalf("expected docker target load command, got %+v", stream.TargetCommand)
	}
}

func TestDatabaseWorkloadsProduceStreams(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{
			{Type: "mysql", Name: "app"},
			{Type: "postgresql", Name: "analytics"},
		},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Streams) != 2 {
		t.Fatalf("expected two database streams, got %+v", plan.Streams)
	}
	mysqlSource := strings.Join(plan.Streams[0].SourceCommand, " ")
	mysqlTarget := strings.Join(plan.Streams[0].TargetCommand, " ")
	if plan.Streams[0].SourceCommand[0] != "sh" || !strings.Contains(mysqlSource, "mysqldump") || !strings.Contains(mysqlSource, "--no-tablespaces") || plan.Streams[0].TargetCommand[0] != "sh" || !strings.Contains(mysqlTarget, "sed -e") || !strings.Contains(mysqlTarget, "utf8mb4_0900_ai_ci") || !strings.Contains(mysqlTarget, "mysql") {
		t.Fatalf("unexpected mysql stream: %+v", plan.Streams[0])
	}
	postgresTarget := strings.Join(plan.Streams[1].TargetCommand, " ")
	if plan.Streams[1].SourceCommand[0] != "pg_dump" || plan.Streams[1].TargetCommand[0] != "sh" || !strings.Contains(postgresTarget, "runuser -u postgres") || !strings.Contains(postgresTarget, "pg_restore") || !strings.Contains(postgresTarget, "--no-owner") || !strings.Contains(postgresTarget, "--no-acl") {
		t.Fatalf("unexpected postgresql stream: %+v", plan.Streams[1])
	}
}

func TestFileSetWorkloadProducesTarStream(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "file-set",
			Name: "app-files",
			Data: map[string]any{"paths": []any{"/srv/app", "/etc/nginx"}, "targetPath": "/"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Streams) != 1 {
		t.Fatalf("expected one file stream, got %+v", plan.Streams)
	}
	stream := plan.Streams[0]
	if stream.SourceCommand[0] != "tar" || stream.TargetCommand[0] != "tar" {
		t.Fatalf("expected tar stream, got %+v", stream)
	}
	if strings.Contains(strings.Join(stream.SourceCommand, " "), " /srv/app") {
		t.Fatalf("source tar paths should be relative to / after -C /: %+v", stream.SourceCommand)
	}
}

func TestComposeUsesWorkingDirAndConfigFile(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "docker-compose",
			Name: "web",
			Data: map[string]any{"workingDir": "/srv/web", "configFile": "/srv/web/docker-compose.yml"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	last := plan.Actions[len(plan.Actions)-1]
	if last.Command[0] != "sh" || !strings.Contains(last.Command[2], "/srv/web/docker-compose.yml") {
		t.Fatalf("expected compose command with working dir and config file, got %+v", last.Command)
	}
	if last.Phase != "cutover" || !strings.Contains(last.Command[2], "up") || !strings.Contains(last.Command[2], "--build") {
		t.Fatalf("expected compose cutover up action, got %+v", last)
	}
	if len(last.Rollback) == 0 || !strings.Contains(last.Rollback[0], "down") {
		t.Fatalf("expected compose rollback metadata, got %+v", last.Rollback)
	}
}

func TestStandaloneContainerPlansCutoverRunAction(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "docker-standalone",
			Name: "portfolio",
			Data: map[string]any{
				"image":         "portfolio:latest",
				"restartPolicy": "always",
				"safeEnvironment": map[string]any{
					"NODE_ENV": "production",
				},
				"portBindings": map[string]any{
					"3000/tcp": []any{map[string]any{"HostPort": "3001"}},
				},
			},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	var runActionFound bool
	for _, action := range plan.Actions {
		if action.ID == "target.workload.docker-standalone.portfolio.run" {
			runActionFound = true
			if action.Phase != "cutover" || action.HostRole != "target" || action.Impact != "service" {
				t.Fatalf("unexpected standalone run action metadata: %+v", action)
			}
			script := strings.Join(action.Command, " ")
			for _, expected := range []string{"docker inspect", "docker run", "portfolio:latest", "NODE_ENV=production", "3001:3000/tcp"} {
				if !strings.Contains(script, expected) {
					t.Fatalf("expected %q in standalone run script: %+v", expected, action.Command)
				}
			}
			if len(action.Rollback) == 0 || !strings.Contains(action.Rollback[0], "docker stop") {
				t.Fatalf("expected rollback metadata, got %+v", action.Rollback)
			}
		}
	}
	if !runActionFound {
		t.Fatalf("expected standalone cutover action, got %+v", plan.Actions)
	}
}

func TestDatabasePasswordEnvReferencesAreNotLiteralSecrets(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "mysql",
			Name: "app",
			Data: map[string]any{"sourcePasswordEnv": "SRC_MYSQL_PWD", "targetPasswordEnv": "DST_MYSQL_PWD"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	stream := plan.Streams[0]
	if !strings.Contains(strings.Join(stream.SourceCommand, " "), "${SRC_MYSQL_PWD}") || !strings.Contains(strings.Join(stream.TargetCommand, " "), "${DST_MYSQL_PWD}") {
		t.Fatalf("expected env references in stream commands: %+v", stream)
	}
	if strings.Contains(strings.Join(stream.SourceCommand, " "), "secret") || strings.Contains(strings.Join(stream.TargetCommand, " "), "secret") {
		t.Fatalf("literal secret leaked into stream command: %+v", stream)
	}
}

func TestInvalidPasswordEnvReferenceIsNeutralized(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "mysql",
			Name: "app",
			Data: map[string]any{"sourcePasswordEnv": "bad;touch"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Streams[0].SourceCommand, " "), "HOSTSHIFT_INVALID_ENV") {
		t.Fatalf("expected invalid env to be neutralized, got %+v", plan.Streams[0].SourceCommand)
	}
}

func TestHTTPCheckProducesCurlVerifyAction(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Checks: []profile.Check{{
			Type: "http",
			Name: "homepage",
			Data: map[string]any{
				"url":            "http://127.0.0.1:8080/health",
				"hostHeader":     "example.com",
				"timeoutSeconds": 15,
			},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	action := plan.Actions[len(plan.Actions)-1]
	if action.Phase != "verify" || action.Command[0] != "curl" {
		t.Fatalf("expected curl verify action, got %+v", action)
	}
	joined := strings.Join(action.Command, " ")
	if !strings.Contains(joined, "Host: example.com") || !strings.Contains(joined, "15") {
		t.Fatalf("expected host header and timeout, got %+v", action.Command)
	}
}

func TestLaravelDatabaseCheckProducesTargetDockerExec(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Checks: []profile.Check{{
			Type: "laravelDatabase",
			Name: "Laravel DB",
			Data: map[string]any{"container": "example-app"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	action := plan.Actions[len(plan.Actions)-1]
	if action.HostRole != "target" || action.Impact != "read-only" {
		t.Fatalf("expected target read-only check, got %+v", action)
	}
	if len(action.Command) != 3 || action.Command[0] != "sh" || action.Command[1] != "-lc" {
		t.Fatalf("expected shell-wrapped Laravel DB command, got %+v", action.Command)
	}
	if !strings.Contains(action.Command[2], "docker") || !strings.Contains(action.Command[2], "example-app") || !strings.Contains(action.Command[2], "hostshift-db-ok") {
		t.Fatalf("unexpected Laravel DB command: %+v", action.Command)
	}
	if strings.HasPrefix(action.Command[2], "'") {
		t.Fatalf("shell script argument must not be pre-quoted: %+v", action.Command)
	}
}

func TestFileChecksProduceReadOnlyVerifyActions(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Checks: []profile.Check{
			{Type: "fileExists", Name: "health-file", Data: map[string]any{"path": "/srv/app/public/health"}},
			{Type: "fileContains", Name: "sshd-config", Data: map[string]any{"path": "/etc/ssh/sshd_config.d/99-hostshift.conf", "contains": "ClientAliveInterval 120"}},
		},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Actions) < 4 {
		t.Fatalf("expected verify actions, got %+v", plan.Actions)
	}
	fileExists := plan.Actions[len(plan.Actions)-2]
	fileContains := plan.Actions[len(plan.Actions)-1]
	if fileExists.Command[0] != "test" || fileExists.Command[1] != "-f" {
		t.Fatalf("expected fileExists verify action, got %+v", fileExists.Command)
	}
	if fileContains.Command[0] != "grep" || fileContains.Command[1] != "-Fq" {
		t.Fatalf("expected fileContains verify action, got %+v", fileContains.Command)
	}
	if fileExists.Impact != "read-only" || fileContains.Impact != "read-only" {
		t.Fatalf("file checks must be read-only: %+v %+v", fileExists, fileContains)
	}
}

func TestMySQLScalarCheckProducesReadOnlyTargetAssertion(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Checks: []profile.Check{{
			Type: "mysqlScalar",
			Name: "row-count",
			Data: map[string]any{
				"database": "app",
				"query":    "SELECT COUNT(*) FROM pages",
				"expected": "2",
			},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	action := plan.Actions[len(plan.Actions)-1]
	if action.HostRole != "target" || action.Impact != "read-only" {
		t.Fatalf("expected target read-only check, got %+v", action)
	}
	if action.Command[0] != "sh" || !strings.Contains(action.Command[2], "mysql --batch --skip-column-names") {
		t.Fatalf("unexpected mysql scalar command: %+v", action.Command)
	}
	var packageCommand []string
	for _, item := range plan.Actions {
		if item.ID == "target.prepare.packages" {
			packageCommand = item.Command
			break
		}
	}
	if !strings.Contains(strings.Join(packageCommand, " "), "mysql-client") {
		t.Fatalf("expected mysql-client package capability, got %+v", packageCommand)
	}
}

func TestPostgresScalarCheckProducesReadOnlyTargetAssertion(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Checks: []profile.Check{{
			Type: "postgresScalar",
			Name: "row-count",
			Data: map[string]any{
				"database": "analytics",
				"query":    "SELECT COUNT(*) FROM metrics",
				"expected": "2",
			},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	action := plan.Actions[len(plan.Actions)-1]
	if action.HostRole != "target" || action.Impact != "read-only" {
		t.Fatalf("expected target read-only check, got %+v", action)
	}
	if action.Command[0] != "sh" || !strings.Contains(action.Command[2], "runuser -u postgres -- psql --tuples-only --no-align") {
		t.Fatalf("unexpected postgres scalar command: %+v", action.Command)
	}
	var packageCommand []string
	for _, item := range plan.Actions {
		if item.ID == "target.prepare.packages" {
			packageCommand = item.Command
			break
		}
	}
	packageCommandText := strings.Join(packageCommand, " ")
	if !strings.Contains(packageCommandText, "postgresql ") || !strings.Contains(packageCommandText, "postgresql-client") {
		t.Fatalf("expected postgresql server and client package capabilities, got %+v", packageCommand)
	}
}

func TestNginxConfigCheckProducesPostSyncReloadAction(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Checks:        []profile.Check{{Type: "nginxConfig", Name: "reload"}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	action := plan.Actions[len(plan.Actions)-1]
	if action.Phase != "verify" || action.Impact != "service" || !strings.Contains(action.Command[2], "nginx -t") {
		t.Fatalf("expected nginx prepare reload action, got %+v", action)
	}
}

func TestNginxFileSetDisablesPackagedDefaultSiteOnTarget(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
		Workloads: []profile.Workload{{
			Type: "file-set",
			Name: "nginx-files",
			Data: map[string]any{"paths": []any{"/etc/nginx/sites-available/example.conf", "/etc/nginx/sites-enabled/example.conf"}, "targetPath": "/"},
		}},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		if action.ID == "target.nginx.disable-default-site" {
			if action.Phase != "prepare" || action.HostRole != "target" || strings.Join(action.Command, " ") != "rm -f /etc/nginx/sites-enabled/default" {
				t.Fatalf("unexpected nginx default-site action: %+v", action)
			}
			return
		}
	}
	t.Fatalf("expected nginx default-site action, got %+v", plan.Actions)
}

func TestServiceAndFirewallChecksProduceReadOnlyVerifyActions(t *testing.T) {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		Approved:      true,
		Checks: []profile.Check{
			{Type: "serviceActive", Name: "nginx", Data: map[string]any{"service": "nginx"}},
			{Type: "ufwRule", Name: "mysql-rule", Data: map[string]any{"from": "172.17.0.0/16", "port": 3306, "proto": "tcp"}},
			{Type: "nftRule", Name: "mysql-nft-rule", Data: map[string]any{"family": "inet", "table": "hostshift", "chain": "input", "contains": "tcp dport 3306 accept"}},
		},
	}
	plan, err := Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	service := plan.Actions[len(plan.Actions)-3]
	firewall := plan.Actions[len(plan.Actions)-2]
	nft := plan.Actions[len(plan.Actions)-1]
	if service.Command[0] != "systemctl" || service.Command[3] != "nginx" || service.Impact != "read-only" {
		t.Fatalf("expected serviceActive read-only command, got %+v", service)
	}
	if firewall.Command[0] != "sh" || !strings.Contains(firewall.Command[2], "ufw show added") || firewall.Impact != "read-only" {
		t.Fatalf("expected ufwRule read-only command, got %+v", firewall)
	}
	if nft.Command[0] != "sh" || !strings.Contains(nft.Command[2], "nft list chain 'inet' 'hostshift' 'input'") || nft.Impact != "read-only" {
		t.Fatalf("expected nftRule read-only command, got %+v", nft)
	}
}
