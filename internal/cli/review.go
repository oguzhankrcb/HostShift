package cli

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/safety"
)

type profileReview struct {
	Profile              string          `json:"profile"`
	Status               string          `json:"status"`
	ReadyForApply        bool            `json:"readyForApply"`
	SourcePolicy         string          `json:"sourcePolicy"`
	SourceWillBeModified bool            `json:"sourceWillBeModified"`
	SafeForAI            bool            `json:"safeForAI"`
	Summary              string          `json:"summary"`
	Findings             []reviewFinding `json:"findings"`
	OperatorChecklist    []string        `json:"operatorChecklist"`
	AIBrief              []string        `json:"aiBrief"`
}

type reviewFinding struct {
	Severity       string `json:"severity"`
	Category       string `json:"category"`
	Message        string `json:"message"`
	Evidence       string `json:"evidence,omitempty"`
	Recommendation string `json:"recommendation"`
}

func review(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
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
	return write(stdout, reviewPlan(prof, plan), *jsonOutput)
}

func reviewPlan(prof profile.Profile, plan planner.Plan) profileReview {
	findings := []reviewFinding{}
	for _, blocker := range plan.Blockers {
		findings = append(findings, reviewFinding{
			Severity:       "blocker",
			Category:       "plan",
			Message:        blocker,
			Recommendation: "Resolve this before any apply command.",
		})
	}
	for _, warning := range plan.Warnings {
		findings = append(findings, reviewFinding{
			Severity:       "warning",
			Category:       "compatibility",
			Message:        warning,
			Recommendation: "Review workload compatibility and add verification checks that prove the target behavior.",
		})
	}
	if prof.SourcePolicy != "strict-read-only" {
		findings = append(findings, reviewFinding{
			Severity:       "blocker",
			Category:       "source-safety",
			Message:        "Source policy is not strict-read-only.",
			Evidence:       prof.SourcePolicy,
			Recommendation: "Keep sourcePolicy set to strict-read-only. HostShift must not mutate the source server.",
		})
	}
	if len(prof.Checks) == 0 {
		findings = append(findings, reviewFinding{
			Severity:       "warning",
			Category:       "verification",
			Message:        "Profile has no verification checks.",
			Recommendation: "Add HTTP, database, service, file, firewall, or nginx checks that prove the migrated workload works on the target.",
		})
	}
	if len(prof.Workloads) == 0 {
		findings = append(findings, reviewFinding{
			Severity:       "warning",
			Category:       "coverage",
			Message:        "Profile has no workloads.",
			Recommendation: "Run discover again or add reviewed workloads before migration.",
		})
	}
	for _, stream := range plan.Streams {
		if len(stream.Preconditions) == 0 {
			findings = append(findings, reviewFinding{
				Severity:       "info",
				Category:       "stream-review",
				Message:        "Source-to-target stream should be reviewed before apply.",
				Evidence:       stream.ID,
				Recommendation: "Confirm the source command is read-only and the target command writes only to the intended target path or service.",
			})
		}
	}
	for _, action := range plan.Actions {
		if action.HostRole == core.HostRoleTarget && (action.Impact == core.ImpactService || action.Impact == core.ImpactNetwork) {
			findings = append(findings, reviewFinding{
				Severity:       "info",
				Category:       "target-impact",
				Message:        "Plan includes a target-side service or network action.",
				Evidence:       action.ID,
				Recommendation: "Review preconditions and rollback metadata before running the corresponding apply phase.",
			})
		}
	}

	ready := len(plan.Blockers) == 0 && prof.Approved && prof.Target.SSH != ""
	status := "needs-review"
	if len(plan.Blockers) > 0 {
		status = "blocked"
	} else if ready {
		status = "ready-for-dry-run"
	}
	summary := fmt.Sprintf("%s review found %d findings across %d workloads, %d checks, %d actions, and %d streams.", prof.Name, len(findings), len(prof.Workloads), len(prof.Checks), len(plan.Actions), len(plan.Streams))
	checklist := []string{
		"Review every blocker and warning.",
		"Confirm workload list matches the intended source server scope.",
		"Confirm verification checks prove application, database, service, and network behavior on the target.",
		"Run plan, prepare, sync, and verify dry-runs before any apply command.",
		"Run apply phases only from the human-operated CLI after reviewing rollback metadata.",
	}
	brief := []string{
		"Do not run arbitrary source SSH commands.",
		"Do not suggest MCP apply operations; MCP exposes planning, review, and dry-run tools only.",
		"Treat sourceWillBeModified=false as an invariant that must remain true.",
		"Ask for operator approval before any target mutation.",
	}
	if !ready {
		brief = append(brief, "The profile is not ready for apply; explain the blocking or review items before suggesting next commands.")
	} else {
		brief = append(brief, "The profile can proceed to dry-run review; apply still requires a human CLI command.")
	}
	return profileReview{
		Profile:              prof.Name,
		Status:               status,
		ReadyForApply:        ready,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: plan.SourceWillBeModified,
		SafeForAI:            !plan.SourceWillBeModified && !strings.Contains(strings.ToLower(status), "apply"),
		Summary:              summary,
		Findings:             findings,
		OperatorChecklist:    checklist,
		AIBrief:              brief,
	}
}
