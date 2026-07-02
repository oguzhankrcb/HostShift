package executor

import (
	"context"
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
	Apply    bool
	StateDir string
	RunID    string
}

type Result struct {
	ActionID string        `json:"actionId"`
	Phase    core.Phase    `json:"phase"`
	HostRole core.HostRole `json:"hostRole"`
	DryRun   bool          `json:"dryRun"`
	Skipped  bool          `json:"skipped"`
	Stream   bool          `json:"stream,omitempty"`
	Output   string        `json:"output,omitempty"`
	Error    string        `json:"error,omitempty"`
}

func Phase(ctx context.Context, prof profile.Profile, plan planner.Plan, phase core.Phase, runner Runner, options Options) ([]Result, error) {
	if options.RunID == "" {
		options.RunID = NewRunID(string(phase))
	}
	if options.Apply && len(plan.Blockers) > 0 {
		return nil, fmt.Errorf("cannot apply while plan has blockers: %s", strings.Join(plan.Blockers, "; "))
	}
	if err := state.Save(options.StateDir, state.RunState{RunID: options.RunID, Profile: prof.Name, Phase: string(phase), BlockedBy: plan.Blockers}); err != nil {
		return nil, err
	}
	results := []Result{}
	for _, action := range plan.Actions {
		if action.Phase != phase {
			continue
		}
		result := Result{ActionID: action.ID, Phase: action.Phase, HostRole: action.HostRole, DryRun: !options.Apply, Skipped: !options.Apply}
		if !options.Apply {
			results = append(results, result)
			continue
		}
		out, err := runAction(ctx, prof, runner, action)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			_ = state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: action.ID, Message: "failed: " + err.Error()})
			return results, err
		}
		result.Output = safety.Redact(strings.TrimSpace(string(out)))
		results = append(results, result)
		if err := state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: action.ID, Message: "completed"}); err != nil {
			return results, err
		}
	}
	for _, stream := range plan.Streams {
		if stream.Phase != phase {
			continue
		}
		result := Result{ActionID: stream.ID, Phase: stream.Phase, HostRole: core.HostRoleSource, DryRun: !options.Apply, Skipped: !options.Apply, Stream: true}
		if !options.Apply {
			results = append(results, result)
			continue
		}
		out, err := runStream(ctx, prof, runner, stream)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			_ = state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: stream.ID, Message: "failed: " + err.Error()})
			return results, err
		}
		result.Output = safety.Redact(strings.TrimSpace(string(out)))
		results = append(results, result)
		if err := state.AppendAudit(options.StateDir, state.AuditEvent{RunID: options.RunID, Phase: string(phase), Action: stream.ID, Message: "completed stream"}); err != nil {
			return results, err
		}
	}
	completed := make([]string, 0, len(results))
	for _, result := range results {
		if !result.Skipped && result.Error == "" {
			completed = append(completed, result.ActionID)
		}
	}
	if err := state.Save(options.StateDir, state.RunState{RunID: options.RunID, Profile: prof.Name, Phase: string(phase), Completed: completed, BlockedBy: plan.Blockers}); err != nil {
		return results, err
	}
	return results, nil
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
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().Unix())
}
