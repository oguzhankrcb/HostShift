package planner

import (
	"fmt"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/platform"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

type Plan struct {
	Profile              string              `json:"profile"`
	SourcePolicy         string              `json:"sourcePolicy"`
	SourceWillBeModified bool                `json:"sourceWillBeModified"`
	Actions              []core.Action       `json:"actions"`
	Streams              []core.StreamAction `json:"streams,omitempty"`
	Blockers             []string            `json:"blockers"`
	Warnings             []string            `json:"warnings,omitempty"`
}

func Build(prof profile.Profile, now time.Time) (Plan, error) {
	actions := []core.Action{
		{ID: "source.inventory", Phase: core.PhaseDiscover, HostRole: core.HostRoleSource, Impact: core.ImpactReadOnly, Command: []string{"cat", "/etc/os-release"}},
		{ID: "target.verify.ssh", Phase: core.PhaseVerify, HostRole: core.HostRoleTarget, Impact: core.ImpactReadOnly, Command: []string{"true"}},
	}
	streams := []core.StreamAction{}
	blockers := []string{}
	warnings := []string{}
	if prof.SourcePolicy != "strict-read-only" {
		blockers = append(blockers, "Source policy must be strict-read-only")
	}
	if !prof.Approved {
		blockers = append(blockers, "Profile is not approved")
	}
	if prof.Target.SSH == "" {
		blockers = append(blockers, "Target SSH alias is missing")
	}
	sourceStatus, sourceKnown := supportStatus(prof.Platforms.Source, now)
	targetStatus, targetKnown := supportStatus(prof.Platforms.Target, now)
	if sourceKnown && sourceStatus == platform.SupportEOL {
		warnings = append(warnings, fmt.Sprintf("Source platform %s is EOL; read-only export is allowed but verification must be strict", prof.Platforms.Source))
	}
	if targetKnown && (targetStatus == platform.SupportEOL || targetStatus == platform.SupportUnsupported) {
		blockers = append(blockers, fmt.Sprintf("Target platform %s is not supported", prof.Platforms.Target))
	}
	if prof.Platforms.Source != "" && prof.Platforms.Target != "" && platformID(prof.Platforms.Source) != platformID(prof.Platforms.Target) {
		warnings = append(warnings, fmt.Sprintf("Cross-distribution migration %s -> %s requires workload compatibility checks", prof.Platforms.Source, prof.Platforms.Target))
	}
	capabilities := requiredCapabilities(prof)
	packageAction, packageBlockers := preparePackagesAction(prof.Platforms.Target, capabilities)
	blockers = append(blockers, packageBlockers...)
	if packageAction.ID != "" {
		actions = append(actions, packageAction)
	}
	actions = append(actions, targetConfigurationActions(prof)...)
	for _, workload := range prof.Workloads {
		blockers = append(blockers, blockersForWorkload(workload)...)
		workloadActions, stream, hasStream := actionsForWorkload(workload)
		actions = append(actions, workloadActions...)
		if hasStream {
			streams = append(streams, stream)
		}
	}
	for _, check := range prof.Checks {
		actions = append(actions, actionForCheck(check))
	}
	if err := core.ValidatePlan(actions); err != nil {
		return Plan{}, err
	}
	if err := core.ValidateStreams(streams); err != nil {
		return Plan{}, err
	}
	return Plan{
		Profile:              prof.Name,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: false,
		Actions:              actions,
		Streams:              streams,
		Blockers:             blockers,
		Warnings:             warnings,
	}, nil
}

func requiredCapabilities(prof profile.Profile) []string {
	set := map[string]bool{"rsync": true}
	if firewallUsesUFW(prof.Firewall) {
		set["ufw"] = true
	}
	if len(prof.SSHD.Settings) > 0 {
		set["openssh-server"] = true
	}
	if prof.MySQL.Settings.BindAddress != "" || prof.MySQL.Settings.MySQLXBindAddress != "" {
		set["mysql-server"] = true
	}
	for _, workload := range prof.Workloads {
		switch workload.Type {
		case "docker-compose":
			set["docker-runtime"] = true
			set["docker-compose"] = true
		case "docker-standalone":
			set["docker-runtime"] = true
		case "file-set":
			set["tar"] = true
			for _, item := range dataStringSlice(workload.Data, "paths", "Paths") {
				if item == "/etc/nginx" || strings.HasPrefix(item, "/etc/nginx/") {
					set["nginx"] = true
				}
				if item == "/etc/apache2" || strings.HasPrefix(item, "/etc/apache2/") {
					set["apache"] = true
				}
				if item == "/etc/cron.d" || strings.HasPrefix(item, "/etc/cron.d/") ||
					item == "/etc/cron.daily" || strings.HasPrefix(item, "/etc/cron.daily/") ||
					item == "/etc/cron.hourly" || strings.HasPrefix(item, "/etc/cron.hourly/") ||
					item == "/etc/cron.monthly" || strings.HasPrefix(item, "/etc/cron.monthly/") ||
					item == "/etc/cron.weekly" || strings.HasPrefix(item, "/etc/cron.weekly/") {
					set["cron"] = true
				}
			}
		case "apache-vhost":
			set["apache"] = true
		case "cron":
			set["cron"] = true
		case "mysql":
			set["mysql-client"] = true
		case "mariadb":
			set["mariadb-client"] = true
		case "postgresql":
			set["postgresql-server"] = true
			set["postgresql-client"] = true
		case "redis":
			set["redis-server"] = true
			set["redis-tools"] = true
		}
	}
	for _, check := range prof.Checks {
		switch check.Type {
		case "http":
			set["curl"] = true
		case "laravelDatabase":
			set["docker-runtime"] = true
		case "mysqlScalar":
			set["mysql-client"] = true
		case "postgresScalar":
			set["postgresql-server"] = true
			set["postgresql-client"] = true
		}
	}
	out := []string{}
	for _, capability := range []string{"rsync", "tar", "curl", "ufw", "openssh-server", "nginx", "apache", "cron", "docker-runtime", "docker-compose", "mysql-server", "mysql-client", "mariadb-client", "postgresql-server", "postgresql-client", "redis-server", "redis-tools"} {
		if set[capability] {
			out = append(out, capability)
		}
	}
	return out
}

func blockersForWorkload(workload profile.Workload) []string {
	if workload.Type != "redis" {
		return nil
	}
	hasSnapshot := dataString(workload.Data, "snapshotPath", "SnapshotPath") != ""
	hasReplica := dataString(workload.Data, "replicaHost", "ReplicaHost") != ""
	if hasSnapshot && hasReplica {
		return []string{fmt.Sprintf("Redis workload %s must choose either snapshotPath or replicaHost, not both", workload.Name)}
	}
	if !hasSnapshot && !hasReplica {
		return []string{fmt.Sprintf("Redis workload %s requires snapshotPath or replicaHost for source read-only export", workload.Name)}
	}
	return nil
}

func targetConfigurationActions(prof profile.Profile) []core.Action {
	actions := []core.Action{}
	actions = append(actions, firewallActions(prof.Firewall)...)
	if action := sshdAction(prof.SSHD); action.ID != "" {
		actions = append(actions, action)
	}
	if action := mysqlAction(prof.MySQL); action.ID != "" {
		actions = append(actions, action)
	}
	if migratesNginxConfig(prof.Workloads) {
		actions = append(actions, core.Action{
			ID:       "target.nginx.disable-default-site",
			Phase:    core.PhasePrepare,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactWrite,
			Command:  []string{"rm", "-f", "/etc/nginx/sites-enabled/default"},
			Rollback: []string{"test ! -e /etc/nginx/sites-available/default || ln -sf /etc/nginx/sites-available/default /etc/nginx/sites-enabled/default"},
		})
	}
	if migratesApacheConfig(prof.Workloads) {
		actions = append(actions, core.Action{
			ID:            "target.apache.disable-default-site",
			Phase:         core.PhasePrepare,
			HostRole:      core.HostRoleTarget,
			Impact:        core.ImpactService,
			Command:       []string{"sh", "-lc", "a2dissite 000-default.conf || true"},
			Preconditions: []string{"Apache is installed on target"},
			Rollback:      []string{"a2ensite 000-default.conf || true"},
		})
	}
	return actions
}

func migratesNginxConfig(workloads []profile.Workload) bool {
	for _, workload := range workloads {
		if workload.Type != "file-set" {
			continue
		}
		for _, item := range dataStringSlice(workload.Data, "paths", "Paths") {
			if item == "/etc/nginx" || strings.HasPrefix(item, "/etc/nginx/") {
				return true
			}
		}
	}
	return false
}

func migratesApacheConfig(workloads []profile.Workload) bool {
	for _, workload := range workloads {
		if workload.Type == "apache-vhost" {
			return true
		}
		if workload.Type != "file-set" {
			continue
		}
		for _, item := range dataStringSlice(workload.Data, "paths", "Paths") {
			if item == "/etc/apache2" || strings.HasPrefix(item, "/etc/apache2/") {
				return true
			}
		}
	}
	return false
}

func firewallUsesUFW(firewall profile.Firewall) bool {
	if firewall.Enabled != nil && !*firewall.Enabled {
		return false
	}
	return len(firewall.Rules) > 0 || firewall.Enable
}

func firewallActions(firewall profile.Firewall) []core.Action {
	if !firewallUsesUFW(firewall) {
		return nil
	}
	actions := []core.Action{}
	for index, rule := range firewall.Rules {
		actions = append(actions, core.Action{
			ID:       fmt.Sprintf("target.firewall.ufw.allow.%d", index+1),
			Phase:    core.PhasePrepare,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactNetwork,
			Command:  []string{"ufw", "allow", "from", rule.From, "to", "any", "port", strconv.Itoa(rule.Port), "proto", rule.Proto},
			Rollback: []string{"ufw delete allow from " + rule.From + " to any port " + strconv.Itoa(rule.Port) + " proto " + rule.Proto},
		})
	}
	if firewall.Enable {
		actions = append(actions, core.Action{
			ID:       "target.firewall.ufw.enable",
			Phase:    core.PhasePrepare,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactNetwork,
			Command:  []string{"ufw", "--force", "enable"},
			Rollback: []string{"ufw disable"},
		})
	}
	return actions
}

func sshdAction(sshd profile.SSHD) core.Action {
	if len(sshd.Settings) == 0 {
		return core.Action{}
	}
	lines := []string{}
	for _, key := range []string{"ClientAliveInterval", "ClientAliveCountMax"} {
		if value, ok := sshd.Settings[key]; ok {
			lines = append(lines, fmt.Sprintf("%s %d", key, value))
		}
	}
	if len(lines) == 0 {
		return core.Action{}
	}
	script := "install -d -m 755 /etc/ssh/sshd_config.d && " +
		printfLinesCommand(lines) + " > /etc/ssh/sshd_config.d/99-hostshift.conf && " +
		"sshd -t && (systemctl reload ssh || systemctl reload sshd)"
	return core.Action{
		ID:            "target.sshd.settings",
		Phase:         core.PhasePrepare,
		HostRole:      core.HostRoleTarget,
		Impact:        core.ImpactService,
		Command:       []string{"sh", "-lc", script},
		Preconditions: []string{"OpenSSH server is installed on target"},
		Rollback:      []string{"rm -f /etc/ssh/sshd_config.d/99-hostshift.conf && sshd -t && (systemctl reload ssh || systemctl reload sshd)"},
	}
}

func mysqlAction(mysql profile.MySQL) core.Action {
	settings := mysql.Settings
	if settings.BindAddress == "" && settings.MySQLXBindAddress == "" {
		return core.Action{}
	}
	lines := []string{"[mysqld]"}
	if settings.BindAddress != "" {
		lines = append(lines, "bind-address = "+settings.BindAddress)
	}
	if settings.MySQLXBindAddress != "" {
		lines = append(lines, "mysqlx-bind-address = "+settings.MySQLXBindAddress)
	}
	script := "install -d -m 755 /etc/mysql/mysql.conf.d && " +
		printfLinesCommand(lines) + " > /etc/mysql/mysql.conf.d/99-hostshift-bind.cnf && " +
		"(systemctl reload mysql || systemctl restart mysql)"
	return core.Action{
		ID:            "target.mysql.bind-settings",
		Phase:         core.PhasePrepare,
		HostRole:      core.HostRoleTarget,
		Impact:        core.ImpactService,
		Command:       []string{"sh", "-lc", script},
		Preconditions: []string{"MySQL server is installed on target"},
		Rollback:      []string{"rm -f /etc/mysql/mysql.conf.d/99-hostshift-bind.cnf && (systemctl reload mysql || systemctl restart mysql)"},
	}
}

func printfLinesCommand(lines []string) string {
	args := []string{"printf", shellQuote("%s\\n")}
	for _, line := range lines {
		args = append(args, shellQuote(line))
	}
	return strings.Join(args, " ")
}

func preparePackagesAction(targetPlatform string, capabilities []string) (core.Action, []string) {
	adapter, ok := adapterForPlatform(targetPlatform)
	if !ok {
		return core.Action{}, []string{"Target platform is unknown; package capabilities could not be mapped to distribution packages"}
	}
	packages := []string{}
	blockers := []string{}
	for _, capability := range capabilities {
		pkg, ok := adapter.PackageFor(capability)
		if !ok {
			blockers = append(blockers, fmt.Sprintf("Target platform %s has no package mapping for capability %s", targetPlatform, capability))
			continue
		}
		if !contains(packages, pkg) {
			packages = append(packages, pkg)
		}
	}
	if len(packages) == 0 {
		return core.Action{}, blockers
	}
	command := []string{adapter.PackageManager() + "-get", "install", "-y"}
	if adapter.PackageManager() == "apt" {
		command = []string{"apt-get", "install", "-y"}
	}
	command = append(command, packages...)
	return core.Action{ID: "target.prepare.packages", Phase: core.PhasePrepare, HostRole: core.HostRoleTarget, Impact: core.ImpactWrite, Command: command}, blockers
}

func adapterForPlatform(value string) (platform.Adapter, bool) {
	id, version, ok := splitPlatform(value)
	if !ok {
		return nil, false
	}
	adapter, err := platform.Detect(platform.OSRelease{ID: id, VersionID: version})
	if err != nil {
		return nil, false
	}
	return adapter, true
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func actionForCheck(check profile.Check) core.Action {
	name := check.Name
	if name == "" {
		name = check.Type
	}
	id := "target.check." + strings.ReplaceAll(check.Type+"."+name, " ", "-")
	switch check.Type {
	case "http":
		timeout := dataInt(check.Data, "timeoutSeconds", "TimeoutSeconds")
		if timeout == 0 {
			timeout = 10
		}
		command := []string{"curl", "--fail", "--silent", "--show-error", "--max-time", strconv.Itoa(timeout)}
		if hostHeader := dataString(check.Data, "hostHeader", "HostHeader"); hostHeader != "" {
			command = append(command, "--header", "Host: "+hostHeader)
		}
		command = append(command, dataString(check.Data, "url", "URL"))
		return core.Action{ID: id, Phase: core.PhaseVerify, HostRole: core.HostRoleTarget, Impact: core.ImpactReadOnly, Command: command}
	case "laravelDatabase":
		container := dataString(check.Data, "container", "Container")
		script := "docker exec " + shellQuote(container) +
			" php artisan tinker " +
			shellQuote("--execute=DB::connection()->getPdo(); echo 'hostshift-db-ok';")
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"sh", "-lc", script},
		}
	case "fileExists":
		filePath := dataString(check.Data, "path", "Path")
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"test", "-f", filePath},
		}
	case "fileContains":
		filePath := dataString(check.Data, "path", "Path")
		needle := dataString(check.Data, "contains", "Contains")
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"grep", "-Fq", "--", needle, filePath},
		}
	case "mysqlScalar":
		database := dataString(check.Data, "database", "Database")
		query := dataString(check.Data, "query", "Query")
		expected := dataString(check.Data, "expected", "Expected")
		script := "test \"$(mysql --batch --skip-column-names --database=" + shellQuote(database) + " --execute=" + shellQuote(query) + ")\" = " + shellQuote(expected)
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"sh", "-lc", script},
		}
	case "postgresScalar":
		database := dataString(check.Data, "database", "Database")
		query := dataString(check.Data, "query", "Query")
		expected := dataString(check.Data, "expected", "Expected")
		script := "test \"$(runuser -u postgres -- psql --tuples-only --no-align --dbname=" + shellQuote(database) + " --command=" + shellQuote(query) + ")\" = " + shellQuote(expected)
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"sh", "-lc", script},
		}
	case "serviceActive":
		service := dataString(check.Data, "service", "Service")
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"systemctl", "is-active", "--quiet", service},
		}
	case "ufwRule":
		from := dataString(check.Data, "from", "From")
		port := dataInt(check.Data, "port", "Port")
		proto := dataString(check.Data, "proto", "Proto")
		needle := fmt.Sprintf("ufw allow from %s to any port %d proto %s", from, port, proto)
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"sh", "-lc", "ufw show added | grep -Fq -- " + shellQuote(needle)},
		}
	case "nftRule":
		family := dataString(check.Data, "family", "Family")
		table := dataString(check.Data, "table", "Table")
		chain := dataString(check.Data, "chain", "Chain")
		contains := dataString(check.Data, "contains", "Contains")
		script := "nft list chain " + shellQuote(family) + " " + shellQuote(table) + " " + shellQuote(chain) + " | grep -Fq -- " + shellQuote(contains)
		return core.Action{
			ID:       id,
			Phase:    core.PhaseVerify,
			HostRole: core.HostRoleTarget,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"sh", "-lc", script},
		}
	case "nginxConfig":
		return core.Action{
			ID:            id,
			Phase:         core.PhaseVerify,
			HostRole:      core.HostRoleTarget,
			Impact:        core.ImpactService,
			Command:       []string{"sh", "-lc", "nginx -t && (systemctl reload nginx || systemctl restart nginx)"},
			Preconditions: []string{"Nginx config files are present on target"},
			Rollback:      []string{"systemctl reload nginx || true"},
		}
	default:
		return core.Action{
			ID:       id,
			Phase:    core.PhasePlan,
			HostRole: core.HostRoleLocal,
			Impact:   core.ImpactReadOnly,
			Command:  []string{"hostshift", "inspect-check", check.Type},
		}
	}
}

