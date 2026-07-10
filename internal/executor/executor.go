package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/safety"
	"github.com/oguzhankaracabay/hostshift/internal/state"
)

type Runner interface {
	RunSource(context.Context, string, []string) ([]byte, error)
	RunTarget(context.Context, string, []string) ([]byte, error)
	Stream(context.Context, string, []string, string, []string) ([]byte, error)
}

type Options struct {
	Apply         bool
	StateDir      string
	RunID         string
	Resume        *state.RunState
	PreserveState bool
	RetryFailed   string
}

type Result struct {
	ActionID            string        `json:"actionId"`
	Phase               core.Phase    `json:"phase"`
	HostRole            core.HostRole `json:"hostRole"`
	DryRun              bool          `json:"dryRun"`
	Skipped             bool          `json:"skipped"`
	Stream              bool          `json:"stream,omitempty"`
	Output              string        `json:"output,omitempty"`
	Error               string        `json:"error,omitempty"`
	PreviouslyCompleted bool          `json:"previouslyCompleted,omitempty"`
}

func Phase(ctx context.Context, prof profile.Profile, plan planner.Plan, phase core.Phase, runner Runner, options Options) ([]Result, error) {
	if options.RunID == "" {
		options.RunID = NewRunID(string(phase))
	}
	if !options.PreserveState {
		lock, err := state.AcquireRunLock(options.StateDir, options.RunID)
		if err != nil {
			return nil, err
		}
		defer lock.Release()
	}
	if options.Apply && len(plan.Blockers) > 0 {
		return nil, fmt.Errorf("cannot apply while plan has blockers: %s", strings.Join(plan.Blockers, "; "))
	}
	planHash, err := phasePlanHash(prof, plan, phase)
	if err != nil {
		return nil, err
	}
	completed, err := resumeCompleted(options, prof, plan, phase, planHash)
	if err != nil {
		return nil, err
	}
	if err := validateRetryConfirmation(options); err != nil {
		return nil, err
	}
	run := state.RunState{
		RunID:     options.RunID,
		Profile:   prof.Name,
		Phase:     string(phase),
		PlanHash:  planHash,
		Status:    phaseStatus(options.Apply, plan.Blockers),
		Completed: append([]string{}, completed...),
		BlockedBy: append([]string{}, plan.Blockers...),
	}
	if options.Resume != nil {
		run.FailedAction = options.Resume.FailedAction
		run.UncertainAction = options.Resume.UncertainAction
		run.LastError = options.Resume.LastError
	}
	if !options.PreserveState {
		if err := state.Save(options.StateDir, run); err != nil {
			return nil, err
		}
	}
	completedSet := stringSet(completed)
	results := []Result{}
	for _, action := range plan.Actions {
		if action.Phase != phase {
			continue
		}
		result := Result{ActionID: action.ID, Phase: action.Phase, HostRole: action.HostRole, DryRun: !options.Apply, Skipped: !options.Apply}
		if completedSet[action.ID] {
			result.Skipped = true
			result.PreviouslyCompleted = true
			results = append(results, result)
			continue
		}
		if !options.Apply {
			results = append(results, result)
			continue
		}
		run.UncertainAction = action.ID
		run.FailedAction = ""
		run.LastError = ""
		if err := saveRunState(options, run); err != nil {
			return results, err
		}
		out, err := runAction(ctx, prof, runner, action)
		if err != nil {
			errorText := safety.Redact(err.Error())
			redactedErr := fmt.Errorf("%s", errorText)
			result.Error = errorText
			results = append(results, result)
			_ = state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: action.ID, Message: "failed: " + errorText})
			run.Status = "failed"
			run.FailedAction = action.ID
			run.UncertainAction = action.ID
			run.LastError = errorText
			if saveErr := saveRunState(options, run); saveErr != nil {
				return results, fmt.Errorf("%w; state save failed: %v", redactedErr, saveErr)
			}
			return results, redactedErr
		}
		result.Output = safety.Redact(strings.TrimSpace(string(out)))
		results = append(results, result)
		completed = append(completed, action.ID)
		completedSet[action.ID] = true
		run.Completed = append([]string{}, completed...)
		run.UncertainAction = ""
		if err := saveRunState(options, run); err != nil {
			return results, err
		}
		if err := state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: action.ID, Message: "completed"}); err != nil {
			return results, err
		}
	}
	for _, stream := range plan.Streams {
		if stream.Phase != phase {
			continue
		}
		result := Result{ActionID: stream.ID, Phase: stream.Phase, HostRole: core.HostRoleSource, DryRun: !options.Apply, Skipped: !options.Apply, Stream: true}
		if completedSet[stream.ID] {
			result.Skipped = true
			result.PreviouslyCompleted = true
			results = append(results, result)
			continue
		}
		if !options.Apply {
			results = append(results, result)
			continue
		}
		run.UncertainAction = stream.ID
		run.FailedAction = ""
		run.LastError = ""
		if err := saveRunState(options, run); err != nil {
			return results, err
		}
		out, err := runStream(ctx, prof, runner, stream)
		if err != nil {
			errorText := safety.Redact(err.Error())
			redactedErr := fmt.Errorf("%s", errorText)
			result.Error = errorText
			results = append(results, result)
			_ = state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: stream.ID, Message: "failed: " + errorText})
			run.Status = "failed"
			run.FailedAction = stream.ID
			run.UncertainAction = stream.ID
			run.LastError = errorText
			if saveErr := saveRunState(options, run); saveErr != nil {
				return results, fmt.Errorf("%w; state save failed: %v", redactedErr, saveErr)
			}
			return results, redactedErr
		}
		result.Output = safety.Redact(strings.TrimSpace(string(out)))
		results = append(results, result)
		completed = append(completed, stream.ID)
		completedSet[stream.ID] = true
		run.Completed = append([]string{}, completed...)
		run.UncertainAction = ""
		if err := saveRunState(options, run); err != nil {
			return results, err
		}
		if err := state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: stream.ID, Message: "completed stream"}); err != nil {
			return results, err
		}
	}
	run.Status = finalPhaseStatus(options.Apply, plan.Blockers)
	run.Completed = append([]string{}, completed...)
	run.FailedAction = ""
	run.UncertainAction = ""
	run.LastError = ""
	if err := saveRunState(options, run); err != nil {
		return results, err
	}
	return results, nil
}

