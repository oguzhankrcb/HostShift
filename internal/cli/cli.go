package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/oguzhankaracabay/hostshift/internal/core"
	"github.com/oguzhankaracabay/hostshift/internal/dockere2e"
	"github.com/oguzhankaracabay/hostshift/internal/executor"
	"github.com/oguzhankaracabay/hostshift/internal/planner"
	"github.com/oguzhankaracabay/hostshift/internal/profile"
	"github.com/oguzhankaracabay/hostshift/internal/safety"
	"github.com/oguzhankaracabay/hostshift/internal/source"
	"github.com/oguzhankaracabay/hostshift/internal/ssh"
	"github.com/oguzhankaracabay/hostshift/internal/state"
	"github.com/oguzhankaracabay/hostshift/internal/version"
	"github.com/oguzhankaracabay/hostshift/internal/vme2e"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
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
	case "explain":
		return explain(args[1:], stdout)
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
	case "sbom":
		return sbom(ctx, args[1:], stdout)
	case "matrix":
		return matrix(args[1:], stdout)
	case "docker-e2e":
		return dockere2e.Run(ctx, args[1:], stdout, stderr)
	case "vm-e2e":
		return vme2e.Run(ctx, args[1:], stdout, stderr)
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

type planExplanation struct {
	Profile              string         `json:"profile"`
	SourcePolicy         string         `json:"sourcePolicy"`
	SourceWillBeModified bool           `json:"sourceWillBeModified"`
	ReadyForApply        bool           `json:"readyForApply"`
	Blockers             []string       `json:"blockers,omitempty"`
	Warnings             []string       `json:"warnings,omitempty"`
	Summary              string         `json:"summary"`
	Workloads            []string       `json:"workloads,omitempty"`
	ActionCounts         map[string]int `json:"actionCounts"`
	StreamCount          int            `json:"streamCount"`
	TargetImpacts        map[string]int `json:"targetImpacts"`
	NextActions          []string       `json:"nextActions"`
	SafetyNotes          []string       `json:"safetyNotes"`
}

func explain(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
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
	return write(stdout, explainPlan(prof, plan), *jsonOutput)
}

func explainPlan(prof profile.Profile, plan planner.Plan) planExplanation {
	actionCounts := map[string]int{}
	targetImpacts := map[string]int{}
	for _, action := range plan.Actions {
		actionCounts[string(action.Phase)]++
		if action.HostRole == core.HostRoleTarget {
			targetImpacts[string(action.Impact)]++
		}
	}
	workloads := make([]string, 0, len(prof.Workloads))
	for _, workload := range prof.Workloads {
		workloads = append(workloads, workload.Type+":"+workload.Name)
	}
	ready := len(plan.Blockers) == 0 && prof.Approved && prof.Target.SSH != ""
	summary := fmt.Sprintf("%s has %d workloads, %d actions, and %d streams. Source mutation is disabled.", prof.Name, len(prof.Workloads), len(plan.Actions), len(plan.Streams))
	nextActions := []string{}
	if len(plan.Blockers) > 0 {
		nextActions = append(nextActions, "Resolve blockers before running any apply command.")
	} else if !prof.Approved {
		nextActions = append(nextActions, "Review the generated profile and set approved: true only after human review.")
	} else {
		nextActions = append(nextActions, "Run prepare, sync, and verify as dry-runs before any apply command.")
		nextActions = append(nextActions, "Apply target phases only from the CLI after reviewing actions, streams, and rollback metadata.")
	}
	if len(plan.Streams) > 0 {
		nextActions = append(nextActions, "Review every stream source command; source streams must remain read-only.")
	}
	return planExplanation{
		Profile:              prof.Name,
		SourcePolicy:         prof.SourcePolicy,
		SourceWillBeModified: plan.SourceWillBeModified,
		ReadyForApply:        ready,
		Blockers:             plan.Blockers,
		Warnings:             plan.Warnings,
		Summary:              summary,
		Workloads:            workloads,
		ActionCounts:         actionCounts,
		StreamCount:          len(plan.Streams),
		TargetImpacts:        targetImpacts,
		NextActions:          nextActions,
		SafetyNotes: []string{
			"MCP and AI integrations do not expose apply commands.",
			"The source host is treated as a read-only observation endpoint.",
			"Target writes, service restarts, and network changes require reviewed CLI apply commands.",
		},
	}
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

type goModule struct {
	Name    string
	Version string
}

type sbomDocument struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      sbomCreationInfo   `json:"creationInfo"`
	Packages          []sbomPackage      `json:"packages"`
	Relationships     []sbomRelationship `json:"relationships"`
}

type sbomCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
}

type sbomPackage struct {
	Name              string            `json:"name"`
	SPDXID            string            `json:"SPDXID"`
	VersionInfo       string            `json:"versionInfo"`
	DownloadLocation  string            `json:"downloadLocation"`
	FilesAnalyzed     bool              `json:"filesAnalyzed"`
	LicenseConcluded  string            `json:"licenseConcluded"`
	LicenseDeclared   string            `json:"licenseDeclared"`
	CopyrightText     string            `json:"copyrightText"`
	ExternalReference []sbomExternalRef `json:"externalRefs"`
}

type sbomExternalRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

type sbomRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
}

func sbom(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sbom", flag.ContinueOnError)
	output := fs.String("output", "dist/hostshift.sbom.spdx.json", "output SPDX JSON path")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	modules, err := listGoModules(ctx)
	if err != nil {
		return err
	}
	document := buildSBOMDocument(modules, time.Now().UTC())
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(*output, body, 0o644); err != nil {
		return err
	}
	return write(stdout, map[string]any{
		"output":       *output,
		"packageCount": len(document.Packages),
		"format":       "SPDX-2.3",
	}, *jsonOutput)
}

func listGoModules(ctx context.Context) ([]goModule, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "all")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("go list -m all failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	modules := []goModule{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		module := goModule{Name: fields[0]}
		if len(fields) > 1 {
			module.Version = fields[1]
		}
		modules = append(modules, module)
	}
	return modules, nil
}

func buildSBOMDocument(modules []goModule, now time.Time) sbomDocument {
	packages := make([]sbomPackage, 0, len(modules))
	for index, module := range modules {
		spdxID := fmt.Sprintf("SPDXRef-Package-%s-%d", sanitizeSPDXID(module.Name), index+1)
		version := module.Version
		if version == "" {
			version = "main"
		}
		downloadLocation := "NOASSERTION"
		if module.Version != "" {
			downloadLocation = "https://" + module.Name
		}
		purl := "pkg:golang/" + url.PathEscape(module.Name)
		if module.Version != "" {
			purl += "@" + url.PathEscape(module.Version)
		}
		packages = append(packages, sbomPackage{
			Name:             module.Name,
			SPDXID:           spdxID,
			VersionInfo:      version,
			DownloadLocation: downloadLocation,
			FilesAnalyzed:    false,
			LicenseConcluded: "NOASSERTION",
			LicenseDeclared:  "NOASSERTION",
			CopyrightText:    "NOASSERTION",
			ExternalReference: []sbomExternalRef{{
				ReferenceCategory: "PACKAGE-MANAGER",
				ReferenceType:     "purl",
				ReferenceLocator:  purl,
			}},
		})
	}
	relationships := []sbomRelationship{}
	if len(packages) > 0 {
		relationships = append(relationships, sbomRelationship{
			SPDXElementID:      "SPDXRef-DOCUMENT",
			RelationshipType:   "DESCRIBES",
			RelatedSPDXElement: packages[0].SPDXID,
		})
	}
	return sbomDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              "HostShift Go module dependency SBOM",
		DocumentNamespace: fmt.Sprintf("https://github.com/oguzhankaracabay/hostshift/sbom/%d", now.UnixMilli()),
		CreationInfo: sbomCreationInfo{
			Created:  now.Format(time.RFC3339),
			Creators: []string{"Tool: hostshift sbom"},
		},
		Packages:      packages,
		Relationships: relationships,
	}
}

func sanitizeSPDXID(value string) string {
	replacer := strings.NewReplacer("/", "-", "_", "-", ":", "-", "@", "-", " ", "-")
	value = replacer.Replace(value)
	out := strings.Builder{}
	for _, char := range value {
		if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '.' || char == '-' {
			out.WriteRune(char)
		} else {
			out.WriteByte('-')
		}
	}
	safe := strings.Trim(out.String(), "-")
	if safe == "" {
		return "module"
	}
	return safe
}

type dockerMatrixPair struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	SourceImage string `json:"sourceImage"`
	TargetImage string `json:"targetImage"`
}

var dockerMatrixTargets = map[string][]string{
	"ubuntu22": {"ubuntu22", "ubuntu24", "ubuntu25", "debian12"},
	"debian12": {"ubuntu22", "ubuntu24", "ubuntu25", "debian12", "debian13"},
}

var dockerMatrixImages = map[string]string{
	"ubuntu22": "ubuntu:22.04",
	"ubuntu24": "ubuntu:24.04",
	"ubuntu25": "ubuntu:25.10",
	"debian12": "debian:12",
	"debian13": "debian:13",
}

func matrix(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("matrix subcommand is required")
	}
	switch args[0] {
	case "docker":
		return dockerMatrix(args[1:], stdout)
	case "vm":
		return vmMatrix(args[1:], stdout)
	default:
		return fmt.Errorf("unknown matrix subcommand: %s", args[0])
	}
}