func actionsForWorkload(workload profile.Workload) ([]core.Action, core.StreamAction, bool) {
	id := "target.workload." + strings.ReplaceAll(workload.Type+"."+workload.Name, " ", "-")
	switch workload.Type {
	case "docker-compose":
		workingDir := dataString(workload.Data, "workingDir", "WorkingDir")
		configFile := dataString(workload.Data, "configFile", "ConfigFile")
		prepareCommand := composeCommand(workingDir, configFile, "config")
		cutoverCommand := composeCommand(workingDir, configFile, "up", "-d", "--build")
		return []core.Action{
			{
				ID:       id,
				Phase:    core.PhasePrepare,
				HostRole: core.HostRoleTarget,
				Impact:   core.ImpactWrite,
				Command:  prepareCommand,
			},
			{
				ID:       id + ".up",
				Phase:    core.PhaseCutover,
				HostRole: core.HostRoleTarget,
				Impact:   core.ImpactService,
				Command:  cutoverCommand,
				Rollback: []string{strings.Join(composeCommand(workingDir, configFile, "down"), " ")},
			},
		}, core.StreamAction{}, false
	case "docker-standalone":
		image := dataString(workload.Data, "image", "Image")
		if image == "" {
			image = workload.Name
		}
		return []core.Action{{
				ID:       id + ".run",
				Phase:    core.PhaseCutover,
				HostRole: core.HostRoleTarget,
				Impact:   core.ImpactService,
				Command:  []string{"sh", "-lc", standaloneRunScript(workload, image)},
				Rollback: []string{"docker stop " + shellQuote(workload.Name) + " || true"},
			}}, core.StreamAction{
				ID:            id + ".image",
				Phase:         core.PhaseSync,
				SourceCommand: []string{"docker", "image", "save", image},
				TargetCommand: []string{"docker", "image", "load"},
			}, true
	case "file-set":
		paths := dataStringSlice(workload.Data, "paths", "Paths")
		targetPath := dataString(workload.Data, "targetPath", "TargetPath")
		if targetPath == "" {
			targetPath = "/"
		}
		sourceCommand := []string{"tar", "--create", "--file=-", "--one-file-system", "--warning=no-file-changed", "-C", "/"}
		for _, item := range paths {
			sourceCommand = append(sourceCommand, strings.TrimPrefix(item, "/"))
		}
		return nil, core.StreamAction{
			ID:            id + ".tar",
			Phase:         core.PhaseSync,
			SourceCommand: sourceCommand,
			TargetCommand: []string{"tar", "--extract", "--file=-", "-C", targetPath},
		}, true
	case "mysql", "mariadb":
		passwordArg := ""
		if sourceEnv := dataString(workload.Data, "sourcePasswordEnv", "SourcePasswordEnv"); sourceEnv != "" {
			passwordArg = "--password=${" + safeEnvName(sourceEnv) + "} "
		}
		dumpBase := "mysqldump --single-transaction --quick --skip-lock-tables "
		databaseArg := "--databases " + shellQuote(workload.Name)
		sourceScript := "if mysqldump --help | grep -q -- '--no-tablespaces'; then exec " + dumpBase + "--no-tablespaces " + passwordArg + databaseArg + "; else exec " + dumpBase + passwordArg + databaseArg + "; fi"
		sourceCommand := []string{"sh", "-lc", sourceScript}
		mysqlCommand := "mysql"
		if targetEnv := dataString(workload.Data, "targetPasswordEnv", "TargetPasswordEnv"); targetEnv != "" {
			mysqlCommand = "mysql --password=${" + safeEnvName(targetEnv) + "}"
		}
		targetCommand := []string{"sh", "-lc", mysqlDumpCompatibilityFilter() + " | " + mysqlCommand}
		return nil, core.StreamAction{
			ID:            id + ".dump",
			Phase:         core.PhaseSync,
			SourceCommand: sourceCommand,
			TargetCommand: targetCommand,
		}, true
	case "postgresql":
		sourceCommand := []string{"pg_dump", "--format=custom", "--dbname", workload.Name}
		if sourceEnv := dataString(workload.Data, "sourcePasswordEnv", "SourcePasswordEnv"); sourceEnv != "" {
			sourceCommand = []string{"env", "PGPASSWORD=${" + safeEnvName(sourceEnv) + "}", "pg_dump", "--format=custom", "--dbname", workload.Name}
		}
		database := shellQuote(workload.Name)
		ensureDatabase := "runuser -u postgres -- psql --tuples-only --no-align --command " + shellQuote("SELECT 1 FROM pg_database WHERE datname='"+workload.Name+"'") + " postgres | grep -qx 1 || runuser -u postgres -- createdb " + database
		restore := "exec runuser -u postgres -- pg_restore --clean --if-exists --no-owner --no-acl --dbname " + database
		targetCommand := []string{"sh", "-lc", ensureDatabase + "; " + restore}
		if targetEnv := dataString(workload.Data, "targetPasswordEnv", "TargetPasswordEnv"); targetEnv != "" {
			targetCommand = []string{"env", "PGPASSWORD=${" + safeEnvName(targetEnv) + "}", "pg_restore", "--clean", "--if-exists", "--no-owner", "--no-acl", "--dbname", workload.Name}
		}
		return nil, core.StreamAction{
			ID:            id + ".dump",
			Phase:         core.PhaseSync,
			SourceCommand: sourceCommand,
			TargetCommand: targetCommand,
		}, true
	case "redis":
		actions, stream, ok := redisWorkloadActions(id, workload)
		return actions, stream, ok
	case "apache-vhost":
		return []core.Action{{
			ID:            id + ".activate",
			Phase:         core.PhaseVerify,
			HostRole:      core.HostRoleTarget,
			Impact:        core.ImpactService,
			Command:       []string{"sh", "-lc", apacheActivationScript(workload)},
			Preconditions: []string{"Apache config files are present on target"},
			Rollback:      apacheRollback(workload),
		}}, core.StreamAction{}, false
	case "systemd-service":
		service := dataString(workload.Data, "service", "Service")
		if service == "" {
			service = workload.Name
		}
		return []core.Action{{
			ID:            id + ".start",
			Phase:         core.PhaseCutover,
			HostRole:      core.HostRoleTarget,
			Impact:        core.ImpactService,
			Command:       []string{"sh", "-lc", "systemctl daemon-reload && systemctl enable --now " + shellQuote(service)},
			Preconditions: []string{"Systemd unit and application files are present on target"},
			Rollback:      []string{"systemctl disable --now " + shellQuote(service) + " || true"},
		}}, core.StreamAction{}, false
	case "cron":
		service := dataString(workload.Data, "service", "Service")
		if service == "" {
			return []core.Action{{
				ID:            id + ".reload",
				Phase:         core.PhaseCutover,
				HostRole:      core.HostRoleTarget,
				Impact:        core.ImpactService,
				Command:       []string{"sh", "-lc", "(systemctl reload cron || systemctl restart cron || systemctl reload crond || systemctl restart crond)"},
				Preconditions: []string{"Cron package is installed and cron files have been synced to target"},
				Rollback:      []string{"systemctl reload cron || systemctl reload crond || true"},
			}}, core.StreamAction{}, false
		}
		quoted := shellQuote(service)
		return []core.Action{{
			ID:            id + ".reload",
			Phase:         core.PhaseCutover,
			HostRole:      core.HostRoleTarget,
			Impact:        core.ImpactService,
			Command:       []string{"sh", "-lc", "systemctl reload " + quoted + " || systemctl restart " + quoted},
			Preconditions: []string{"Cron package is installed and cron files have been synced to target"},
			Rollback:      []string{"systemctl reload " + quoted + " || true"},
		}}, core.StreamAction{}, false
	default:
		return []core.Action{{ID: id, Phase: core.PhasePlan, HostRole: core.HostRoleLocal, Impact: core.ImpactReadOnly, Command: []string{"hostshift", "inspect-workload", workload.Type}}}, core.StreamAction{}, false
	}
}

