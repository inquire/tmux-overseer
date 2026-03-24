package hook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HookInput is the JSON payload sent by Claude Code on each hook event.
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

// counters holds the persistent counters loaded from the .counters file.
type counters struct {
	PromptCount    int
	ToolCount      int
	SessionStartTS int64
	AgentMode      string
	SubagentCount  int
}

// Process handles a single hook invocation. This is the main entry point.
func Process(input []byte, statusDir, tmuxPane string) error {
	var h HookInput
	if err := json.Unmarshal(input, &h); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	// Resolve pane ID
	paneID := tmuxPane
	if paneID == "" {
		if h.SessionID != "" {
			paneID = "session-" + h.SessionID
		} else {
			return nil
		}
	}

	safePaneID := sanitizePaneID(paneID)
	statusFile := filepath.Join(statusDir, "status-"+safePaneID+".json")
	eventsFile := filepath.Join(statusDir, "status-"+safePaneID+".events.jsonl")
	countersFile := filepath.Join(statusDir, "status-"+safePaneID+".counters")
	subagentFile := filepath.Join(statusDir, "status-"+safePaneID+".subagents.json")

	ts := time.Now().Format("15:04:05")

	// SessionEnd: clean up and exit early
	if h.HookEvent == "SessionEnd" {
		os.Remove(statusFile)
		os.Remove(eventsFile)
		os.Remove(countersFile)
		os.Remove(subagentFile)
		return nil
	}

	// Load counters
	c := loadCounters(countersFile)

	// Subagent count from cache
	saCount := c.SubagentCount

	var status string
	var eventJSON string

	switch h.HookEvent {
	case "SessionStart":
		status = "idle"
		c.SessionStartTS = time.Now().Unix()
		c.PromptCount = 0
		c.ToolCount = 0
		os.WriteFile(subagentFile, []byte("[]"), 0o644)
		truncateFile(eventsFile)
		eventJSON = buildEventJSON(ts, "session_start", nil)

	case "UserPromptSubmit":
		status = "working"
		c.PromptCount++
		prompt := truncStr(h.Prompt, 200)
		eventJSON = buildEventJSON(ts, "prompt", map[string]string{"text": prompt})

	case "PreToolUse":
		c.ToolCount++
		if h.ToolName == "AskQuestion" {
			status = "waiting"
		} else {
			status = "working"
		}
		inputSummary := extractToolInputSummary(h.ToolInput)
		// When an Agent tool is about to be spawned, cache its description so
		// the immediately-following SubagentStart event can use it (Claude Code
		// does not include description in the SubagentStart payload itself).
		if h.ToolName == "Agent" {
			if agentDesc := extractStringFieldFromObject(h.ToolInput, "description"); agentDesc != "" {
				pendingFile := filepath.Join(statusDir, "status-"+safePaneID+".pending-agent-desc")
				os.WriteFile(pendingFile, []byte(agentDesc), 0o644)
			}
		}
		// Persist the full TodoWrite input so the TUI can show native Claude Code tasks.
		if h.ToolName == "TodoWrite" && len(h.ToolInput) > 0 {
			todosFile := filepath.Join(statusDir, "status-"+safePaneID+".todos.json")
			os.WriteFile(todosFile, []byte(h.ToolInput), 0o644)
		}
		// Track superpowers TaskCreate/TaskUpdate so sessions show task progress.
		if h.ToolName == "TaskCreate" && len(h.ToolInput) > 0 {
			taskListAdd(filepath.Join(statusDir, "status-"+safePaneID+".tasklist.json"), h.ToolInput)
		}
		if h.ToolName == "TaskUpdate" && len(h.ToolInput) > 0 {
			taskListUpdate(filepath.Join(statusDir, "status-"+safePaneID+".tasklist.json"), h.ToolInput)
		}
		eventJSON = buildEventJSON(ts, "tool_start", map[string]string{
			"tool":  h.ToolName,
			"input": inputSummary,
		})

	case "PostToolUse":
		status = "working"
		outputSummary := extractStringField(h.ToolOutput, 300)
		eventJSON = buildEventJSON(ts, "tool_result", map[string]string{
			"tool":   h.ToolName,
			"output": outputSummary,
		})

	case "PostToolUseFailure":
		status = "working"
		errMsg := truncStr(h.Error, 200)
		eventJSON = buildEventJSON(ts, "tool_error", map[string]string{
			"tool":  h.ToolName,
			"error": errMsg,
		})

	case "SubagentStart":
		status = "working"
		// Claude Code does not include the agent description in the SubagentStart
		// payload. It was available in the preceding PreToolUse(Agent) event and
		// cached to a pending file. Consume it now, falling back to any top-level
		// or tool_input fields if the file isn't there.
		desc := h.Description
		if desc == "" {
			pendingFile := filepath.Join(statusDir, "status-"+safePaneID+".pending-agent-desc")
			if data, err := os.ReadFile(pendingFile); err == nil {
				desc = strings.TrimSpace(string(data))
				os.Remove(pendingFile)
			}
		}
		if desc == "" {
			desc = extractStringFieldFromObject(h.ToolInput, "description")
		}
		if desc == "" {
			desc = truncStr(extractStringFieldFromObject(h.ToolInput, "prompt"), 80)
		}
		saType := h.AgentType
		if saType == "" {
			saType = inferAgentType(desc)
		}
		saCount, _ = subagentAdd(subagentFile, SubagentEntry{
			ID:            h.AgentID,
			AgentType:     saType,
			Description:   desc,
			Model:         h.Model,
			StartedAt:     ts,
			Status:        "working",
			ParentAgentID: h.ParentAgentID,
			SandboxType:   DetectSandbox(),
		})
		eventJSON = buildEventJSON(ts, "subagent_start", map[string]string{
			"description": desc,
			"model":       h.Model,
			"tool":        saType,
		})

	case "SubagentStop":
		status = "working"
		if h.AgentID != "" {
			saCount, _ = subagentRemove(subagentFile, h.AgentID)
		}
		summary := truncStr(h.LastAssistantMsg, 200)
		eventJSON = buildEventJSON(ts, "subagent_stop", map[string]string{
			"tool":    h.AgentType,
			"summary": summary,
		})

	case "Stop":
		status = "idle"
		summary := truncStr(h.LastAssistantMsg, 200)
		eventJSON = buildEventJSON(ts, "stop", map[string]string{
			"reason":       h.StopReason,
			"last_message": summary,
		})

	case "Notification":
		status = "waiting"
		msg := truncStr(h.Message, 200)
		eventJSON = buildEventJSON(ts, "notification", map[string]string{"text": msg})

	case "PreCompact":
		status = "working"
		eventJSON = buildEventJSON(ts, "compact", nil)

	case "TaskCompleted":
		status = "idle"
		eventJSON = buildEventJSON(ts, "task_completed", nil)

	case "TeammateIdle":
		status = "idle"
		eventJSON = buildEventJSON(ts, "teammate_idle", nil)

	case "InstructionsLoaded":
		// Event log only, no status change
		eventJSON = buildEventJSON(ts, "rules_loaded", nil)

	default:
		return nil
	}

	// Tag with agent context
	if h.AgentID != "" && eventJSON != "" {
		eventJSON = appendAgentTag(eventJSON, h.AgentID, h.AgentType)
	}

	// Derive agent mode
	agentMode := "agent"
	if h.PermissionMode == "plan" {
		agentMode = "plan"
	}
	c.AgentMode = agentMode
	c.SubagentCount = saCount

	// Save counters
	saveCounters(countersFile, c)

	// Write status JSON
	if status != "" {
		writeStatusJSON(statusFile, paneID, h, status, c)
	}

	// Append event
	if eventJSON != "" {
		appendEvent(eventsFile, eventJSON)
	}

	// Forward to hookserver (fire-and-forget)
	go forwardToHookserver(statusDir, input)

	// Periodic cleanup on Stop
	if h.HookEvent == "Stop" {
		cleanupOldFiles(statusDir)
	}

	return nil
}

