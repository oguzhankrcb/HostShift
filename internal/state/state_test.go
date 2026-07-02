package state

import "testing"

func TestSaveLoadAndAudit(t *testing.T) {
	dir := t.TempDir()
	run := RunState{RunID: "test-run", Profile: "profile.yaml", Phase: "sync"}
	if err := Save(dir, run); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, run.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.RunID != run.RunID || loaded.Phase != run.Phase {
		t.Fatalf("unexpected state: %+v", loaded)
	}
	if err := AppendAudit(dir, AuditEvent{RunID: run.RunID, Phase: "sync", Action: "source.read"}); err != nil {
		t.Fatal(err)
	}
}
