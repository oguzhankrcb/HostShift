package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"syscall"
	"time"
)

var safeRunID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

type RunState struct {
	RunID           string    `json:"runId"`
	Profile         string    `json:"profile"`
	Phase           string    `json:"phase"`
	PlanHash        string    `json:"planHash,omitempty"`
	Status          string    `json:"status,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`
	Completed       []string  `json:"completed,omitempty"`
	BlockedBy       []string  `json:"blockedBy,omitempty"`
	FailedAction    string    `json:"failedAction,omitempty"`
	UncertainAction string    `json:"uncertainAction,omitempty"`
	LastError       string    `json:"lastError,omitempty"`
}

type AuditEvent struct {
	Time    time.Time `json:"time"`
	RunID   string    `json:"runId"`
	Phase   string    `json:"phase"`
	Action  string    `json:"action"`
	Message string    `json:"message,omitempty"`
}

type RunLock struct {
	file *os.File
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
	if err := ValidateRunID(run.RunID); err != nil {
		return err
	}
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
	temp, err := os.CreateTemp(runDir, ".state-*.json")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(body, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempPath, filepath.Join(runDir, "state.json"))
}

func Load(dir, runID string) (RunState, error) {
	if err := ValidateRunID(runID); err != nil {
		return RunState{}, err
	}
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
	if err := ValidateRunID(event.RunID); err != nil {
		return err
	}
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

func AcquireRunLock(dir, runID string) (*RunLock, error) {
	if err := ValidateRunID(runID); err != nil {
		return nil, err
	}
	if dir == "" {
		dir = DefaultDir()
	}
	runDir := filepath.Join(dir, "runs", runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(runDir, "run.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		return nil, fmt.Errorf("run %s is already active", runID)
	}
	return &RunLock{file: file}, nil
}

func (lock *RunLock) Release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func ValidateRunID(runID string) error {
	if !safeRunID.MatchString(runID) {
		return fmt.Errorf("run id contains unsafe characters")
	}
	return nil
}
