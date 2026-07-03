package source

import (
	"context"
	"slices"
	"testing"
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
	for _, expected := range []string{"osRelease", "dockerComposeProjects", "nftRuleset"} {
		if !slices.Contains(names, expected) {
			t.Fatalf("expected %s in fact allowlist", expected)
		}
	}
	if _, ok := FactByName("touchTmp"); ok {
		t.Fatal("unexpected mutating fact in allowlist")
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
		"mysqlDatabases":      {OK: true, Value: "information_schema\napp\nmysql\n"},
		"postgresDatabases":   {OK: true, Value: "postgres\nanalytics\ntemplate1\n"},
		"nginxConfigDump":     {OK: true, Value: "nginx: configuration file /etc/nginx/nginx.conf test is successful"},
		"letsEncryptFiles":    {OK: true, Value: "/etc/letsencrypt/live/example.com/fullchain.pem"},
		"apacheConfigDump":    {OK: false, Error: "apache missing"},
		"dockerVersion":       {OK: true, Value: `"25.0.0"`},
		"dockerNetworks":      {OK: true, Value: "{}"},
		"enabledServices":     {OK: true, Value: "nginx.service enabled"},
		"runningServices":     {OK: true, Value: "nginx.service loaded active running"},
		"listeners":           {OK: true, Value: "LISTEN 0 4096 *:80"},
		"ufwStatus":           {OK: true, Value: "Status: active"},
		"nftRuleset":          {OK: true, Value: "table inet filter {}"},
		"sshdEffectiveConfig": {OK: true, Value: "port 22"},
	}
	prof := ProfileFromFacts("example", "source-host", facts)
	want := map[string]bool{
		"docker-compose:web":            true,
		"file-set:srv-web":              true,
		"docker-standalone:redis-cache": true,
		"file-set:nginx-config":         true,
		"file-set:letsencrypt":          true,
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