func redisWorkloadActions(id string, workload profile.Workload) ([]core.Action, core.StreamAction, bool) {
	snapshotPath := dataString(workload.Data, "snapshotPath", "SnapshotPath")
	replicaHost := dataString(workload.Data, "replicaHost", "ReplicaHost")
	if (snapshotPath == "" && replicaHost == "") || (snapshotPath != "" && replicaHost != "") {
		return nil, core.StreamAction{}, false
	}
	targetPath := dataString(workload.Data, "targetPath", "TargetPath")
	if targetPath == "" {
		targetPath = "/var/lib/redis/dump.rdb"
	}
	targetScript := "install -d -m 755 " + shellQuote(path.Dir(targetPath)) + " && cat > " + shellQuote(targetPath)
	stream := core.StreamAction{
		ID:            id + ".rdb",
		Phase:         core.PhaseSync,
		TargetCommand: []string{"sh", "-lc", targetScript},
		Preconditions: []string{"Target Redis is installed; source uses an existing snapshot or read-only replica stream"},
		Rollback:      []string{"rm -f " + shellQuote(targetPath)},
	}
	if snapshotPath != "" {
		stream.SourceCommand = []string{"cat", snapshotPath}
	} else {
		port := dataInt(workload.Data, "replicaPort", "ReplicaPort")
		if port == 0 {
			port = 6379
		}
		stream.SourceCommand = []string{"redis-cli", "-h", replicaHost, "-p", strconv.Itoa(port), "--rdb", "-"}
	}
	return []core.Action{{
		ID:            id + ".restart",
		Phase:         core.PhaseCutover,
		HostRole:      core.HostRoleTarget,
		Impact:        core.ImpactService,
		Command:       []string{"sh", "-lc", "systemctl restart redis-server || systemctl restart redis"},
		Preconditions: []string{"Redis RDB file has been streamed to the target path"},
		Rollback:      []string{"systemctl restart redis-server || systemctl restart redis || true"},
	}}, stream, true
}

