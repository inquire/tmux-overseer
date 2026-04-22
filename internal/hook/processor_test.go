package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessPreToolUse(t *testing.T) {
	dir := t.TempDir()
	input := `{"session_id":"test","hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"path":"/tmp/x"}}`

	err := Process([]byte(input), dir, "%99")
	if err != nil {
		t.Fatal(err)
	}

	// Check status file
	statusFile := filepath.Join(dir, "status-_99.json")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("status file not written: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("status file invalid JSON: %v", err)
	}
	if m["status"] != "working" {
		t.Errorf("status = %v, want working", m["status"])
	}
	if m["last_tool"] != "Read" {
		t.Errorf("last_tool = %v, want Read", m["last_tool"])
	}

	// Check events file
	eventsFile := filepath.Join(dir, "status-_99.events.jsonl")
	evData, err := os.ReadFile(eventsFile)
	if err != nil {
		t.Fatalf("events file not written: %v", err)
	}
	var ev map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(evData))), &ev); err != nil {
		t.Fatalf("event invalid JSON: %v\n%s", err, evData)
	}
	if ev["type"] != "tool_start" {
		t.Errorf("event type = %v, want tool_start", ev["type"])
	}

	// Check counters
	countersFile := filepath.Join(dir, "status-_99.counters")
	cData, _ := os.ReadFile(countersFile)
	if !strings.Contains(string(cData), "TOOL_COUNT=1") {
		t.Errorf("counters missing TOOL_COUNT=1:\n%s", cData)
	}
}

func TestProcessAskQuestion(t *testing.T) {
	dir := t.TempDir()
	input := `{"session_id":"test","hook_event_name":"PreToolUse","tool_name":"AskQuestion","tool_input":{}}`

	if err := Process([]byte(input), dir, "%5"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "status-_5.json"))
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	if m["status"] != "waiting" {
		t.Errorf("AskQuestion status = %v, want waiting", m["status"])
	}
}

func TestProcessSessionEnd(t *testing.T) {
	dir := t.TempDir()
	pane := "_99"
	for _, suffix := range []string{".json", ".events.jsonl", ".counters", ".subagents.json"} {
		_ = os.WriteFile(filepath.Join(dir, "status-"+pane+suffix), []byte("x"), 0o644)
	}

	input := `{"session_id":"test","hook_event_name":"SessionEnd"}`
	if err := Process([]byte(input), dir, "%99"); err != nil {
		t.Fatal(err)
	}

	for _, suffix := range []string{".json", ".events.jsonl", ".counters", ".subagents.json"} {
		if _, err := os.Stat(filepath.Join(dir, "status-"+pane+suffix)); err == nil {
			t.Errorf("file status-%s%s should have been deleted", pane, suffix)
		}
	}
}

func TestProcessSessionStart(t *testing.T) {
	dir := t.TempDir()
	input := `{"session_id":"test","hook_event_name":"SessionStart","model":"opus","cwd":"/tmp"}`

	if err := Process([]byte(input), dir, "%1"); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "status-_1.json"))
	var m map[string]interface{}
	_ = json.Unmarshal(data, &m)
	if m["status"] != "idle" {
		t.Errorf("status = %v, want idle", m["status"])
	}

	// Subagent file should be initialized
	saData, _ := os.ReadFile(filepath.Join(dir, "status-_1.subagents.json"))
	if string(saData) != "[]" {
		t.Errorf("subagents = %s, want []", saData)
	}
}

func TestProcessFallbackSessionID(t *testing.T) {
	dir := t.TempDir()
	input := `{"session_id":"abc-123","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`

	// No tmux pane — should use session ID
	if err := Process([]byte(input), dir, ""); err != nil {
		t.Fatal(err)
	}

	statusFile := filepath.Join(dir, "status-session-abc-123.json")
	if _, err := os.Stat(statusFile); err != nil {
		t.Errorf("expected status file at %s", statusFile)
	}
}

