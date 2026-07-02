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