func apacheActivationScript(workload profile.Workload) string {
	parts := []string{}
	for _, module := range dataStringSlice(workload.Data, "modules", "Modules") {
		parts = append(parts, "a2enmod "+shellQuote(module))
	}
	for _, site := range dataStringSlice(workload.Data, "sites", "Sites") {
		parts = append(parts, "a2ensite "+shellQuote(site))
	}
	parts = append(parts, "apache2ctl configtest", "(systemctl reload apache2 || systemctl restart apache2)")
	return strings.Join(parts, " && ")
}

func apacheRollback(workload profile.Workload) []string {
	parts := []string{}
	for _, site := range dataStringSlice(workload.Data, "sites", "Sites") {
		parts = append(parts, "a2dissite "+shellQuote(site))
	}
	parts = append(parts, "systemctl reload apache2 || true")
	return []string{strings.Join(parts, " && ")}
}

func mysqlDumpCompatibilityFilter() string {
	return "sed -e " +
		shellQuote("s/utf8mb4_0900_ai_ci/utf8mb4_unicode_ci/g") +
		" -e " +
		shellQuote("s/ \\/\\*!80016 DEFAULT ENCRYPTION='N' \\*\\///g")
}

func composeCommand(workingDir, configFile string, args ...string) []string {
	command := []string{"docker", "compose"}
	if configFile != "" {
		command = append(command, "-f", configFile)
	}
	command = append(command, args...)
	if workingDir == "" {
		return command
	}
	return []string{"sh", "-lc", "cd " + shellQuote(workingDir) + " && " + joinShell(command)}
}

