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
	capabilities := responses[0]["result"].(map[string]any)["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"]; !ok {
		t.Fatalf("initialize response missing tools capability: %+v", responses[0])
	}
	if _, ok := capabilities["prompts"]; !ok {
		t.Fatalf("initialize response missing prompts capability: %+v", responses[0])
	}
	if _, ok := capabilities["resources"]; !ok {
		t.Fatalf("initialize response missing resources capability: %+v", responses[0])
	}
	tools := responses[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, raw := range tools {
		tool := raw.(map[string]any)
		names[tool["name"].(string)] = true
	}
	for _, name := range []string{"hostshift_doctor", "hostshift_discover", "hostshift_plan", "hostshift_explain", "hostshift_review", "hostshift_prepare_dry_run", "hostshift_sync_dry_run", "hostshift_verify_dry_run", "hostshift_cutover_dry_run", "hostshift_status", "hostshift_resume_dry_run", "hostshift_profile_migrate", "hostshift_policy_source", "hostshift_capabilities", "hostshift_rollback"} {
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

func TestMCPListsAndReadsHostShiftResources(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"hostshift://source-safety"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"hostshift://capabilities"}}`,
	}, "\n") + "\n"
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(input), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	resources := responses[0]["result"].(map[string]any)["resources"].([]any)
	found := map[string]bool{}
	for _, raw := range resources {
		resource := raw.(map[string]any)
		found[resource["uri"].(string)] = true
	}
	for _, uri := range []string{"hostshift://source-safety", "hostshift://migration-workflow", "hostshift://capabilities"} {
		if !found[uri] {
			t.Fatalf("expected resource %s in %+v", uri, resources)
		}
	}
	sourceSafety := responses[1]["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)["text"].(string)
	for _, expected := range []string{
		"immutable observation endpoint",
		"Forbidden source behavior",
		"service start, stop, restart, reload",
	} {
		if !strings.Contains(sourceSafety, expected) {
			t.Fatalf("expected source safety resource to contain %q: %s", expected, sourceSafety)
		}
	}
	capabilities := responses[2]["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)["text"].(string)
	for _, expected := range []string{
		`"sourceWillBeModified": false`,
		`"applyToolsExposed": false`,
		`"type": "docker-compose"`,
		`"memcachedConfigPaths"`,
	} {
		if !strings.Contains(capabilities, expected) {
			t.Fatalf("expected capabilities resource to contain %q: %s", expected, capabilities)
		}
	}
}

func TestMCPListsAndGetsHostShiftPrompt(t *testing.T) {
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"prompts/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"prompts/get","params":{"name":"hostshift_migration_operator"}}`,
	}, "\n") + "\n"
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(input), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	prompts := responses[0]["result"].(map[string]any)["prompts"].([]any)
	found := false
	for _, raw := range prompts {
		prompt := raw.(map[string]any)
		if prompt["name"] == "hostshift_migration_operator" {
			found = true
			if !strings.Contains(prompt["description"].(string), "read-only source") {
				t.Fatalf("expected safety-oriented prompt description: %+v", prompt)
			}
		}
	}
	if !found {
		t.Fatalf("expected hostshift_migration_operator prompt in %+v", prompts)
	}
	getResult := responses[1]["result"].(map[string]any)
	messages := getResult["messages"].([]any)
	if len(messages) != 1 {
		t.Fatalf("expected one prompt message: %+v", getResult)
	}
	content := messages[0].(map[string]any)["content"].(map[string]any)["text"].(string)
	for _, expected := range []string{
		"strictly read-only observation endpoint",
		"hostshift_capabilities",
		"Do not run arbitrary source SSH commands.",
		"A human operator must run CLI --apply",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected prompt to contain %q: %s", expected, content)
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

func TestMCPStatusAndResumeToolsRemainDryRunOnly(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "profile.yaml")
	body := []byte(`schemaVersion: 2
name: mcp-resume
source:
  ssh: old-server
target:
  ssh: new-server
platforms:
  source: ubuntu:24.04
  target: ubuntu:24.04
sourcePolicy: strict-read-only
approved: true
workloads:
  - type: file-set
    name: app
    data:
      paths: [/srv/app]
      targetPath: /srv
`)
	if err := os.WriteFile(profilePath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"prepare", "--profile", profilePath, "--state-dir", dir, "--run-id", "mcp-resume"}, &strings.Builder{}, &strings.Builder{}); err != nil {
		t.Fatal(err)
	}
	requests := []map[string]any{
		{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "hostshift_status",
				"arguments": map[string]any{
					"runId":    "mcp-resume",
					"stateDir": dir,
				},
			},
		},
		{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/call",
			"params": map[string]any{
				"name": "hostshift_resume_dry_run",
				"arguments": map[string]any{
					"profile":  profilePath,
					"runId":    "mcp-resume",
					"stateDir": dir,
				},
			},
		},
	}
	var input strings.Builder
	for _, request := range requests {
		encoded, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		input.Write(encoded)
		input.WriteByte('\n')
	}
	var stdout strings.Builder
	if err := ServeMCP(context.Background(), strings.NewReader(input.String()), &stdout); err != nil {
		t.Fatal(err)
	}
	responses := decodeMCPResponses(t, stdout.String())
	statusText := responses[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(statusText, `"status": "dry-run"`) || !strings.Contains(statusText, `"planHash"`) {
		t.Fatalf("expected saved state from status tool: %s", statusText)
	}
	resumeText := responses[1]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	for _, expected := range []string{`"resumed": true`, `"apply": false`, `"sourceWillBeModified": false`} {
		if !strings.Contains(resumeText, expected) {
			t.Fatalf("expected resume dry-run output to contain %q: %s", expected, resumeText)
		}
	}
}

