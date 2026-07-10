package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/state"
)

type fakeRunner struct {
	source         [][]string
	target         [][]string
	streams        []streamCall
	failTargetCall int
	failStreamCall int
	targetError    error
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
	if f.failTargetCall > 0 && len(f.target) == f.failTargetCall {
		if f.targetError != nil {
			return nil, f.targetError
		}
		return nil, errors.New("target failed")
	}
	return []byte("target ok\n"), nil
}

func (f *fakeRunner) Stream(_ context.Context, _ string, sourceCommand []string, _ string, targetCommand []string) ([]byte, error) {
	f.streams = append(f.streams, streamCall{source: sourceCommand, target: targetCommand})
	if f.failStreamCall > 0 && len(f.streams) == f.failStreamCall {
		return nil, errors.New("stream failed")
	}
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

func TestPhaseResumeSkipsCompletedActionsAfterFailure(t *testing.T) {
	dir := t.TempDir()
	prof := approvedProfile()
	plan := planner.Plan{
		Profile:              prof.Name,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: false,
		Actions: []core.Action{
			{ID: "target.first", Phase: core.PhasePrepare, HostRole: core.HostRoleTarget, Impact: core.ImpactWrite, Command: []string{"first"}},
			{ID: "target.second", Phase: core.PhasePrepare, HostRole: core.HostRoleTarget, Impact: core.ImpactWrite, Command: []string{"second"}},
		},
	}
	firstRunner := &fakeRunner{failTargetCall: 2}
	results, err := Phase(context.Background(), prof, plan, core.PhasePrepare, firstRunner, Options{Apply: true, StateDir: dir, RunID: "resume-run"})
	if err == nil || len(results) != 2 {
		t.Fatalf("expected second action failure, results=%+v err=%v", results, err)
	}
	run, err := state.Load(dir, "resume-run")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || run.FailedAction != "target.second" || strings.Join(run.Completed, ",") != "target.first" || run.PlanHash == "" {
		t.Fatalf("unexpected failed run state: %+v", run)
	}
	if run.UncertainAction != "target.second" {
		t.Fatalf("failed action must be marked uncertain: %+v", run)
	}
	unconfirmedRunner := &fakeRunner{}
	_, err = Phase(context.Background(), prof, plan, core.PhasePrepare, unconfirmedRunner, Options{Apply: true, StateDir: dir, RunID: "resume-run", Resume: &run})
	if err == nil || !strings.Contains(err.Error(), "--retry-failed target.second") {
		t.Fatalf("expected explicit failed action retry confirmation, got %v", err)
	}
	if len(unconfirmedRunner.target) != 0 {
		t.Fatalf("unconfirmed resume must not execute commands: %+v", unconfirmedRunner.target)
	}
	resumeRunner := &fakeRunner{}
	resumed, err := Phase(context.Background(), prof, plan, core.PhasePrepare, resumeRunner, Options{Apply: true, StateDir: dir, RunID: "resume-run", Resume: &run, RetryFailed: "target.second"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 2 || !resumed[0].Skipped || !resumed[0].PreviouslyCompleted {
		t.Fatalf("expected first action to be skipped as previously completed: %+v", resumed)
	}
	if len(resumeRunner.target) != 1 || strings.Join(resumeRunner.target[0], " ") != "second" {
		t.Fatalf("resume must execute only the pending action: %+v", resumeRunner.target)
	}
	finished, err := state.Load(dir, "resume-run")
	if err != nil {
		t.Fatal(err)
	}
	if finished.Status != "completed" || strings.Join(finished.Completed, ",") != "target.first,target.second" || finished.FailedAction != "" || finished.UncertainAction != "" || finished.LastError != "" {
		t.Fatalf("unexpected completed resume state: %+v", finished)
	}
}

func TestPhaseResumeRejectsChangedPlan(t *testing.T) {
	dir := t.TempDir()
	prof := approvedProfile()
	plan := planner.Plan{
		Profile:              prof.Name,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: false,
		Actions:              []core.Action{{ID: "target.first", Phase: core.PhasePrepare, HostRole: core.HostRoleTarget, Impact: core.ImpactWrite, Command: []string{"first"}}},
	}
	if _, err := Phase(context.Background(), prof, plan, core.PhasePrepare, &fakeRunner{}, Options{Apply: true, StateDir: dir, RunID: "changed-plan"}); err != nil {
		t.Fatal(err)
	}
	run, err := state.Load(dir, "changed-plan")
	if err != nil {
		t.Fatal(err)
	}
	plan.Actions[0].Command = []string{"changed"}
	runner := &fakeRunner{}
	_, err = Phase(context.Background(), prof, plan, core.PhasePrepare, runner, Options{Apply: true, StateDir: dir, RunID: "changed-plan", Resume: &run})
	if err == nil || !strings.Contains(err.Error(), "fingerprint mismatch") {
		t.Fatalf("expected changed plan to block resume, got %v", err)
	}
	if len(runner.target) != 0 {
		t.Fatalf("changed plan must not execute target commands: %+v", runner.target)
	}
}

func TestPhaseResumeRejectsLegacyStateWithoutFingerprint(t *testing.T) {
	prof := approvedProfile()
	plan := planner.Plan{Profile: prof.Name, SourcePolicy: prof.SourcePolicy, SourceWillBeModified: false}
	run := state.RunState{RunID: "legacy", Profile: prof.Name, Phase: string(core.PhasePrepare)}
	_, err := Phase(context.Background(), prof, plan, core.PhasePrepare, &fakeRunner{}, Options{Apply: true, StateDir: t.TempDir(), RunID: "legacy", Resume: &run})
	if err == nil || !strings.Contains(err.Error(), "predates safe resume fingerprints") {
		t.Fatalf("expected legacy state to be rejected, got %v", err)
	}
}

func TestPhaseResumeRequiresExplicitRetryForFailedStream(t *testing.T) {
	dir := t.TempDir()
	prof := approvedProfile()
	plan := planner.Plan{
		Profile:              prof.Name,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: false,
		Streams: []core.StreamAction{{
			ID:            "stream.data",
			Phase:         core.PhaseSync,
			SourceCommand: []string{"cat", "/srv/app/data.tar"},
			TargetCommand: []string{"tar", "--extract", "--file=-", "-C", "/srv/app"},
		}},
	}
	if _, err := Phase(context.Background(), prof, plan, core.PhaseSync, &fakeRunner{failStreamCall: 1}, Options{Apply: true, StateDir: dir, RunID: "stream-resume"}); err == nil {
		t.Fatal("expected initial stream failure")
	}
	run, err := state.Load(dir, "stream-resume")
	if err != nil {
		t.Fatal(err)
	}
	if run.FailedAction != "stream.data" || run.UncertainAction != "stream.data" {
		t.Fatalf("failed stream must remain uncertain: %+v", run)
	}
	resumeRunner := &fakeRunner{}
	if _, err := Phase(context.Background(), prof, plan, core.PhaseSync, resumeRunner, Options{Apply: true, StateDir: dir, RunID: "stream-resume", Resume: &run}); err == nil {
		t.Fatal("expected unconfirmed stream retry to be rejected")
	}
	if len(resumeRunner.streams) != 0 {
		t.Fatalf("unconfirmed stream must not be retried: %+v", resumeRunner.streams)
	}
	if _, err := Phase(context.Background(), prof, plan, core.PhaseSync, resumeRunner, Options{Apply: true, StateDir: dir, RunID: "stream-resume", Resume: &run, RetryFailed: "stream.data"}); err != nil {
		t.Fatal(err)
	}
	if len(resumeRunner.streams) != 1 {
		t.Fatalf("confirmed resume should retry exactly one stream: %+v", resumeRunner.streams)
	}
}

func TestPhaseRedactsFailureFromStateAuditAndReturnedError(t *testing.T) {
	dir := t.TempDir()
	prof := approvedProfile()
	plan := planner.Plan{
		Profile:              prof.Name,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: false,
		Actions:              []core.Action{{ID: "target.secret-error", Phase: core.PhasePrepare, HostRole: core.HostRoleTarget, Impact: core.ImpactWrite, Command: []string{"false"}}},
	}
	_, err := Phase(context.Background(), prof, plan, core.PhasePrepare, &fakeRunner{failTargetCall: 1, targetError: errors.New("password=supersecret")}, Options{Apply: true, StateDir: dir, RunID: "redacted-error"})
	if err == nil || strings.Contains(err.Error(), "supersecret") || !strings.Contains(err.Error(), "[redacted]") {
		t.Fatalf("returned error must be redacted, got %v", err)
	}
	for _, path := range []string{
		filepath.Join(dir, "runs", "redacted-error", "state.json"),
		filepath.Join(dir, "runs", "redacted-error", "audit.jsonl"),
	} {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if strings.Contains(string(body), "supersecret") || !strings.Contains(string(body), "[redacted]") {
			t.Fatalf("failure file must contain only redacted error: %s", body)
		}
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