func sanitizePaneID(id string) string {
	var b strings.Builder
	for _, c := range id {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func loadCounters(path string) counters {
	c := counters{
		SessionStartTS: time.Now().Unix(),
		AgentMode:      "agent",
	}
	f, err := os.Open(path)
	if err != nil {
		return c
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "PROMPT_COUNT":
			c.PromptCount, _ = strconv.Atoi(parts[1])
		case "TOOL_COUNT":
			c.ToolCount, _ = strconv.Atoi(parts[1])
		case "SESSION_START_TS":
			c.SessionStartTS, _ = strconv.ParseInt(parts[1], 10, 64)
		case "AGENT_MODE":
			c.AgentMode = parts[1]
		case "SUBAGENT_COUNT":
			c.SubagentCount, _ = strconv.Atoi(parts[1])
		}
	}
	return c
}

func saveCounters(path string, c counters) {
	data := fmt.Sprintf("PROMPT_COUNT=%d\nTOOL_COUNT=%d\nSESSION_START_TS=%d\nAGENT_MODE=%s\nSUBAGENT_COUNT=%d\n",
		c.PromptCount, c.ToolCount, c.SessionStartTS, c.AgentMode, c.SubagentCount)
	os.WriteFile(path, []byte(data), 0o644)
}

func writeStatusJSON(path, paneID string, h HookInput, status string, c counters) {
	now := time.Now().Unix()
	data := fmt.Sprintf(`{"pane_id":"%s","session_id":"%s","status":"%s","event":"%s","cost":0,"model":"%s","cwd":"%s","permission_mode":"%s","agent_mode":"%s","last_tool":"%s","worktree_path":"%s","worktree_branch":"%s","original_repo":"%s","effort_level":"%s","prompt_count":%d,"tool_count":%d,"session_start_ts":%d,"subagent_count":%d,"sandbox_type":"%s","timestamp":%d}`,
		jsonEscape(paneID),
		jsonEscape(h.SessionID),
		status,
		h.HookEvent,
		jsonEscape(h.Model),
		jsonEscape(h.CWD),
		h.PermissionMode,
		c.AgentMode,
		jsonEscape(h.ToolName),
		jsonEscape(h.Worktree.Path),
		jsonEscape(h.Worktree.Branch),
		jsonEscape(h.Worktree.OriginalRepo),
		jsonEscape(h.Effort),
		c.PromptCount,
		c.ToolCount,
		c.SessionStartTS,
		c.SubagentCount,
		DetectSandbox(),
		now,
	)
	os.WriteFile(path, []byte(data), 0o644)
}

func appendEvent(path, eventJSON string) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.WriteString(eventJSON + "\n")
	f.Close()

	// Prune if >200 lines
	pruneEventsFile(path, 200)
}

