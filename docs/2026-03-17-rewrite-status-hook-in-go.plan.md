---
name: Rewrite status-hook in Go
overview: "In `tmux-overseer`, replace the bash status-hook.sh with a compiled Go binary to reduce per-tool-call hook latency from ~60ms to <10ms, eliminating UI flicker in Claude Code."
tags: ["performance", "refactor"]
todos:
  - id: hook-binary-cmd
    content: Create cmd/claude-hook/main.go that reads JSON from stdin, parses it, and dispatches to event handlers
    status: pending
  - id: hook-processor
    content: Create internal/hook/processor.go with event routing, status/event/counter file writing
    status: pending
  - id: hook-json-escape
    content: Create internal/hook/json.go with fast JSON event builder using string concatenation
    status: pending
  - id: hook-subagent
    content: Create internal/hook/subagent.go for SubagentStart/Stop list management
    status: pending
  - id: hook-tests
    content: Write tests for processor, JSON builder, and subagent management
    status: pending
  - id: hook-build-install
    content: Add claude-hook target to Makefile, update setup-hooks.sh to point to the binary
    status: pending
  - id: hook-benchmark
    content: Benchmark the Go binary vs bash script, verify <10ms for PreToolUse
    status: pending
---

# Rewrite status-hook in Go

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

## What We're Building

A compiled Go binary (`claude-hook`) that replaces `scripts/status-hook.sh`. It reads hook JSON from stdin, writes the same status files (`~/.claude-tmux/status-*.json`, `*.events.jsonl`, `*.counters`, `*.subagents.json`), and forwards events to the hookserver — but in ~5-10ms instead of ~60ms because there are zero process spawns (no jq, no bash subshells).

The binary is a second cmd in the tmux-overseer module, sharing internal packages. It produces byte-identical output files so the TUI reads them without changes.

**Architecture:** Single `main.go` reads stdin, unmarshals JSON once, routes to event handler, writes files with `os.WriteFile`. No external dependencies beyond stdlib. The existing `internal/state` package provides `StatusDir()`.

**Tech Stack:** Go stdlib only (encoding/json, os, fmt, time, path/filepath, net/http, syscall)

---

## File Structure

```
cmd/
├── tmux-overseer/main.go          (existing TUI — no changes)
└── claude-hook/main.go             (NEW — hook binary entry point)
internal/
└── hook/                           (NEW — hook processing logic)
    ├── processor.go                Event routing + file writing
    ├── processor_test.go           Tests
    ├── json.go                     Fast JSON event builder (no encoding/json for output)
    ├── json_test.go                Tests
    ├── subagent.go                 Subagent list management
    └── subagent_test.go            Tests
scripts/
├── status-hook.sh                  (KEEP as fallback, no changes)
└── setup-hooks.sh                  (MODIFY — point to Go binary, fall back to bash)
Makefile                            (MODIFY — add claude-hook target)
```

---

### Task 1: Hook Input Types + JSON Builder

**Files:**
- Create: `internal/hook/json.go`
- Test: `internal/hook/json_test.go`

- [ ] **Step 1: Write the test**

