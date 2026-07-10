package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadAndAudit(t *testing.T) {
	dir := t.TempDir()
	run := RunState{RunID: "test-run", Profile: "profile.yaml", Phase: "sync", PlanHash: "abc123", Status: "running", Completed: []string{"first"}}
	if err := Save(dir, run); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != run.RunID || loaded.Phase != run.Phase || loaded.PlanHash != run.PlanHash || loaded.Status != run.Status || len(loaded.Completed) != 1 {
		t.Fatalf("unexpected state: %+v", loaded)
	}
	info, err := os.Stat(filepath.Join(dir, "runs", run.RunID, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state file must be private, got %o", info.Mode().Perm())
	}
	if err := AppendAudit(dir, AuditEvent{RunID: run.RunID, Phase: "sync", Action: "source.read"}); err != nil {
		t.Fatal(err)
	}
}

func TestRunIDRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, RunState{RunID: "../outside", Profile: "example", Phase: "sync"}); err == nil {
		t.Fatal("expected unsafe run id to be rejected")
	}
	if _, err := Load(dir, "../outside"); err == nil {
		t.Fatal("expected unsafe run id load to be rejected")
	}
	if err := AppendAudit(dir, AuditEvent{RunID: "../outside", Phase: "sync"}); err == nil {
		t.Fatal("expected unsafe audit run id to be rejected")
	}
}

func TestRunLockPreventsConcurrentExecution(t *testing.T) {
	dir := t.TempDir()
	first, err := AcquireRunLock(dir, "locked-run")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireRunLock(dir, "locked-run"); err == nil {
		t.Fatal("expected second lock acquisition to fail")
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireRunLock(dir, "locked-run")
	if err != nil {
		t.Fatalf("expected lock to be reusable after release: %v", err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}