func pruneEventsFile(path string, maxLines int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	f.Close()

	if len(lines) <= maxLines {
		return
	}

	lines = lines[len(lines)-maxLines:]
	os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func truncateFile(path string) {
	os.WriteFile(path, nil, 0o644)
}

func forwardToHookserver(statusDir string, input []byte) {
	portFile := filepath.Join(statusDir, ".hookserver-port")
	data, err := os.ReadFile(portFile)
	if err != nil {
		return
	}
	port := strings.TrimSpace(string(data))
	if port == "" {
		return
	}

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Post("http://127.0.0.1:"+port+"/hook", "application/json", strings.NewReader(string(input)))
	if err != nil {
		return
	}
	resp.Body.Close()
}

func cleanupOldFiles(statusDir string) {
	cutoff := time.Now().Add(-60 * time.Minute)
	for _, pattern := range []string{"status-*.json", "status-*.events.jsonl", "status-*.counters", "status-*.subagents.json"} {
		matches, _ := filepath.Glob(filepath.Join(statusDir, pattern))
		for _, m := range matches {
			info, err := os.Stat(m)
			if err == nil && info.ModTime().Before(cutoff) {
				os.Remove(m)
			}
		}
	}
}

func extractToolInputSummary(raw json.RawMessage) string {
	if raw == nil {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}

	// Priority order matches the bash script
	for _, key := range []string{"command", "path", "pattern", "query", "search_term", "glob_pattern", "url", "description", "prompt"} {
		if v, ok := m[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				limit := 80
				if key == "command" {
					limit = 120
				}
				return truncStr(s, limit)
			}
		}
	}

	// Fallback: first 2 keys
	var keys []string
	for k := range m {
		keys = append(keys, k)
		if len(keys) >= 2 {
			break
		}
	}
	return strings.Join(keys, ",")
}

