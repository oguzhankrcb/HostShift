package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/oguzhankaracabay/hostshift/internal/mcp"
	"github.com/oguzhankaracabay/hostshift/internal/version"
)

func ServeMCP(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	return mcp.Serve(ctx, hostshiftMCPServer(), stdin, stdout)
}

func hostshiftMCPServer() mcp.Server {
	return mcp.Server{
		Name:         "hostshift",
		Title:        "HostShift",
		Version:      version.Version,
		Instructions: "HostShift exposes read-only-source server migration planning, explanation, review, and dry-run tools. MCP tools do not expose --apply; target mutations require the human-operated CLI.",
		Resources: []mcp.Resource{
			{
				URI:         "hostshift://source-safety",
				Name:        "source-safety",
				Title:       "HostShift Source Safety",
				Description: "The source immutability contract AI clients must preserve.",
				MimeType:    "text/markdown",
				Text:        hostshiftSourceSafetyResource(),
			},
			{
				URI:         "hostshift://migration-workflow",
				Name:        "migration-workflow",
				Title:       "HostShift Migration Workflow",
				Description: "The reviewed HostShift dry-run-first migration workflow.",
				MimeType:    "text/markdown",
				Text:        hostshiftWorkflowResource(),
			},
			{
				URI:         "hostshift://capabilities",
				Name:        "capabilities",
				Title:       "HostShift Capabilities Catalog",
				Description: "Supported releases, package mappings, workloads, checks, and source facts.",
				MimeType:    "application/json",
				Text:        hostshiftCapabilitiesResource(),
			},
		},
		Prompts: []mcp.Prompt{
			{
				Name:        "hostshift_migration_operator",
				Title:       "HostShift Migration Operator",
				Description: "Guide an AI client through a safe HostShift migration while preserving the read-only source invariant.",
				Messages: []mcp.PromptMessage{{
					Role: "user",
					Text: hostshiftMigrationOperatorPrompt(),
				}},
			},
		},
		Tools: []mcp.Tool{
			cliTool("hostshift_doctor", "HostShift Doctor", "Validate source and target SSH aliases and report the source safety contract.", objectSchema(map[string]any{
				"source": stringSchema("Source SSH alias."),
				"target": stringSchema("Target SSH alias."),
			}, "source", "target"), func(args map[string]any) []string {
				return []string{"doctor", "--source", requiredString(args, "source"), "--target", requiredString(args, "target")}
			}),
			cliTool("hostshift_discover", "HostShift Discover", "Read allowlisted source facts and write a reviewable profile. This never mutates the source.", objectSchema(map[string]any{
				"source":  stringSchema("Source SSH alias."),
				"name":    stringSchema("Generated profile name."),
				"profile": stringSchema("Optional output profile path."),
			}, "source", "name"), func(args map[string]any) []string {
				out := []string{"discover", "--source", requiredString(args, "source"), "--name", requiredString(args, "name")}
				if profile := optionalString(args, "profile"); profile != "" {
					out = append(out, "--profile", profile)
				}
				return out
			}),
			phaseTool("hostshift_plan", "HostShift Plan", "Build the migration plan without applying target changes.", "plan"),
			phaseTool("hostshift_explain", "HostShift Explain", "Summarize blockers, warnings, workload counts, streams, target impacts, and safe next actions for AI-assisted review.", "explain"),
			phaseTool("hostshift_review", "HostShift Review", "Return structured migration findings, operator checklist, and AI safety brief without applying changes.", "review"),
			phaseTool("hostshift_prepare_dry_run", "HostShift Prepare Dry Run", "Show target preparation actions without applying them.", "prepare"),
			phaseTool("hostshift_sync_dry_run", "HostShift Sync Dry Run", "Show source-to-target streams without applying them.", "sync"),
			phaseTool("hostshift_verify_dry_run", "HostShift Verify Dry Run", "Show target verification checks without applying them.", "verify"),
			phaseTool("hostshift_cutover_dry_run", "HostShift Cutover Dry Run", "Show target-only cutover actions and confirmation code without applying them.", "cutover"),
			cliTool("hostshift_status", "HostShift Run Status", "Read local state for a HostShift run without executing remote commands.", objectSchema(map[string]any{
				"runId":    stringSchema("Run identifier."),
				"stateDir": stringSchema("Optional HostShift state directory."),
			}, "runId"), func(args map[string]any) []string {
				out := []string{"status", "--run-id", requiredString(args, "runId")}
				if stateDir := optionalString(args, "stateDir"); stateDir != "" {
					out = append(out, "--state-dir", stateDir)
				}
				return out
			}),
			cliTool("hostshift_resume_dry_run", "HostShift Resume Dry Run", "Show completed, pending, failed, and uncertain steps for a saved run without applying or retrying them.", objectSchema(map[string]any{
				"profile":  stringSchema("Profile path used by the saved run."),
				"runId":    stringSchema("Run identifier."),
				"stateDir": stringSchema("Optional HostShift state directory."),
				"target":   stringSchema("Optional target SSH alias override; it must match the saved plan fingerprint."),
			}, "profile", "runId"), func(args map[string]any) []string {
				out := []string{"resume", "--profile", requiredString(args, "profile"), "--run-id", requiredString(args, "runId")}
				if stateDir := optionalString(args, "stateDir"); stateDir != "" {
					out = append(out, "--state-dir", stateDir)
				}
				if target := optionalString(args, "target"); target != "" {
					out = append(out, "--target", target)
				}
				return out
			}),
			cliTool("hostshift_profile_migrate", "HostShift Profile Migrate", "Convert a legacy HostShift profile to profile v2 YAML. This is a local file conversion and never mutates a source host.", objectSchema(map[string]any{
				"input":  stringSchema("Input legacy profile path."),
				"output": stringSchema("Output v2 profile path."),
			}, "input", "output"), func(args map[string]any) []string {
				return []string{"profile", "migrate", "--input", requiredString(args, "input"), "--output", requiredString(args, "output")}
			}),
			cliTool("hostshift_policy_source", "HostShift Source Policy", "Return the strict read-only source policy contract for AI clients and operators.", objectSchema(map[string]any{}), func(args map[string]any) []string {
				return []string{"policy", "source"}
			}),
			cliTool("hostshift_capabilities", "HostShift Capabilities", "Return supported platforms, package mappings, workload types, check types, source facts, and AI safety guidance without running remote commands.", objectSchema(map[string]any{}), func(args map[string]any) []string {
				return []string{"capabilities"}
			}),
			cliTool("hostshift_rollback", "HostShift Rollback Metadata", "Report manual rollback guidance and target rollback metadata. The source is never changed.", objectSchema(map[string]any{
				"profile": stringSchema("Profile path."),
			}, "profile"), func(args map[string]any) []string {
				return []string{"rollback", "--profile", requiredString(args, "profile")}
			}),
		},
	}
}