```go
package hook

import (
	"encoding/json"
	"testing"
)

func TestJSONEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello`, `hello`},
		{`say "hi"`, `say \"hi\"`},
		{"line\nnew", `line\nnew`},
		{`back\slash`, `back\\slash`},
		{"tab\there", `tab\there`},
	}
	for _, tt := range tests {
		got := jsonEscape(tt.input)
		if got != tt.want {
			t.Errorf("jsonEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildEventJSON(t *testing.T) {
	raw := buildEventJSON("12:30:00", "tool_start", map[string]string{
		"tool":  "Read",
		"input": "/tmp/test",
	})
	// Must be valid JSON
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if m["ts"] != "12:30:00" {
		t.Errorf("ts = %v", m["ts"])
	}
	if m["tool"] != "Read" {
		t.Errorf("tool = %v", m["tool"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ~/go/src/tmux-overseer && go test ./internal/hook/ -v -run TestJSON`

- [ ] **Step 3: Write implementation**

```go
package hook

import "strings"

// jsonEscape escapes a string for embedding in a JSON value (no quotes added).
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// buildEventJSON builds a JSON object string from ts, type, and extra key-value pairs.
// Uses string concatenation — faster than encoding/json for simple flat objects.
func buildEventJSON(ts, eventType string, extra map[string]string) string {
	var b strings.Builder
	b.WriteString(`{"ts":"`)
	b.WriteString(jsonEscape(ts))
	b.WriteString(`","type":"`)
	b.WriteString(jsonEscape(eventType))
	b.WriteByte('"')
	for k, v := range extra {
		b.WriteString(`,"`)
		b.WriteString(k)
		b.WriteString(`":"`)
		b.WriteString(jsonEscape(v))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// appendAgentTag adds agent_id and agent_type fields to a JSON string.
func appendAgentTag(eventJSON, agentID, agentType string) string {
	if agentID == "" {
		return eventJSON
	}
	// Replace trailing } with extra fields
	return eventJSON[:len(eventJSON)-1] +
		`,"agent_id":"` + jsonEscape(agentID) +
		`","agent_type":"` + jsonEscape(agentType) + `"}`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ~/go/src/tmux-overseer && go test ./internal/hook/ -v`

- [ ] **Step 5: Commit**

```bash
git add internal/hook/json.go internal/hook/json_test.go
git commit -m "feat(hook): add fast JSON event builder for Go hook binary"
```

---

### Task 2: Subagent List Management

**Files:**
- Create: `internal/hook/subagent.go`
- Test: `internal/hook/subagent_test.go`

- [ ] **Step 1: Write the test**

```go
package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubagentAdd(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "subagents.json")
	os.WriteFile(fp, []byte("[]"), 0o644)

	count, err := subagentAdd(fp, SubagentEntry{
		ID: "a1", AgentType: "explore", Description: "searching",
		Model: "opus", StartedAt: "12:00", Status: "working",
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestSubagentRemove(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "subagents.json")
	os.WriteFile(fp, []byte(`[{"id":"a1"},{"id":"a2"}]`), 0o644)

	count, err := subagentRemove(fp, "a1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}
```

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Write implementation**

```go
package hook

import (
	"encoding/json"
	"os"
)

type SubagentEntry struct {
	ID            string `json:"id"`
	AgentType     string `json:"agent_type"`
	Description   string `json:"description"`
	Model         string `json:"model"`
	StartedAt     string `json:"started_at"`
	Status        string `json:"status"`
	ParentAgentID string `json:"parent_agent_id,omitempty"`
}

func readSubagentList(path string) ([]SubagentEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var list []SubagentEntry
	json.Unmarshal(data, &list)
	return list, nil
}

func subagentAdd(path string, entry SubagentEntry) (int, error) {
	list, _ := readSubagentList(path)
	list = append(list, entry)
	data, _ := json.Marshal(list)
	return len(list), os.WriteFile(path, data, 0o644)
}

func subagentRemove(path string, agentID string) (int, error) {
	list, _ := readSubagentList(path)
	var filtered []SubagentEntry
	for _, e := range list {
		if e.ID != agentID {
			filtered = append(filtered, e)
		}
	}
	if filtered == nil {
		filtered = []SubagentEntry{}
	}
	data, _ := json.Marshal(filtered)
	return len(filtered), os.WriteFile(path, data, 0o644)
}

func subagentCount(path string) int {
	list, _ := readSubagentList(path)
	return len(list)
}
```

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/hook/subagent.go internal/hook/subagent_test.go
git commit -m "feat(hook): add subagent list management"
```

---

### Task 3: Main Processor (Event Router + File Writer)

**Files:**
- Create: `internal/hook/processor.go`
- Test: `internal/hook/processor_test.go`

- [ ] **Step 1: Write the test**

```go
package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProcessPreToolUse(t *testing.T) {
	dir := t.TempDir()
	input := `{"session_id":"test","hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"path":"/tmp/x"}}`

	err := Process([]byte(input), dir, "%99")
	if err != nil {
		t.Fatal(err)
	}

	// Check status file was written
	statusFile := filepath.Join(dir, "status-_99_.json")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("status file not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("status file is empty")
	}

	// Check events file was appended
	eventsFile := filepath.Join(dir, "status-_99_.events.jsonl")
	data, err = os.ReadFile(eventsFile)
	if err != nil {
		t.Fatalf("events file not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("events file is empty")
	}
}

func TestProcessSessionEnd(t *testing.T) {
	dir := t.TempDir()
	// Create files that should be cleaned up
	pane := "_99_"
	for _, suffix := range []string{".json", ".events.jsonl", ".counters", ".subagents.json"} {
		os.WriteFile(filepath.Join(dir, "status-"+pane+suffix), []byte("x"), 0o644)
	}

	input := `{"session_id":"test","hook_event_name":"SessionEnd"}`
	Process([]byte(input), dir, "%99")

	// All files should be deleted
	for _, suffix := range []string{".json", ".events.jsonl", ".counters", ".subagents.json"} {
		if _, err := os.Stat(filepath.Join(dir, "status-"+pane+suffix)); err == nil {
			t.Errorf("file status-%s%s should have been deleted", pane, suffix)
		}
	}
}
```

- [ ] **Step 2: Run test, verify fail**

- [ ] **Step 3: Write implementation**

`processor.go` — the core: unmarshal stdin JSON once into a struct, switch on event name, write files. This is a direct port of `status-hook.sh` logic but with zero process spawns.

Key struct:
```go
type HookInput struct {
	SessionID        string          `json:"session_id"`
	HookEvent        string          `json:"hook_event_name"`
	Model            string          `json:"model"`
	CWD              string          `json:"cwd"`
	TranscriptPath   string          `json:"transcript_path"`
	PermissionMode   string          `json:"permission_mode"`
	ToolName         string          `json:"tool_name"`
	ToolInput        json.RawMessage `json:"tool_input"`
	ToolOutput       json.RawMessage `json:"tool_output"`
	Error            string          `json:"error"`
	StopReason       string          `json:"stop_hook_reason"`
	Prompt           string          `json:"prompt"`
	AgentID          string          `json:"agent_id"`
	AgentType        string          `json:"agent_type"`
	ParentAgentID    string          `json:"parent_agent_id"`
	Description      string          `json:"description"`
	LastAssistantMsg string          `json:"last_assistant_message"`
	Effort           string          `json:"effort"`
	Message          string          `json:"message"`
	Worktree         struct {
		Name         string `json:"name"`
		Path         string `json:"path"`
		Branch       string `json:"branch"`
		OriginalRepo string `json:"originalRepo"`
	} `json:"worktree"`
}
```

Key function:
```go
func Process(input []byte, statusDir, tmuxPane string) error
```

Logic mirrors status-hook.sh exactly:
1. Unmarshal input once
2. Resolve pane ID (TMUX_PANE env or session_id fallback)
3. Load counters file (key=value format)
4. Switch on event → set status, build event JSON, update counters
5. Save counters
6. Write status JSON (using fmt.Fprintf, not encoding/json)
7. Append event to JSONL file, prune if >200 lines
8. Fire-and-forget POST to hookserver if port file exists

- [ ] **Step 4: Run test, verify pass**

- [ ] **Step 5: Commit**

```bash
git add internal/hook/processor.go internal/hook/processor_test.go
git commit -m "feat(hook): add main event processor with file writing"
```

---

### Task 4: Binary Entry Point

**Files:**
- Create: `cmd/claude-hook/main.go`

- [ ] **Step 1: Write main.go**

```go
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/inquire/tmux-overseer/internal/hook"
)

func main() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}

	statusDir := os.Getenv("HOME") + "/.claude-tmux"
	os.MkdirAll(statusDir, 0o755)

	tmuxPane := os.Getenv("TMUX_PANE")

	if err := hook.Process(input, statusDir, tmuxPane); err != nil {
		fmt.Fprintf(os.Stderr, "hook error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Build**

Run: `cd ~/go/src/tmux-overseer && go build -o claude-hook ./cmd/claude-hook`

- [ ] **Step 3: Commit**

```bash
git add cmd/claude-hook/
git commit -m "feat(hook): add claude-hook binary entry point"
```

---

### Task 5: Makefile + Setup Script

**Files:**
- Modify: `Makefile`
- Modify: `scripts/setup-hooks.sh`

- [ ] **Step 1: Add claude-hook target to Makefile**

Add after the existing `build` target:
```makefile
hook:
	@echo "Building claude-hook..."
	go build -trimpath -ldflags '-s -w' -o claude-hook ./cmd/claude-hook

install-hook: hook
	@echo "Installing claude-hook..."
	cp claude-hook $(shell go env GOPATH)/bin/claude-hook

install-all: install install-hook
```

Update `clean`:
```makefile
clean:
	rm -f $(BIN_NAME) claude-hook
```

- [ ] **Step 2: Update setup-hooks.sh to prefer Go binary**

Change the hook command resolution to prefer `claude-hook` in PATH, fall back to bash script:
```bash
# Prefer compiled Go hook binary (fast), fall back to bash script
if command -v claude-hook &>/dev/null; then
    HOOKS_SCRIPT="$(command -v claude-hook)"
    echo "Using compiled Go hook binary: $HOOKS_SCRIPT"
else
    HOOKS_SCRIPT="$SCRIPT_DIR/status-hook.sh"
    echo "Using bash hook script: $HOOKS_SCRIPT (install claude-hook for better performance)"
fi
```

- [ ] **Step 3: Build and verify**

```bash
cd ~/go/src/tmux-overseer && make hook
echo '{"session_id":"t","hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"path":"/tmp/t"}}' | time ./claude-hook
```

Target: <10ms

- [ ] **Step 4: Commit**

```bash
git add Makefile scripts/setup-hooks.sh
git commit -m "feat(hook): add Makefile target and setup script for Go hook binary"
```

---

### Task 6: Benchmark + Re-register Hooks

- [ ] **Step 1: Benchmark Go vs bash**

```bash
cd ~/go/src/tmux-overseer

# Go binary
echo '{"session_id":"t","hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"path":"/tmp/t"}}' | time ./claude-hook

# Bash script
echo '{"session_id":"t","hook_event_name":"PreToolUse","tool_name":"Read","tool_input":{"path":"/tmp/t"}}' | time bash scripts/status-hook.sh
```

Expected: Go <10ms, bash ~58ms

- [ ] **Step 2: Install and re-register hooks**

```bash
make install-hook
bash scripts/setup-hooks.sh
```

- [ ] **Step 3: Verify in Claude Code**

Start a Claude Code session, confirm no flicker, check status files are being written correctly.

- [ ] **Step 4: Commit**

```bash
git add -A && git commit -m "chore: benchmark and finalize Go hook binary"
```

## Verification

1. `go test ./internal/hook/ -v` — all tests pass
2. `make hook` — builds without errors
3. PreToolUse benchmark: `echo '...' | time ./claude-hook` — <10ms wall clock
4. PostToolUse benchmark: same — <10ms
5. Status files match format: `jq . ~/.claude-tmux/status-*.json` — valid JSON
6. Events file: `tail -1 ~/.claude-tmux/status-*.events.jsonl | jq .` — valid JSON
7. Counters file: `cat ~/.claude-tmux/status-*.counters` — valid key=value pairs
8. SessionEnd: files cleaned up
9. No flicker in Claude Code during rapid tool use
10. Bash fallback: remove `claude-hook` from PATH, re-run setup — falls back to bash script