// extractStringFieldFromObject extracts a named string key from a JSON object.
// Returns "" if the key is absent or not a string.
func extractStringFieldFromObject(raw json.RawMessage, key string) string {
	if raw == nil {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	json.Unmarshal(v, &s)
	return s
}

func extractStringField(raw json.RawMessage, maxLen int) string {
	if raw == nil {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return truncStr(s, maxLen)
	}
	// Not a string — stringify it
	return truncStr(string(raw), maxLen)
}

func truncStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func inferAgentType(desc string) string {
	lower := strings.ToLower(desc)
	switch {
	case strings.Contains(lower, "explor"):
		return "explore"
	case strings.Contains(lower, "shell") || strings.Contains(lower, "bash") || strings.Contains(lower, "command"):
		return "shell"
	case strings.Contains(lower, "brows"):
		return "browser"
	case strings.Contains(lower, "review"):
		return "code-reviewer"
	case strings.Contains(lower, "simplif"):
		return "code-simplifier"
	case strings.Contains(lower, "plan"):
		return "plan"
	case strings.Contains(lower, "debug"):
		return "debug"
	case strings.Contains(lower, "search") || strings.Contains(lower, "find"):
		return "explore"
	case strings.Contains(lower, "test"):
		return "test"
	default:
		return "general"
	}
}

// taskEntry is one item in the tasklist JSON file.
type taskEntry struct {
	ID     string `json:"id"`
	Status string `json:"status"` // "pending", "in_progress", "completed"
	Subject string `json:"subject"`
}

// taskListAdd appends a new task from a TaskCreate tool_input.
func taskListAdd(path string, raw json.RawMessage) {
	var input struct {
		Subject     string `json:"subject"`
		Description string `json:"description"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return
	}
	subject := input.Subject
	if subject == "" {
		subject = input.Description
	}
	if subject == "" {
		return
	}

	var list []taskEntry
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &list)
	}

	// Auto-assign sequential ID.
	id := fmt.Sprintf("%d", len(list)+1)
	list = append(list, taskEntry{ID: id, Status: "pending", Subject: subject})

	data, _ := json.Marshal(list)
	os.WriteFile(path, data, 0o644)
}

// taskListUpdate updates a task status from a TaskUpdate tool_input.
func taskListUpdate(path string, raw json.RawMessage) {
	var input struct {
		ID     string `json:"id"`
		TaskID string `json:"taskId"`
		Status string `json:"status"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return
	}
	id := input.ID
	if id == "" {
		id = input.TaskID
	}
	if id == "" || input.Status == "" {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var list []taskEntry
	if json.Unmarshal(data, &list) != nil {
		return
	}
	for i, t := range list {
		if t.ID == id {
			list[i].Status = input.Status
			break
		}
	}
	out, _ := json.Marshal(list)
	os.WriteFile(path, out, 0o644)
}