type mcpDoctorReport struct {
	ServerName               string            `json:"serverName"`
	ServerTitle              string            `json:"serverTitle"`
	ServerVersion            string            `json:"serverVersion"`
	ProtocolVersion          string            `json:"protocolVersion"`
	ToolCount                int               `json:"toolCount"`
	Tools                    []string          `json:"tools"`
	PromptCount              int               `json:"promptCount"`
	Prompts                  []string          `json:"prompts"`
	ResourceCount            int               `json:"resourceCount"`
	Resources                []string          `json:"resources"`
	RequiredToolsPresent     bool              `json:"requiredToolsPresent"`
	RequiredPromptsPresent   bool              `json:"requiredPromptsPresent"`
	RequiredResourcesPresent bool              `json:"requiredResourcesPresent"`
	ApplyToolsExposed        bool              `json:"applyToolsExposed"`
	SourceWillBeModified     bool              `json:"sourceWillBeModified"`
	ClaudeConfig             claudeConfigCheck `json:"claudeConfig"`
	Status                   string            `json:"status"`
	Warnings                 []string          `json:"warnings,omitempty"`
}

type claudeConfigCheck struct {
	Path         string   `json:"path"`
	Exists       bool     `json:"exists"`
	Server       string   `json:"server,omitempty"`
	Command      string   `json:"command,omitempty"`
	Args         []string `json:"args,omitempty"`
	Valid        bool     `json:"valid"`
	Error        string   `json:"error,omitempty"`
	Instructions string   `json:"instructions"`
}

func mcpCommand(args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("mcp subcommand is required")
	}
	switch args[0] {
	case "doctor":
		return mcpDoctor(args[1:], stdout)
	case "stdio":
		return fmt.Errorf("mcp stdio is handled by cmd/hostshift")
	default:
		return fmt.Errorf("unknown mcp subcommand: %s", args[0])
	}
}

func mcpDoctor(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("mcp doctor", flag.ContinueOnError)
	claudeConfig := fs.String("claude-config", "integrations/claude/claude_desktop_config.example.json", "Claude Desktop config path")
	jsonOutput := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	report := buildMCPDoctorReport(*claudeConfig)
	return write(stdout, report, *jsonOutput)
}