func standaloneRunScript(workload profile.Workload, image string) string {
	name := workload.Name
	restartPolicy := dataString(workload.Data, "restartPolicy", "RestartPolicy")
	if restartPolicy == "" {
		restartPolicy = "no"
	}
	args := []string{"docker", "run", "-d", "--name", shellQuote(name), "--restart", shellQuote(restartPolicy)}
	for containerPort, bindings := range dataPortBindings(workload.Data) {
		for _, binding := range bindings {
			hostPort := binding["HostPort"]
			if hostPort == "" {
				continue
			}
			hostIP := binding["HostIp"]
			published := hostPort + ":" + containerPort
			if hostIP != "" && hostIP != "0.0.0.0" && hostIP != "::" {
				published = hostIP + ":" + published
			}
			args = append(args, "-p", shellQuote(published))
		}
	}
	for key, value := range dataStringMap(workload.Data, "safeEnvironment", "SafeEnvironment") {
		args = append(args, "-e", shellQuote(key+"="+value))
	}
	if user := dataString(workload.Data, "user", "User"); user != "" {
		args = append(args, "--user", shellQuote(user))
	}
	if workingDir := dataString(workload.Data, "workingDir", "WorkingDir"); workingDir != "" {
		args = append(args, "--workdir", shellQuote(workingDir))
	}
	args = append(args, shellQuote(image))
	return "if docker inspect " + shellQuote(name) + " >/dev/null 2>&1; then docker start " + shellQuote(name) + "; else " + strings.Join(args, " ") + "; fi"
}

