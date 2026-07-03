package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/executor"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/safety"
	"github.com/oguzhankaracabay/hostshift/internal/source"
	"github.com/oguzhankaracabay/hostshift/internal/ssh"
	"github.com/oguzhankaracabay/hostshift/internal/state"
	"github.com/oguzhankaracabay/hostshift/internal/version"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	_ = ctx
	_ = stderr
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprint(stdout, helpText())
		return nil
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(stdout, version.Version)
		return nil
	case "doctor":
		return doctor(args[1:], stdout)
	case "discover":
		return discover(ctx, args[1:], stdout)
	case "plan":
		return plan(args[1:], stdout)
	case "prepare":
		return runPhase(ctx, args[1:], stdout, core.PhasePrepare)
	case "sync":
		return runPhase(ctx, args[1:], stdout, core.PhaseSync)
	case "verify":
		return runPhase(ctx, args[1:], stdout, core.PhaseVerify)
	case "cutover":
		return cutover(ctx, args[1:], stdout)
	case "rollback":
		return rollback(args[1:], stdout)
	case "profile":
		return profileCommand(args[1:], stdout)
	case "status":
		return status(args[1:], stdout)
	case "resume":
		return resume(args[1:], stdout)
	case "policy":
		return policy(args[1:], stdout)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

func cutover(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("cutover", flag.ContinueOnError)
	profilePath := fs.String("profile", "", "profile path")
	target := fs.String("target", "", "target ssh alias override")
	apply := fs.Bool("apply", false, "execute target cutover actions")
	confirm := fs.String("confirm", "", "confirmation code")
	stateDir := fs.String("state-dir", "", "state directory")
	runID := fs.String("run-id", "", "run id")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profilePath == "" {
		return fmt.Errorf("--profile is required")
	}
	prof, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	if *target != "" {
		if err := safety.SSHAlias(*target); err != nil {
			return err
		}
		prof.Target.SSH = *target
	}
	plan, err := planner.Build(prof, time.Now().UTC())
	if err != nil {
		return err
	}
	code := confirmationCode(prof)
	if !*apply {
		actions := []core.Action{}
		for _, action := range plan.Actions {
			if action.Phase == core.PhaseCutover {
				actions = append(actions, action)
			}
		}
		return write(stdout, map[string]any{
			"dryRun":               true,
			"confirmationCode":     code,
			"sourceWillBeModified": false,
			"blockers":             plan.Blockers,
			"actions":              actions,
		}, *jsonOutput)
	}
	if len(plan.Blockers) > 0 {
		return fmt.Errorf("cannot apply while plan has blockers: %s", strings.Join(plan.Blockers, "; "))
	}
	if *confirm != code {
		return fmt.Errorf("invalid confirmation code; expected %s", code)
	}
	results, err := executor.Phase(ctx, prof, plan, core.PhaseCutover, ssh.Runner{}, executor.Options{
		Apply:    true,
		StateDir: *stateDir,
		RunID:    *runID,
	})
	if err != nil {
		return err
	}
	return write(stdout, map[string]any{
		"phase":                core.PhaseCutover,
		"apply":                true,
		"sourceWillBeModified": false,
		"results":              results,
	}, *jsonOutput)
}

func rollback(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("rollback", flag.ContinueOnError)
	profilePath := fs.String("profile", "", "profile path")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profilePath == "" {
		return fmt.Errorf("--profile is required")
	}
	prof, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	plan, err := planner.Build(prof, time.Now().UTC())
	if err != nil {
		return err
	}
	rollbackActions := map[string][]string{}
	for _, action := range plan.Actions {
		if len(action.Rollback) > 0 {
			rollbackActions[action.ID] = action.Rollback
		}
	}
	return write(stdout, map[string]any{
		"automatic":       false,
		"sourceChanged":   false,
		"sourcePolicy":    prof.SourcePolicy,
		"targetRollbacks": rollbackActions,
		"message":         "The source was never changed. Keep DNS on the source and inspect the target before stopping target services.",
	}, *jsonOutput)
}

func doctor(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	source := fs.String("source", "", "source ssh alias")
	target := fs.String("target", "", "target ssh alias")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := safety.SSHAlias(*source); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if err := safety.SSHAlias(*target); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	body := map[string]any{
		"version":              version.Version,
		"source":               *source,
		"target":               *target,
		"sourceWillBeModified": false,
		"sourcePolicy":         "strict-read-only",
	}
	return write(stdout, body, *jsonOutput)
}

func discover(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	sourceAlias := fs.String("source", "", "source ssh alias")
	name := fs.String("name", "", "profile name")
	profilePath := fs.String("profile", "", "profile path")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("--name is required")
	}
	client, err := source.New(*sourceAlias, ssh.Runner{})
	if err != nil {
		return err
	}
	facts := client.Discover(ctx)
	requiredFailures := []string{}
	for _, fact := range source.Facts {
		result := facts[fact.Name]
		if !fact.Optional && !result.OK {
			requiredFailures = append(requiredFailures, fact.Name)
		}
	}
	if len(requiredFailures) > 0 {
		return fmt.Errorf("required source facts failed: %v", requiredFailures)
	}
	prof := source.ProfileFromFacts(*name, *sourceAlias, facts)
	if *profilePath == "" {
		*profilePath = *name + ".profile.yaml"
	}
	if err := profile.Save(*profilePath, prof); err != nil {
		return err
	}
	return write(stdout, map[string]any{
		"profile":              *profilePath,
		"sourcePolicy":         prof.SourcePolicy,
		"sourceWillBeModified": false,
		"facts":                facts,
		"requiredFailures":     requiredFailures,
	}, *jsonOutput)
}