func dockerMatrix(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("matrix docker", flag.ContinueOnError)
	list := fs.Bool("list", false, "list matrix pairs")
	listImages := fs.Bool("list-images", false, "list unique base images")
	pairFilter := fs.String("pair", "", "matrix pair filter, for example ubuntu22->debian12")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pairs := dockerMatrixPairs()
	if *pairFilter != "" {
		filtered := []dockerMatrixPair{}
		for _, pair := range pairs {
			if pair.Source+"->"+pair.Target == *pairFilter {
				filtered = append(filtered, pair)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("unknown matrix pair: %s", *pairFilter)
		}
		pairs = filtered
	}
	if *listImages {
		images := dockerMatrixUniqueImages(pairs)
		if *jsonOutput {
			return write(stdout, map[string]any{"images": images}, true)
		}
		for _, image := range images {
			fmt.Fprintln(stdout, image)
		}
		return nil
	}
	if *list {
		if *jsonOutput {
			return write(stdout, map[string]any{"pairs": pairs}, true)
		}
		for _, pair := range pairs {
			fmt.Fprintf(stdout, "%s -> %s\n", pair.Source, pair.Target)
		}
		return nil
	}
	return write(stdout, map[string]any{
		"pairs":                pairs,
		"pairCount":            len(pairs),
		"dryRun":               true,
		"sourceWillBeModified": false,
		"message":              "Dry-run only. Set HOSTSHIFT_RUN_DOCKER_MATRIX=1 with tests/integration/docker/run-matrix.sh to execute Docker compose config/build and source immutability checks for each pair.",
		"checks": []string{
			"source immutability checks",
			"Docker compose config/build",
			"HostShift discover/plan/prepare/sync/verify dry-runs",
		},
	}, *jsonOutput)
}

func dockerMatrixPairs() []dockerMatrixPair {
	order := []string{"ubuntu22", "debian12"}
	pairs := []dockerMatrixPair{}
	for _, source := range order {
		for _, target := range dockerMatrixTargets[source] {
			pairs = append(pairs, dockerMatrixPair{
				Source:      source,
				Target:      target,
				SourceImage: dockerMatrixImages[source],
				TargetImage: dockerMatrixImages[target],
			})
		}
	}
	return pairs
}

func dockerMatrixUniqueImages(pairs []dockerMatrixPair) []string {
	seen := map[string]bool{}
	for _, pair := range pairs {
		seen[pair.SourceImage] = true
		seen[pair.TargetImage] = true
	}
	order := []string{"debian:12", "debian:13", "ubuntu:22.04", "ubuntu:24.04", "ubuntu:25.10"}
	images := []string{}
	for _, image := range order {
		if seen[image] {
			images = append(images, image)
		}
	}
	return images
}

type vmMatrixPair struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Provider string `json:"provider"`
}

func vmMatrix(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("matrix vm", flag.ContinueOnError)
	list := fs.Bool("list", false, "list VM matrix pairs")
	pairFilter := fs.String("pair", "", "matrix pair filter, for example ubuntu22->debian12")
	provider := fs.String("provider", "lima", "VM provider")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *provider != "lima" {
		return fmt.Errorf("unknown VM provider: %s", *provider)
	}
	pairs := vmMatrixPairs(*provider)
	if *pairFilter != "" {
		filtered := []vmMatrixPair{}
		for _, pair := range pairs {
			if pair.Source+"->"+pair.Target == *pairFilter {
				filtered = append(filtered, pair)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("unknown matrix pair: %s", *pairFilter)
		}
		pairs = filtered
	}
	if *list {
		if *jsonOutput {
			return write(stdout, map[string]any{"provider": *provider, "pairs": pairs}, true)
		}
		for _, pair := range pairs {
			fmt.Fprintf(stdout, "%s -> %s\n", pair.Source, pair.Target)
		}
		return nil
	}
	return write(stdout, map[string]any{
		"provider":             *provider,
		"pairs":                pairs,
		"pairCount":            len(pairs),
		"dryRun":               true,
		"sourceWillBeModified": false,
		"message":              "Dry-run only. Set HOSTSHIFT_RUN_VM_E2E=1 for provider preflight and VM boot. Add --apply with tests/e2e/vm/run-vm-e2e.sh to execute the real provider workflow.",
		"checks": []string{
			"provider preflight and VM boot",
			"Lima template rendering",
			"source immutability snapshot checks",
			"post-reboot target verification",
		},
	}, *jsonOutput)
}

func vmMatrixPairs(provider string) []vmMatrixPair {
	raw := []struct {
		source string
		target string
	}{
		{"ubuntu22", "ubuntu22"},
		{"ubuntu22", "ubuntu24"},
		{"ubuntu22", "ubuntu25"},
		{"ubuntu22", "debian12"},
		{"debian12", "ubuntu22"},
		{"debian12", "ubuntu24"},
		{"debian12", "ubuntu25"},
		{"debian12", "debian12"},
		{"debian12", "debian13"},
	}
	pairs := make([]vmMatrixPair, 0, len(raw))
	for _, pair := range raw {
		pairs = append(pairs, vmMatrixPair{Source: pair.source, Target: pair.target, Provider: provider})
	}
	return pairs
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
  explain         --profile <file> [--target <ssh>] [--json]
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
  sbom            [--output <file>] [--json]
  matrix docker   [--list] [--list-images] [--pair <source->target>] [--json]
  matrix vm       [--list] [--pair <source->target>] [--provider lima] [--json]
  docker-e2e      [--list] [--list-images] [--pair <source->target>] [--pull-images]
  vm-e2e          [--list] [--pair <source->target>] [--provider lima] [--emit-dir <dir>] [--apply]
  version

Safety:
  HostShift treats the source as a strictly read-only observation endpoint.
`, version.Version)
}

func NewRunID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().Unix())
}