func dataStringSlice(data any, keys ...string) []string {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			raw, ok := item[key]
			if !ok {
				continue
			}
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
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		for _, key := range keys {
			field := value.FieldByName(key)
			if field.IsValid() && field.Kind() == reflect.Slice {
				out := make([]string, 0, field.Len())
				for index := 0; index < field.Len(); index++ {
					if field.Index(index).Kind() == reflect.String {
						out = append(out, field.Index(index).String())
					}
				}
				return out
			}
		}
	}
	return nil
}

func dataStringMap(data any, keys ...string) map[string]string {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			raw, ok := item[key]
			if !ok {
				continue
			}
			out := map[string]string{}
			switch values := raw.(type) {
			case map[string]string:
				return values
			case map[string]any:
				for k, v := range values {
					if str, ok := v.(string); ok {
						out[k] = str
					}
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return nil
}

func dataPortBindings(data any) map[string][]map[string]string {
	item, ok := data.(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := item["portBindings"]
	if !ok {
		raw = item["PortBindings"]
	}
	values, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string][]map[string]string{}
	for containerPort, rawBindings := range values {
		list, ok := rawBindings.([]any)
		if !ok {
			continue
		}
		for _, rawBinding := range list {
			binding, ok := rawBinding.(map[string]any)
			if !ok {
				continue
			}
			normalized := map[string]string{}
			for _, key := range []string{"HostIp", "HostPort"} {
				if value, ok := binding[key].(string); ok {
					normalized[key] = value
				}
			}
			out[containerPort] = append(out[containerPort], normalized)
		}
	}
	return out
}

var envNamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

func safeEnvName(value string) string {
	if !envNamePattern.MatchString(value) {
		return "HOSTSHIFT_INVALID_ENV"
	}
	return value
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func joinShell(args []string) string {
	quoted := make([]string, len(args))
	for index, arg := range args {
		quoted[index] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func dataString(data any, keys ...string) string {
	if data == nil {
		return ""
	}
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			if value, ok := item[key].(string); ok {
				return value
			}
		}
	}
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		for _, key := range keys {
			field := value.FieldByName(key)
			if field.IsValid() && field.Kind() == reflect.String {
				return field.String()
			}
		}
	}
	return ""
}

func dataInt(data any, keys ...string) int {
	if item, ok := data.(map[string]any); ok {
		for _, key := range keys {
			raw, ok := item[key]
			if !ok {
				continue
			}
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
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	if value.IsValid() && value.Kind() == reflect.Struct {
		for _, key := range keys {
			field := value.FieldByName(key)
			if field.IsValid() && field.CanInt() {
				return int(field.Int())
			}
		}
	}
	return 0
}

func supportStatus(value string, now time.Time) (platform.SupportStatus, bool) {
	id, version, ok := splitPlatform(value)
	if !ok {
		return platform.SupportUnsupported, false
	}
	release := platform.OSRelease{ID: id, VersionID: version}
	adapter, err := platform.Detect(release)
	if err != nil {
		return platform.SupportUnsupported, true
	}
	return adapter.Support(release, now), true
}

func platformID(value string) string {
	id, _, ok := splitPlatform(value)
	if !ok {
		return ""
	}
	return id
}

func splitPlatform(value string) (string, string, bool) {
	id, version, ok := strings.Cut(value, ":")
	if !ok || id == "" || version == "" {
		return "", "", false
	}
	return id, version, true
}
