package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPListsSafeHostShiftTools(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(input), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	if responses[0]["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
		t.Fatalf("unexpected initialize response: %+v", responses[0])
	}
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		names[tool["name"].(string)] = true
	}
	for _, name := range []string{"hostshift_doctor", "hostshift_discover", "hostshift_plan", "hostshift_explain", "hostshift_review", "hostshift_prepare_dry_run", "hostshift_sync_dry_run", "hostshift_verify_dry_run", "hostshift_cutover_dry_run", "hostshift_rollback"} {
		if !names[name] {
			t.Fatalf("missing MCP tool %s in %+v", name, names)
		}
	}
	for name := range names {
		if strings.Contains(name, "apply") {
			t.Fatalf("MCP must not expose apply tools: %+v", names)
		}
	}
}

func TestMCPExplainToolCallsGoCLI(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: mcp-explain
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
approved: false
`)
	if err := os.WriteFile(profilePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hostshift_explain",
			"arguments": map[string]any{
				"profile": profilePath,
			},
		},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(string(encoded)+"\n"), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("expected successful tool result: %+v", result)
	}
	content := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, `"summary"`) || !strings.Contains(content, `"sourceWillBeModified": false`) {
		t.Fatalf("expected explain JSON in MCP response: %s", content)
	}
}

func TestMCPReviewToolCallsGoCLI(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: mcp-review
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
approved: false
`)
	if err := os.WriteFile(profilePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hostshift_review",
			"arguments": map[string]any{
				"profile": profilePath,
			},
		},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(string(encoded)+"\n"), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("expected successful tool result: %+v", result)
	}
	content := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, `"operatorChecklist"`) || !strings.Contains(content, `"sourceWillBeModified": false`) {
		t.Fatalf("expected review JSON in MCP response: %s", content)
	}
}

func TestMCPPlanToolCallsGoCLI(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: mcp-plan
source:
  ssh: old-server
target:
  ssh: new-server
sourcePolicy: strict-read-only
approved: false
`)
	if err := os.WriteFile(profilePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hostshift_plan",
			"arguments": map[string]any{
				"profile": profilePath,
			},
		},
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(string(encoded)+"\n"), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	result := responses[0]["result"].(map[string]any)
	if result["isError"] != false {
		t.Fatalf("expected successful tool result: %+v", result)
	}
	content := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(content, `"sourceWillBeModified": false`) {
		t.Fatalf("expected plan JSON in MCP response: %s", content)
	}
}

func TestMCPDoctorReportsClaudeConfigAndSafeToolSurface(t *testing.T) {
	var stdout strings.Builder
	if err := Run(context.Background(), []string{"mcp", "doctor", "--json"}, &stdout, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(stdout.String()), &report); err != nil {
		t.Fatalf("invalid doctor JSON: %v\n%s", err, stdout.String())
	}
	if report["status"] != "ok" {
		t.Fatalf("expected ok status: %+v", report)
	}
	if report["applyToolsExposed"] != false || report["sourceWillBeModified"] != false {
		t.Fatalf("MCP doctor must keep apply tools hidden and source immutable: %+v", report)
	}
	claude := report["claudeConfig"].(map[string]any)
	if claude["valid"] != true || claude["server"] != "hostshift" {
		t.Fatalf("expected valid Claude config check: %+v", claude)
	}
	args := claude["args"].([]any)
	if len(args) != 2 || args[0] != "mcp" || args[1] != "stdio" {
		t.Fatalf("unexpected Claude MCP args: %+v", args)
	}
	tools := report["tools"].([]any)
	for _, raw := range tools {
		name := raw.(string)
		if strings.Contains(name, "apply") {
			t.Fatalf("MCP doctor exposed apply tool: %s", name)
		}
	}
	if report["requiredToolsPresent"] != true {
		t.Fatalf("expected required tools to be present: %+v", report)
	}
}

func decodeMCPResponses(t *testing.T, output string) []map[string]any {
	t.Helper()
	scanner := bufio.NewScanner(strings.NewReader(output))
	responses := []map[string]any{}
	for scanner.Scan() {
		var response map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &response); err != nil {
			t.Fatalf("invalid MCP JSON response %q: %v", scanner.Text(), err)
		}
		responses = append(responses, response)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(responses) == 0 {
		t.Fatal("expected MCP responses")
	}
	return responses
}
