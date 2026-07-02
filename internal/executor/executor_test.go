package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
)

type fakeRunner struct {
	source  [][]string
	target  [][]string
	streams []streamCall
}

type streamCall struct {
	source []string
	target []string
}

func (f *fakeRunner) RunSource(_ context.Context, _ string, command []string) ([]byte, error) {
	f.source = append(f.source, command)
	return []byte("source ok\n"), nil
}

func (f *fakeRunner) RunTarget(_ context.Context, _ string, command []string) ([]byte, error) {
	f.target = append(f.target, command)
	return []byte("target ok\n"), nil
}

func (f *fakeRunner) Stream(_ context.Context, _ string, sourceCommand []string, _ string, targetCommand []string) ([]byte, error) {
	f.streams = append(f.streams, streamCall{source: sourceCommand, target: targetCommand})
	return []byte("stream ok\n"), nil
}

func TestPhaseDryRunDoesNotExecute(t *testing.T) {
	prof := approvedProfile()
	plan, err := planner.Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	results, err := Phase(context.Background(), prof, plan, core.PhasePrepare, runner, Options{StateDir: t.TempDir(), RunID: "dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || !results[0].DryRun || !results[0].Skipped {
		t.Fatalf("expected skipped dry-run result, got %+v", results)
	}
	if len(runner.source) != 0 || len(runner.target) != 0 || len(runner.streams) != 0 {
		t.Fatal("dry-run must not execute remote commands")
	}
}

func TestPhaseApplyRunsOnlySelectedPhase(t *testing.T) {
	prof := approvedProfile()
	plan, err := planner.Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	results, err := Phase(context.Background(), prof, plan, core.PhasePrepare, runner, Options{Apply: true, StateDir: t.TempDir(), RunID: "apply-run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 || len(runner.target) == 0 {
		t.Fatalf("expected target prepare execution, results=%+v runner=%+v", results, runner)
	}
	if len(runner.source) != 0 {
		t.Fatal("prepare phase must not execute source actions")
	}
}

func TestPhaseApplyRefusesBlockers(t *testing.T) {
	prof := approvedProfile()
	prof.Approved = false
	plan, err := planner.Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Phase(context.Background(), prof, plan, core.PhasePrepare, &fakeRunner{}, Options{Apply: true, StateDir: t.TempDir(), RunID: "blocked"}); err == nil {
		t.Fatal("expected apply to fail with blockers")
	}
}

func TestPhaseApplyRunsStreams(t *testing.T) {
	prof := approvedProfile()
	prof.Workloads = []profile.Workload{{Type: "mysql", Name: "app"}}
	plan, err := planner.Build(prof, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	results, err := Phase(context.Background(), prof, plan, core.PhaseSync, runner, Options{Apply: true, StateDir: t.TempDir(), RunID: "stream-run"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Stream {
		t.Fatalf("expected stream result, got %+v", results)
	}
	if len(runner.streams) != 1 {
		t.Fatalf("expected one stream call, got %+v", runner.streams)
	}
	sourceCommand := strings.Join(runner.streams[0].source, " ")
	targetCommand := strings.Join(runner.streams[0].target, " ")
	if runner.streams[0].source[0] != "sh" || !strings.Contains(sourceCommand, "mysqldump") || runner.streams[0].target[0] != "sh" || !strings.Contains(targetCommand, "mysql") {
		t.Fatalf("unexpected stream commands: %+v", runner.streams[0])
	}
}

func TestPhaseApplyRejectsMutatingSourceStream(t *testing.T) {
	prof := approvedProfile()
	plan := planner.Plan{
		Profile:              prof.Name,
		SourcePolicy:         "strict-read-only",
		SourceWillBeModified: false,
		Streams: []core.StreamAction{{
			ID:            "bad.stream",
			Phase:         core.PhaseSync,
			SourceCommand: []string{"touch", "/tmp/bad"},
			TargetCommand: []string{"cat"},
		}},
	}
	_, err := Phase(context.Background(), prof, plan, core.PhaseSync, &fakeRunner{}, Options{Apply: true, StateDir: t.TempDir(), RunID: "bad-stream"})
	if err == nil {
		t.Fatal("expected mutating source stream to be rejected")
	}
}

func approvedProfile() profile.Profile {
	return profile.Profile{
		SchemaVersion: profile.CurrentSchemaVersion,
		Name:          "example",
		Source:        profile.Host{SSH: "old"},
		Target:        profile.Host{SSH: "new"},
		Platforms:     profile.Platforms{Source: "ubuntu:24.04", Target: "ubuntu:24.04"},
		SourcePolicy:  "strict-read-only",
		Approved:      true,
	}
}
