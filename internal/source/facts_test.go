package source

import (
	"context"
	"slices"
	"testing"

	"github.com/oguzhankaracabay/hostshift/internal/safety"
)

type fakeRunner struct {
	commands [][]string
	outputs  map[string][]byte
}

func (f *fakeRunner) Run(_ context.Context, _ string, command []string) ([]byte, error) {
	f.commands = append(f.commands, command)
	if out, ok := f.outputs[command[0]]; ok {
		return out, nil
	}
	return []byte("ok\n"), nil
}

func TestFactNamesExposeAllowlist(t *testing.T) {
	names := FactNames()
	for _, expected := range []string{"osRelease", "dockerComposeProjects", "dockerVolumes", "nftRuleset", "customSystemdUnits"} {
		if !slices.Contains(names, expected) {
			t.Fatalf("expected %s in fact allowlist", expected)
		}
	}
	if _, ok := FactByName("touchTmp"); ok {
		t.Fatal("unexpected mutating fact in allowlist")
	}
}

func TestFactCommandsAreSourceSafe(t *testing.T) {
	for _, fact := range Facts {
		if err := safety.SourceCommand(fact.Command); err != nil {
			t.Fatalf("fact %s is not source-safe: %v", fact.Name, err)
		}
	}
}

func TestDiscoverUsesReadOnlyCommands(t *testing.T) {
	runner := &fakeRunner{outputs: map[string][]byte{
		"cat": []byte("ID=ubuntu\nVERSION_ID=\"24.04\"\n"),
	}}
	client, err := New("source-host", runner)
	if err != nil {
		t.Fatal(err)
	}
	facts := client.Discover(context.Background())
	if !facts["osRelease"].OK {
		t.Fatalf("expected osRelease fact to be ok: %+v", facts["osRelease"])
	}
	prof := ProfileFromFacts("example", "source-host", facts)
	if prof.SourcePolicy != "strict-read-only" {
		t.Fatalf("unexpected source policy: %s", prof.SourcePolicy)
	}
	if prof.Platforms.Source != "ubuntu:24.04" {
		t.Fatalf("unexpected source platform: %s", prof.Platforms.Source)
	}
}

