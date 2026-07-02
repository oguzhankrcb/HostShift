package core

import "testing"

func TestSourceActionsMustBeReadOnly(t *testing.T) {
	action := Action{
		ID:       "source.bad",
		Phase:    PhaseSync,
		HostRole: HostRoleSource,
		Impact:   ImpactWrite,
		Command:  []string{"touch", "/tmp/file"},
	}
	if err := action.Validate(); err == nil {
		t.Fatal("expected source write action to be rejected")
	}
}

func TestTargetWriteActionIsAllowed(t *testing.T) {
	action := Action{
		ID:       "target.prepare",
		Phase:    PhasePrepare,
		HostRole: HostRoleTarget,
		Impact:   ImpactWrite,
		Command:  []string{"apt-get", "install", "-y", "rsync"},
	}
	if err := action.Validate(); err != nil {
		t.Fatalf("expected target write action to be valid: %v", err)
	}
}

func TestStreamActionRequiresBothSides(t *testing.T) {
	stream := StreamAction{
		ID:            "stream.mysql",
		Phase:         PhaseSync,
		SourceCommand: []string{"mysqldump", "--single-transaction", "app"},
		TargetCommand: []string{"mysql"},
	}
	if err := stream.Validate(); err != nil {
		t.Fatalf("expected stream action to be valid: %v", err)
	}
	stream.TargetCommand = nil
	if err := stream.Validate(); err == nil {
		t.Fatal("expected stream action without target command to fail")
	}
}