func TestMCPPolicySourceToolCallsGoCLI(t *testing.T) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "hostshift_policy_source",
			"arguments": map[string]any{},
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
	if !strings.Contains(content, `"sourcePolicy": "strict-read-only"`) || !strings.Contains(content, `"sourceWillBeModified": false`) {
		t.Fatalf("expected source policy JSON in MCP response: %s", content)
	}
}

func TestMCPCapabilitiesToolCallsGoCLI(t *testing.T) {
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "hostshift_capabilities",
			"arguments": map[string]any{},
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
	for _, expected := range []string{
		`"sourceWillBeModified": false`,
		`"applyToolsExposed": false`,
		`"type": "memcached"`,
		`"type": "docker-compose"`,
		`"type": "serviceActive"`,
		`"memcachedConfigPaths"`,
		`"name": "memcached"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected capabilities JSON to contain %q: %s", expected, content)
		}
	}
}

func TestMCPProfileMigrateToolCallsGoCLI(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "profile.v1.json")
	outputPath := filepath.Join(dir, "profile.v2.yaml")
	body := []byte(`{
  "schemaVersion": 1,
  "name": "mcp-migrate",
  "source": {"ssh": "old-server", "policy": "strict-read-only"},
  "target": {"ssh": "new-server"},
  "fileSets": [{"name": "app-files", "paths": ["/srv/app"], "targetPath": "/"}],
  "approved": false
}`)
	if err := os.WriteFile(inputPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "hostshift_profile_migrate",
			"arguments": map[string]any{
				"input":  inputPath,
				"output": outputPath,
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
	if !strings.Contains(content, `"schemaVersion": 2`) || !strings.Contains(content, `"sourceWillBeModified": false`) {
		t.Fatalf("expected profile migrate JSON in MCP response: %s", content)
	}
	migrated, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(migrated), "schemaVersion: 2") || !strings.Contains(string(migrated), "type: file-set") {
		t.Fatalf("expected migrated v2 profile, got: %s", string(migrated))
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
	if report["requiredPromptsPresent"] != true {
		t.Fatalf("expected required prompts to be present: %+v", report)
	}
	prompts := report["prompts"].([]any)
	if len(prompts) != 1 || prompts[0] != "hostshift_migration_operator" {
		t.Fatalf("expected migration operator prompt in doctor report: %+v", report)
	}
	if report["requiredResourcesPresent"] != true {
		t.Fatalf("expected required resources to be present: %+v", report)
	}
	resources := report["resources"].([]any)
	if len(resources) != 3 {
		t.Fatalf("expected three MCP resources in doctor report: %+v", report)
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