func buildMCPDoctorReport(claudeConfigPath string) mcpDoctorReport {
	server := hostshiftMCPServer()
	tools := make([]string, 0, len(server.Tools))
	prompts := make([]string, 0, len(server.Prompts))
	resources := make([]string, 0, len(server.Resources))
	applyToolsExposed := false
	seen := map[string]bool{}
	for _, tool := range server.Tools {
		tools = append(tools, tool.Name)
		seen[tool.Name] = true
		if containsApplyName(tool.Name) {
			applyToolsExposed = true
		}
	}
	seenPrompts := map[string]bool{}
	for _, prompt := range server.Prompts {
		prompts = append(prompts, prompt.Name)
		seenPrompts[prompt.Name] = true
	}
	seenResources := map[string]bool{}
	for _, resource := range server.Resources {
		resources = append(resources, resource.URI)
		seenResources[resource.URI] = true
	}
	requiredToolsPresent := true
	for _, name := range requiredMCPToolNames() {
		if !seen[name] {
			requiredToolsPresent = false
			break
		}
	}
	requiredPromptsPresent := true
	for _, name := range requiredMCPPromptNames() {
		if !seenPrompts[name] {
			requiredPromptsPresent = false
			break
		}
	}
	requiredResourcesPresent := true
	for _, uri := range requiredMCPResourceURIs() {
		if !seenResources[uri] {
			requiredResourcesPresent = false
			break
		}
	}
	report := mcpDoctorReport{
		ServerName:               server.Name,
		ServerTitle:              server.Title,
		ServerVersion:            server.Version,
		ProtocolVersion:          mcp.ProtocolVersion,
		ToolCount:                len(tools),
		Tools:                    tools,
		PromptCount:              len(prompts),
		Prompts:                  prompts,
		ResourceCount:            len(resources),
		Resources:                resources,
		RequiredToolsPresent:     requiredToolsPresent,
		RequiredPromptsPresent:   requiredPromptsPresent,
		RequiredResourcesPresent: requiredResourcesPresent,
		ApplyToolsExposed:        applyToolsExposed,
		SourceWillBeModified:     false,
		ClaudeConfig:             checkClaudeConfig(claudeConfigPath),
		Status:                   "ok",
	}
	if !requiredToolsPresent {
		report.Status = "warning"
		report.Warnings = append(report.Warnings, "one or more required MCP tools are missing")
	}
	if !requiredPromptsPresent {
		report.Status = "warning"
		report.Warnings = append(report.Warnings, "one or more required MCP prompts are missing")
	}
	if !requiredResourcesPresent {
		report.Status = "warning"
		report.Warnings = append(report.Warnings, "one or more required MCP resources are missing")
	}
	if applyToolsExposed {
		report.Status = "warning"
		report.Warnings = append(report.Warnings, "MCP must not expose apply tools")
	}
	if !report.ClaudeConfig.Valid {
		report.Status = "warning"
		report.Warnings = append(report.Warnings, "Claude Desktop config example is missing or invalid")
	}
	return report
}

func hostshiftMigrationOperatorPrompt() string {
	return `Use HostShift as the deterministic migration engine for Ubuntu and Debian server moves.

Safety invariants:
- Treat the source server as a strictly read-only observation endpoint.
- Do not run arbitrary source SSH commands.
- Do not use sudo, install packages, write files, manage services, alter firewall rules, create snapshots, add keys, or place apps in maintenance mode on the source.
- Do not expose or invent apply operations through MCP. Target mutations require a reviewed human CLI command.
- Do not print secrets, .env contents, Compose files, database dumps, or credentials verbatim.

Preferred workflow:
1. Run hostshift_capabilities to inspect supported platforms, workloads, checks, source facts, and package mappings.
2. Run hostshift_doctor for connectivity and source safety.
3. Run hostshift_discover to write a reviewable profile.
4. Run hostshift_plan, hostshift_explain, and hostshift_review before suggesting any target mutation.
5. Use prepare/sync/verify/cutover dry-run MCP tools only. Use hostshift_status and hostshift_resume_dry_run to inspect interrupted runs. A human operator must run CLI --apply after reviewing blockers, actions, streams, rollback metadata, and any failed-action retry requirement.

When a workload cannot be safely read online, say so explicitly and require an operator strategy instead of silently skipping it.`
}

func hostshiftSourceSafetyResource() string {
	return `# HostShift Source Safety

The source server is an immutable observation endpoint.

Allowed source behavior:
- read inventory
- inspect configuration
- stream typed read-only exports to stdout
- run allowlisted fact commands

Forbidden source behavior:
- sudo
- package installation
- service start, stop, restart, reload, or signal operations
- file writes, chmod, chown, mkdir, rm, mv, cp, tee, or shell redirection
- firewall changes
- SSH key changes
- snapshots or maintenance mode
- source-side database dump files or temporary archives

If a workload cannot be read safely online, HostShift must block or ask for an operator strategy. It must not silently skip the workload or weaken this invariant.`
}

