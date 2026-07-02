package source

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/oguzhankaracabay/hostshift/internal/platform"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/safety"
)

type Runner interface {
	Run(context.Context, string, []string) ([]byte, error)
}

type FactSpec struct {
	Name     string
	Command  []string
	Optional bool
}

type FactResult struct {
	OK      bool     `json:"ok" yaml:"ok"`
	Command []string `json:"command" yaml:"command"`
	Value   string   `json:"value,omitempty" yaml:"value,omitempty"`
	Error   string   `json:"error,omitempty" yaml:"error,omitempty"`
}

type ReadOnlySource struct {
	Alias  string
	Runner Runner
}

var Facts = []FactSpec{
	{Name: "osRelease", Command: []string{"cat", "/etc/os-release"}},
	{Name: "architecture", Command: []string{"uname", "-m"}},
	{Name: "hostname", Command: []string{"hostname"}},
	{Name: "disk", Command: []string{"df", "-Pk"}},
	{Name: "memory", Command: []string{"cat", "/proc/meminfo"}},
	{Name: "packages", Command: []string{"dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\n"}},
	{Name: "enabledServices", Command: []string{"systemctl", "list-unit-files", "--state=enabled", "--type=service", "--no-pager", "--no-legend"}, Optional: true},
	{Name: "runningServices", Command: []string{"systemctl", "list-units", "--state=running", "--type=service", "--no-pager", "--no-legend"}, Optional: true},
	{Name: "mounts", Command: []string{"findmnt", "--json", "--real"}},
	{Name: "listeners", Command: []string{"ss", "-lntupH"}, Optional: true},
	{Name: "ufwStatus", Command: []string{"ufw", "status", "verbose"}, Optional: true},
	{Name: "nftRuleset", Command: []string{"nft", "list", "ruleset"}, Optional: true},
	{Name: "sshdEffectiveConfig", Command: []string{"sshd", "-T"}, Optional: true},
	{Name: "sshdConfig", Command: []string{"cat", "/etc/ssh/sshd_config"}, Optional: true},
	{Name: "mysqlServerConfig", Command: []string{"cat", "/etc/mysql/mysql.conf.d/mysqld.cnf"}, Optional: true},
	{Name: "mysqlDatabases", Command: []string{"mysql", "--batch", "--skip-column-names", "--execute=SHOW DATABASES"}, Optional: true},
	{Name: "postgresDatabases", Command: []string{"psql", "--tuples-only", "--no-align", "--command=SELECT datname FROM pg_database WHERE datistemplate = false"}, Optional: true},
	{Name: "nginxConfigDump", Command: []string{"nginx", "-T"}, Optional: true},
	{Name: "apacheConfigDump", Command: []string{"apache2ctl", "-S"}, Optional: true},
	{Name: "letsEncryptFiles", Command: []string{"find", "/etc/letsencrypt", "-maxdepth", "3", "-type", "f", "-print"}, Optional: true},
	{Name: "users", Command: []string{"getent", "passwd"}},
	{Name: "groups", Command: []string{"getent", "group"}},
	{Name: "cron", Command: []string{"find", "/etc/cron.d", "/etc/cron.daily", "/etc/cron.hourly", "/etc/cron.monthly", "/etc/cron.weekly", "-maxdepth", "1", "-type", "f", "-print"}, Optional: true},
	{Name: "dockerVersion", Command: []string{"docker", "version", "--format", "{{json .Server.Version}}"}, Optional: true},
	{Name: "dockerComposeProjects", Command: []string{"docker", "compose", "ls", "--format", "json"}, Optional: true},
	{Name: "dockerContainers", Command: []string{"docker", "ps", "--format", "{{json .}}"}, Optional: true},
	{Name: "dockerNetworks", Command: []string{"docker", "network", "ls", "--format", "{{json .}}"}, Optional: true},
}

func New(alias string, runner Runner) (ReadOnlySource, error) {
	if err := safety.SSHAlias(alias); err != nil {
		return ReadOnlySource{}, err
	}
	if runner == nil {
		return ReadOnlySource{}, fmt.Errorf("source runner is required")
	}
	return ReadOnlySource{Alias: alias, Runner: runner}, nil
}

func FactNames() []string {
	names := make([]string, 0, len(Facts))
	for _, fact := range Facts {
		names = append(names, fact.Name)
	}
	sort.Strings(names)
	return names
}

func FactByName(name string) (FactSpec, bool) {
	for _, fact := range Facts {
		if fact.Name == name {
			return fact, true
		}
	}
	return FactSpec{}, false
}

func (s ReadOnlySource) ReadFact(ctx context.Context, name string) FactResult {
	spec, ok := FactByName(name)
	if !ok {
		return FactResult{OK: false, Error: "fact is not allowlisted"}
	}
	if err := safety.SourceCommand(spec.Command); err != nil {
		return FactResult{OK: false, Command: spec.Command, Error: err.Error()}
	}
	out, err := s.Runner.Run(ctx, s.Alias, spec.Command)
	if err != nil {
		return FactResult{OK: false, Command: spec.Command, Error: err.Error()}
	}
	return FactResult{OK: true, Command: spec.Command, Value: strings.TrimSpace(string(out))}
}

func (s ReadOnlySource) Discover(ctx context.Context) map[string]FactResult {
	results := map[string]FactResult{}
	for _, fact := range Facts {
		result := s.ReadFact(ctx, fact.Name)
		if !result.OK && fact.Optional {
			results[fact.Name] = result
			continue
		}
		results[fact.Name] = result
	}
	return results
}

func ProfileFromFacts(name, alias string, facts map[string]FactResult) profile.Profile {
	prof := profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          name,
		Source:        profile.Host{SSH: alias},
		Target:        profile.Host{},
		SourcePolicy:  "strict-read-only",
		Platforms:     profile.Platforms{},
		Workloads:     []profile.Workload{},
		Approved:      false,
	}
	if osFact := facts["osRelease"]; osFact.OK {
		release := platform.ParseOSRelease(osFact.Value)
		prof.Platforms.Source = release.ID + ":" + release.VersionID
	}
	return prof
}