func TestSanitizePaneID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"%17", "_17"},
		{"simple", "simple"},
		{"a/b c", "a_b_c"},
	}
	for _, tt := range tests {
		got := sanitizePaneID(tt.input)
		if got != tt.want {
			t.Errorf("sanitizePaneID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractToolInputSummary(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"path", `{"path":"/tmp/foo"}`, "/tmp/foo"},
		{"command", `{"command":"ls -la"}`, "ls -la"},
		{"empty", `{}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolInputSummary(json.RawMessage(tt.input))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSubagentStartDescriptionFromPrecedingAgentTool simulates the real Claude Code
// sequence: PreToolUse(Agent) fires first with the description in tool_input, then
// SubagentStart fires with an empty description field.
func TestSubagentStartDescriptionFromPrecedingAgentTool(t *testing.T) {
	dir := t.TempDir()
	pane := "%7"

	// Step 1: PreToolUse for Agent tool — caches description to pending file
	preToolUse := `{
		"session_id":"test",
		"hook_event_name":"PreToolUse",
		"tool_name":"Agent",
		"tool_input":{"description":"Explore example codebase structure","subagent_type":"Explore","prompt":"find all Go files"}
	}`
	if err := Process([]byte(preToolUse), dir, pane); err != nil {
		t.Fatal(err)
	}

	// Step 2: SubagentStart fires with empty description — should pick up the cached value
	subagentStart := `{
		"session_id":"test",
		"hook_event_name":"SubagentStart",
		"agent_id":"agent-abc",
		"agent_type":"Explore"
	}`
	if err := Process([]byte(subagentStart), dir, pane); err != nil {
		t.Fatal(err)
	}

	list, _ := readSubagentList(filepath.Join(dir, "status-_7.subagents.json"))
	if len(list) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(list))
	}
	if list[0].Description != "Explore example codebase structure" {
		t.Errorf("description = %q, want %q", list[0].Description, "Explore example codebase structure")
	}

	// Pending file should be consumed
	if _, err := os.Stat(filepath.Join(dir, "status-_7.pending-agent-desc")); err == nil {
		t.Error("pending-agent-desc file should have been deleted after SubagentStart")
	}
}

// TestSubagentStartDescriptionFallback verifies that when SubagentStart arrives
// without a top-level description, the description is pulled from tool_input.
func TestSubagentStartDescriptionFallback(t *testing.T) {
	dir := t.TempDir()
	// Simulate what Claude Code sends: agent_type present, description absent at
	// top level but present inside tool_input.
	input := `{
		"session_id":"test",
		"hook_event_name":"SubagentStart",
		"agent_id":"agent-abc",
		"agent_type":"Explore",
		"tool_input":{"description":"Explore example codebase structure","prompt":"find auth code"}
	}`

	if err := Process([]byte(input), dir, "%5"); err != nil {
		t.Fatal(err)
	}

	list, _ := readSubagentList(filepath.Join(dir, "status-_5.subagents.json"))
	if len(list) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(list))
	}
	if list[0].Description != "Explore example codebase structure" {
		t.Errorf("description = %q, want %q", list[0].Description, "Explore example codebase structure")
	}
	if list[0].AgentType != "Explore" {
		t.Errorf("agent_type = %q, want Explore", list[0].AgentType)
	}
}

// TestSubagentStartDescriptionPromptFallback verifies fallback to tool_input.prompt
// when neither top-level description nor tool_input.description is present.
func TestSubagentStartDescriptionPromptFallback(t *testing.T) {
	dir := t.TempDir()
	input := `{
		"session_id":"test",
		"hook_event_name":"SubagentStart",
		"agent_id":"agent-xyz",
		"agent_type":"general",
		"tool_input":{"prompt":"search the codebase for auth patterns"}
	}`

	if err := Process([]byte(input), dir, "%6"); err != nil {
		t.Fatal(err)
	}

	list, _ := readSubagentList(filepath.Join(dir, "status-_6.subagents.json"))
	if len(list) != 1 {
		t.Fatalf("expected 1 subagent, got %d", len(list))
	}
	if list[0].Description == "" {
		t.Error("description should not be empty when tool_input.prompt is present")
	}
}

func TestInferAgentType(t *testing.T) {
	tests := []struct {
		desc string
		want string
	}{
		{"Exploring the codebase", "explore"},
		{"Running shell commands", "shell"},
		{"Review PR changes", "code-reviewer"},
		{"something random", "general"},
	}
	for _, tt := range tests {
		got := inferAgentType(tt.desc)
		if got != tt.want {
			t.Errorf("inferAgentType(%q) = %q, want %q", tt.desc, got, tt.want)
		}
	}
}