func validateRetryConfirmation(options Options) error {
	if options.Resume == nil {
		if options.RetryFailed != "" {
			return fmt.Errorf("--retry-failed is valid only when resuming a run")
		}
		return nil
	}
	uncertain := options.Resume.UncertainAction
	if uncertain == "" {
		uncertain = options.Resume.FailedAction
	}
	if uncertain == "" {
		if options.RetryFailed != "" {
			return fmt.Errorf("run has no failed or uncertain action to retry")
		}
		return nil
	}
	if !options.Apply {
		return nil
	}
	if options.RetryFailed != uncertain {
		return fmt.Errorf("action %s may have partially changed the target; resume apply requires --retry-failed %s", uncertain, uncertain)
	}
	return nil
}

func phasePlanHash(prof profile.Profile, plan planner.Plan, phase core.Phase) (string, error) {
	actions := []core.Action{}
	for _, action := range plan.Actions {
		if action.Phase == phase {
			actions = append(actions, action)
		}
	}
	streams := []core.StreamAction{}
	for _, stream := range plan.Streams {
		if stream.Phase == phase {
			streams = append(streams, stream)
		}
	}
	payload := struct {
		Profile      string              `json:"profile"`
		Source       string              `json:"source"`
		Target       string              `json:"target"`
		SourcePolicy string              `json:"sourcePolicy"`
		Phase        core.Phase          `json:"phase"`
		Actions      []core.Action       `json:"actions"`
		Streams      []core.StreamAction `json:"streams"`
		Blockers     []string            `json:"blockers,omitempty"`
	}{prof.Name, prof.Source.SSH, prof.Target.SSH, prof.SourcePolicy, phase, actions, streams, plan.Blockers}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

func resumeCompleted(options Options, prof profile.Profile, plan planner.Plan, phase core.Phase, planHash string) ([]string, error) {
	if options.Resume == nil {
		return nil, nil
	}
	run := options.Resume
	if run.RunID != options.RunID {
		return nil, fmt.Errorf("resume run id mismatch: state has %s, command requested %s", run.RunID, options.RunID)
	}
	if run.Profile != prof.Name {
		return nil, fmt.Errorf("resume profile mismatch: state has %s, profile is %s", run.Profile, prof.Name)
	}
	if run.Phase != string(phase) {
		return nil, fmt.Errorf("resume phase mismatch: state has %s, plan requested %s", run.Phase, phase)
	}
	if run.PlanHash == "" {
		return nil, fmt.Errorf("run %s predates safe resume fingerprints; start a new phase run", run.RunID)
	}
	if run.PlanHash != planHash {
		return nil, fmt.Errorf("resume plan fingerprint mismatch; profile, target, or generated commands changed")
	}
	expected := map[string]bool{}
	for _, action := range plan.Actions {
		if action.Phase == phase {
			expected[action.ID] = true
		}
	}
	for _, stream := range plan.Streams {
		if stream.Phase == phase {
			expected[stream.ID] = true
		}
	}
	completed := []string{}
	seen := map[string]bool{}
	for _, id := range run.Completed {
		if !expected[id] {
			return nil, fmt.Errorf("resume state contains unknown completed action %s", id)
		}
		if !seen[id] {
			completed = append(completed, id)
			seen[id] = true
		}
	}
	for label, id := range map[string]string{"failedAction": run.FailedAction, "uncertainAction": run.UncertainAction} {
		if id == "" {
			continue
		}
		if !expected[id] {
			return nil, fmt.Errorf("resume state contains unknown %s %s", label, id)
		}
		if seen[id] {
			return nil, fmt.Errorf("resume state marks completed action %s as %s", id, label)
		}
	}
	if run.Status == "failed" && run.FailedAction == "" && run.UncertainAction == "" {
		return nil, fmt.Errorf("resume state is inconsistent: failed run has no failed or uncertain action")
	}
	if run.Status == "completed" && len(seen) != len(expected) {
		return nil, fmt.Errorf("resume state is inconsistent: completed run is missing completed action ids")
	}
	return completed, nil
}

func saveRunState(options Options, run state.RunState) error {
	if options.PreserveState {
		return nil
	}
	return state.Save(options.StateDir, run)
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func phaseStatus(apply bool, blockers []string) string {
	if len(blockers) > 0 {
		return "blocked"
	}
	if apply {
		return "running"
	}
	return "dry-run"
}

func finalPhaseStatus(apply bool, blockers []string) string {
	if len(blockers) > 0 {
		return "blocked"
	}
	if apply {
		return "completed"
	}
	return "dry-run"
}

func runStream(ctx context.Context, prof profile.Profile, runner Runner, stream core.StreamAction) ([]byte, error) {
	if err := stream.Validate(); err != nil {
		return nil, err
	}
	if err := safety.SourceCommand(stream.SourceCommand); err != nil {
		return nil, err
	}
	if err := safety.TargetCommand(stream.TargetCommand); err != nil {
		return nil, err
	}
	return runner.Stream(ctx, prof.Source.SSH, stream.SourceCommand, prof.Target.SSH, stream.TargetCommand)
}

func runAction(ctx context.Context, prof profile.Profile, runner Runner, action core.Action) ([]byte, error) {
	if err := action.Validate(); err != nil {
		return nil, err
	}
	switch action.HostRole {
	case core.HostRoleSource:
		return runner.RunSource(ctx, prof.Source.SSH, action.Command)
	case core.HostRoleTarget:
		return runner.RunTarget(ctx, prof.Target.SSH, action.Command)
	case core.HostRoleLocal:
		return nil, fmt.Errorf("local action execution is not enabled: %s", action.ID)
	default:
		return nil, fmt.Errorf("unknown host role: %s", action.HostRole)
	}
}

func NewRunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}
