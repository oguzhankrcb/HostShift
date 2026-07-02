package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type RunState struct {
	RunID     string    `json:"runId"`
	Profile   string    `json:"profile"`
	Phase     string    `json:"phase"`
	UpdatedAt time.Time `json:"updatedAt"`
	Completed []string  `json:"completed,omitempty"`
	BlockedBy []string  `json:"blockedBy,omitempty"`
}

type AuditEvent struct {
	Time    time.Time `json:"time"`
	RunID   string    `json:"runId"`
	Phase   string    `json:"phase"`
	Action  string    `json:"action"`
	Message string    `json:"message,omitempty"`
}

func DefaultDir() string {
	if dir := os.Getenv("HOSTSHIFT_STATE_DIR"); dir != "" {
		return dir
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "hostshift")
	}
	return ".hostshift"
}

func Save(dir string, run RunState) error {
	if dir == "" {
		dir = DefaultDir()
	}
	run.UpdatedAt = time.Now().UTC()
	runDir := filepath.Join(dir, "runs", run.RunID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir, "state.json"), append(body, '\n'), 0o600)
}

func Load(dir, runID string) (RunState, error) {
	if dir == "" {
		dir = DefaultDir()
	}
	body, err := os.ReadFile(filepath.Join(dir, "runs", runID, "state.json"))
	if err != nil {
		return RunState{}, err
	}
	var run RunState
	return run, json.Unmarshal(body, &run)
}

func AppendAudit(dir string, event AuditEvent) error {
	if dir == "" {
		dir = DefaultDir()
	}
	event.Time = time.Now().UTC()
	runDir := filepath.Join(dir, "runs", event.RunID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return err
	}
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(runDir, "audit.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(body, '\n'))
	return err
}
