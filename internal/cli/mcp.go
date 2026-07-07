package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/oguzhankaracabay/hostshift/internal/mcp"
	"github.com/oguzhankaracabay/hostshift/internal/version"
)

func ServeMCP(ctx context.Context, stdin io.Reader, stdout io.Writer) error {
	server := mcp.Server{
		Name:         "hostshift",
		Title:        "HostShift",
		Version:      version.Version,
		Instructions: "HostShift exposes read-only-source server migration planning and explanation tools. MCP tools do not expose --apply; target mutations require the human-operated CLI.",
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
			phaseTool("hostshift_prepare_dry_run", "HostShift Prepare Dry Run", "Show target preparation actions without applying them.", "prepare"),
			phaseTool("hostshift_sync_dry_run", "HostShift Sync Dry Run", "Show source-to-target streams without applying them.", "sync"),
			phaseTool("hostshift_verify_dry_run", "HostShift Verify Dry Run", "Show target verification checks without applying them.", "verify"),
			phaseTool("hostshift_cutover_dry_run", "HostShift Cutover Dry Run", "Show target-only cutover actions and confirmation code without applying them.", "cutover"),
			cliTool("hostshift_rollback", "HostShift Rollback Metadata", "Report manual rollback guidance and target rollback metadata. The source is never changed.", objectSchema(map[string]any{
				"profile": stringSchema("Profile path."),
			}, "profile"), func(args map[string]any) []string {
				return []string{"rollback", "--profile", requiredString(args, "profile")}
			}),
		},
	}
	return mcp.Serve(ctx, server, stdin, stdout)
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