func TestProfileFromFactsSuggestsSafeWorkloads(t *testing.T) {
	facts := map[string]FactResult{
		"osRelease": {OK: true, Value: "ID=ubuntu\nVERSION_ID=\"24.04\"\n"},
		"dockerComposeProjects": {
			OK:    true,
			Value: `[{"Name":"web","ConfigFiles":"/srv/web/docker-compose.yml"}]`,
		},
		"dockerContainers": {
			OK: true,
			Value: `{"Names":"redis-cache","Image":"redis:7","Labels":""}` + "\n" +
				`{"Names":"web-app-1","Image":"example/web:latest","Labels":"com.docker.compose.project=web"}`,
		},
		"dockerVolumes": {
			OK: true,
			Value: `{"Name":"web_uploads","Driver":"local"}` + "\n" +
				`{"Name":"shared-media","Driver":"nfs"}`,
		},
		"mysqlDatabases":        {OK: true, Value: "information_schema\napp\nmysql\n"},
		"postgresDatabases":     {OK: true, Value: "postgres\nanalytics\ntemplate1\n"},
		"nginxConfigDump":       {OK: true, Value: "nginx: configuration file /etc/nginx/nginx.conf test is successful"},
		"letsEncryptFiles":      {OK: true, Value: "/etc/letsencrypt/live/example.com/fullchain.pem"},
		"apacheConfigDump":      {OK: true, Value: "VirtualHost configuration:\n*:80 example.conf"},
		"dockerVersion":         {OK: true, Value: `"25.0.0"`},
		"dockerNetworks":        {OK: true, Value: "{}"},
		"packages":              {OK: true, Value: "redis-server\t7.0\nnginx\t1.24\ncaddy\t2.6\nphp8.3-fpm\t8.3\nsupervisor\t4.2\nfail2ban\t1.0\nmemcached\t1.6\nrabbitmq-server\t3.12\ncertbot\t2.9\nlogrotate\t3.21\n"},
		"enabledServices":       {OK: true, Value: "nginx.service enabled\ncaddy.service enabled\nportfolio.service enabled\nredis-server.service enabled\nphp8.3-fpm.service enabled\nsupervisor.service enabled\nfail2ban.service enabled\nmemcached.service enabled\nrabbitmq-server.service enabled"},
		"runningServices":       {OK: true, Value: "nginx.service loaded active running\ncaddy.service loaded active running\nredis-server.service loaded active running\nphp8.3-fpm.service loaded active running\nsupervisor.service loaded active running\nfail2ban.service loaded active running\nmemcached.service loaded active running\nrabbitmq-server.service loaded active running"},
		"listeners":             {OK: true, Value: "LISTEN 0 4096 *:80"},
		"ufwStatus":             {OK: true, Value: "Status: active"},
		"nftRuleset":            {OK: true, Value: "table inet filter {}"},
		"sshdEffectiveConfig":   {OK: true, Value: "port 22"},
		"caddyConfigPaths":      {OK: true, Value: "/etc/caddy/Caddyfile\n/etc/caddy/sites-enabled/app.caddy"},
		"phpConfigPaths":        {OK: true, Value: "/etc/php/8.3/fpm/php.ini\n/etc/php/8.3/fpm/pool.d/www.conf"},
		"supervisorConfigPaths": {OK: true, Value: "/etc/supervisor/supervisord.conf\n/etc/supervisor/conf.d/worker.conf"},
		"fail2banConfigPaths":   {OK: true, Value: "/etc/fail2ban/jail.local\n/etc/fail2ban/filter.d/nginx.conf"},
		"memcachedConfigPaths":  {OK: true, Value: "/etc/memcached.conf\n/etc/memcached/conf.d/local.conf"},
		"rabbitmqConfigPaths":   {OK: true, Value: "/etc/rabbitmq/rabbitmq.conf\n/etc/rabbitmq/enabled_plugins"},
		"logrotateConfigPaths":  {OK: true, Value: "/etc/logrotate.conf\n/etc/logrotate.d/nginx\n/etc/logrotate.d/mysql-server"},
		"cron":                  {OK: true, Value: "/etc/cron.d/app\n/etc/cron.daily/backup"},
		"customSystemdUnits":    {OK: true, Value: "/etc/systemd/system/portfolio.service\n/etc/systemd/system/multi-user.target.wants/ignored.service"},
	}
	prof := ProfileFromFacts("example", "source-host", facts)
	want := map[string]bool{
		"docker-compose:web":            true,
		"file-set:srv-web":              true,
		"docker-standalone:redis-cache": true,
		"docker-volume:web_uploads":     true,
		"docker-volume:shared-media":    true,
		"file-set:nginx-config":         true,
		"file-set:apache-config":        true,
		"apache-vhost:apache2":          true,
		"file-set:caddy-config":         true,
		"caddy:caddy":                   true,
		"file-set:php-config":           true,
		"php-fpm:php8.3-fpm":            true,
		"file-set:supervisor-config":    true,
		"supervisor:supervisor":         true,
		"file-set:fail2ban-config":      true,
		"fail2ban:fail2ban":             true,
		"file-set:memcached-config":     true,
		"memcached:memcached":           true,
		"file-set:rabbitmq-config":      true,
		"rabbitmq:rabbitmq":             true,
		"file-set:logrotate-config":     true,
		"logrotate:logrotate":           true,
		"file-set:letsencrypt":          true,
		"certbot:certbot":               true,
		"file-set:cron-config":          true,
		"cron:cron":                     true,
		"file-set:systemd-units":        true,
		"systemd-service:portfolio":     true,
		"redis:redis":                   true,
		"mysql:app":                     true,
		"postgresql:analytics":          true,
	}
	got := map[string]bool{}
	for _, workload := range prof.Workloads {
		got[workload.Type+":"+workload.Name] = true
	}
	for key := range want {
		if !got[key] {
			t.Fatalf("expected workload %s in %#v", key, prof.Workloads)
		}
	}
	if got["docker-standalone:web-app-1"] {
		t.Fatalf("compose-owned container should not be suggested as standalone: %#v", prof.Workloads)
	}
}