func hostshiftWorkflowResource() string {
	return `# HostShift Migration Workflow

Use this dry-run-first sequence:

1. hostshift_capabilities
2. hostshift_doctor
3. hostshift_discover
4. hostshift_plan
5. hostshift_explain
6. hostshift_review
7. hostshift_prepare_dry_run
8. hostshift_sync_dry_run
9. hostshift_verify_dry_run
10. hostshift_cutover_dry_run
11. hostshift_status
12. hostshift_resume_dry_run

MCP does not expose apply tools. Target mutation requires a human-operated CLI command after reviewing blockers, warnings, actions, streams, preconditions, and rollback metadata.

Before suggesting an apply command, ensure the profile is reviewed, approved, has target SSH configured, has no blockers, and has verification checks that prove the migrated behavior.`
}

func hostshiftCapabilitiesResource() string {
	body, err := json.MarshalIndent(buildCapabilitiesReport(), "", "  ")
	if err != nil {
		return `{"sourceWillBeModified":false,"applyToolsExposed":false,"error":"failed to encode capabilities"}`
	}
	return string(body)
}

func requiredMCPToolNames() []string {
	return []string{
		"hostshift_doctor",
		"hostshift_discover",
		"hostshift_plan",
		"hostshift_explain",
		"hostshift_review",
		"hostshift_prepare_dry_run",
		"hostshift_sync_dry_run",
		"hostshift_verify_dry_run",
		"hostshift_cutover_dry_run",
		"hostshift_status",
		"hostshift_resume_dry_run",
		"hostshift_profile_migrate",
		"hostshift_policy_source",
		"hostshift_capabilities",
		"hostshift_rollback",
	}
}

func requiredMCPPromptNames() []string {
	return []string{
		"hostshift_migration_operator",
	}
}

func requiredMCPResourceURIs() []string {
	return []string{
		"hostshift://source-safety",
		"hostshift://migration-workflow",
		"hostshift://capabilities",
	}
}

func containsApplyName(name string) bool {
	return bytes.Contains([]byte(name), []byte("apply"))
}

func checkClaudeConfig(path string) claudeConfigCheck {
	check := claudeConfigCheck{
		Path:         path,
		Instructions: "Copy this JSON into Claude Desktop's claude_desktop_config.json and adjust command to the installed hostshift binary path.",
	}
	resolvedPath := resolveRepoRelativePath(path)
	check.Path = resolvedPath
	body, err := os.ReadFile(resolvedPath)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	check.Exists = true
	var root struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		check.Error = err.Error()
		return check
	}
	server, ok := root.MCPServers["hostshift"]
	if !ok {
		check.Error = "missing mcpServers.hostshift"
		return check
	}
	check.Server = "hostshift"
	check.Command = server.Command
	check.Args = server.Args
	if server.Command == "" {
		check.Error = "missing hostshift command"
		return check
	}
	if len(server.Args) != 2 || server.Args[0] != "mcp" || server.Args[1] != "stdio" {
		check.Error = "hostshift args must be [\"mcp\", \"stdio\"]"
		return check
	}
	check.Valid = true
	return check
}

func resolveRepoRelativePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	cwd, err := os.Getwd()
	if err != nil {
		return path
	}
	for {
		candidate := filepath.Join(cwd, path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			return path
		}
		cwd = parent
	}
}

func cliTool(name, title, description string, schema map[string]any, buildArgs func(map[string]any) []string) mcp.Tool {
	return mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		InputSchema: schema,
		Handler: func(ctx context.Context, args map[string]any) (text string, err error) {
			defer func() {
				if recovered := recover(); recovered != nil {
					err = fmt.Errorf("%v", recovered)
				}
			}()
			cliArgs := append(buildArgs(args), "--json")
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if err := Run(ctx, cliArgs, &stdout, &stderr); err != nil {
				if stderr.Len() > 0 {
					return "", fmt.Errorf("%w: %s", err, stderr.String())
				}
				return "", err
			}
			return stdout.String(), nil
		},
	}
}

func phaseTool(name, title, description, command string) mcp.Tool {
	return cliTool(name, title, description, objectSchema(map[string]any{
		"profile": stringSchema("Profile path."),
		"target":  stringSchema("Optional target SSH alias override."),
	}, "profile"), func(args map[string]any) []string {
		out := []string{command, "--profile", requiredString(args, "profile")}
		if target := optionalString(args, "target"); target != "" {
			out = append(out, "--target", target)
		}
		return out
	})
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func requiredString(args map[string]any, key string) string {
	value := optionalString(args, key)
	if value == "" {
		panic(fmt.Sprintf("missing required argument: %s", key))
	}
	return value
}

func optionalString(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	panic(fmt.Sprintf("argument %s must be a string", key))
}