func plan(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("plan", flag.ContinueOnError)
	profilePath := fs.String("profile", "", "profile path")
	target := fs.String("target", "", "target ssh alias override")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profilePath == "" {
		return fmt.Errorf("--profile is required")
	}
	prof, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	if *target != "" {
		if err := safety.SSHAlias(*target); err != nil {
			return err
		}
		prof.Target.SSH = *target
	}
	plan, err := planner.Build(prof, time.Now().UTC())
	if err != nil {
		return err
	}
	return write(stdout, plan, *jsonOutput)
}

func runPhase(ctx context.Context, args []string, stdout io.Writer, phase core.Phase) error {
	fs := flag.NewFlagSet(string(phase), flag.ContinueOnError)
	profilePath := fs.String("profile", "", "profile path")
	target := fs.String("target", "", "target ssh alias override")
	apply := fs.Bool("apply", false, "execute remote actions")
	stateDir := fs.String("state-dir", "", "state directory")
	runID := fs.String("run-id", "", "run id")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *profilePath == "" {
		return fmt.Errorf("--profile is required")
	}
	prof, err := profile.Load(*profilePath)
	if err != nil {
		return err
	}
	if *target != "" {
		if err := safety.SSHAlias(*target); err != nil {
			return err
		}
		prof.Target.SSH = *target
	}
	plan, err := planner.Build(prof, time.Now().UTC())
	if err != nil {
		return err
	}
	results, err := executor.Phase(ctx, prof, plan, phase, ssh.Runner{}, executor.Options{
		Apply:    *apply,
		StateDir: *stateDir,
		RunID:    *runID,
	})
	if err != nil {
		return err
	}
	return write(stdout, map[string]any{
		"phase":                phase,
		"apply":                *apply,
		"sourceWillBeModified": false,
		"blockers":             plan.Blockers,
		"results":              results,
	}, *jsonOutput)
}

func profileCommand(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("profile subcommand is required")
	}
	switch args[0] {
	case "migrate":
		fs := flag.NewFlagSet("profile migrate", flag.ContinueOnError)
		input := fs.String("input", "", "v1 profile path")
		output := fs.String("output", "", "v2 profile path")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *input == "" || *output == "" {
			return fmt.Errorf("--input and --output are required")
		}
		prof, err := profile.Load(*input)
		if err != nil {
			return err
		}
		return profile.Save(*output, prof)
	default:
		return fmt.Errorf("unknown profile subcommand: %s", args[0])
	}
}

func status(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	runID := fs.String("run-id", "", "run id")
	stateDir := fs.String("state-dir", "", "state directory")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	run, err := state.Load(*stateDir, *runID)
	if err != nil {
		return err
	}
	return write(stdout, run, *jsonOutput)
}

func resume(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	runID := fs.String("run-id", "", "run id")
	stateDir := fs.String("state-dir", "", "state directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *runID == "" {
		return fmt.Errorf("--run-id is required")
	}
	run, err := state.Load(*stateDir, *runID)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Run %s is resumable from phase %s; execution engine is not enabled in this milestone.\n", run.RunID, run.Phase)
	return nil
}

func policy(args []string, stdout io.Writer) error {
	if len(args) > 0 && args[0] != "source" {
		return fmt.Errorf("unknown policy topic: %s", args[0])
	}
	return write(stdout, map[string]any{
		"sourcePolicy":         "strict-read-only",
		"sourceWillBeModified": false,
		"forbidden": []string{
			"sudo", "package installation", "service management", "file writes",
			"snapshot creation", "maintenance mode", "firewall changes",
		},
	}, true)
}

func confirmationCode(prof profile.Profile) string {
	return "START-" + strings.ToUpper(prof.Name)
}

func write(stdout io.Writer, value any, jsonOutput bool) error {
	if jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(value)
	}
	switch v := value.(type) {
	case string:
		fmt.Fprintln(stdout, v)
	default:
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(v)
	}
	return nil
}

func helpText() string {
	return fmt.Sprintf(`hostshift %s

Read-only-source server migration CLI for Ubuntu and Debian.

Commands:
  doctor          --source <ssh> --target <ssh> [--json]
  discover        --source <ssh> --name <name> [--profile <file>] [--json]
  plan            --profile <file> [--target <ssh>] [--json]
  prepare         --profile <file> [--target <ssh>] [--apply] [--json]
  sync            --profile <file> [--target <ssh>] [--apply] [--json]
  verify          --profile <file> [--target <ssh>] [--apply] [--json]
  cutover         --profile <file> [--target <ssh>] [--apply --confirm <code>] [--json]
  rollback        --profile <file> [--json]
  mcp stdio       run the HostShift MCP stdio server for AI clients
  profile migrate --input <v1-profile> --output <v2-profile>
  status          --run-id <id> [--state-dir <dir>] [--json]
  resume          --run-id <id> [--state-dir <dir>]
  policy source
  version

Safety:
  HostShift treats the source as a strictly read-only observation endpoint.
`, version.Version)
}

func NewRunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().Unix())
}
