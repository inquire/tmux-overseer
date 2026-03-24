package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
)

// Pre-compiled regexes.
var (
	versionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	costRegex    = regexp.MustCompile(`Cost:\s*\$([0-9]+\.?[0-9]*)`)
	modelRegex   = regexp.MustCompile(`Model:\s*([^|]+)`)
	// Matches model name in Claude startup screen (e.g., "Opus 4.6 · API Usage")
	modelStartupRegex = regexp.MustCompile(`(Opus|Sonnet|Haiku)\s+[\d.]+`)
	// Matches ANSI escape sequences
	ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b\[[0-9;]*m`)
	// Matches dollar amounts (e.g., "$1.24") for cost fallback parsing
	dollarRegex = regexp.MustCompile(`\$([0-9]+\.[0-9]{2,4})`)
)

// HookData represents data from a Claude Code hook file.
type HookData struct {
	PaneID         string  `json:"pane_id"`
	SessionID      string  `json:"session_id"`
	Status         string  `json:"status"`
	Event          string  `json:"event"`
	Cost           float64 `json:"cost"`
	Model          string  `json:"model"`
	CWD            string  `json:"cwd"`
	Timestamp      int64   `json:"timestamp"`
	PermissionMode string  `json:"permission_mode"`
	AgentMode      string  `json:"agent_mode"`
	PromptCount    int     `json:"prompt_count"`
	ToolCount      int     `json:"tool_count"`
	SessionStartTS int64   `json:"session_start_ts"`
	SubagentCount  int     `json:"subagent_count"`
	LastTool       string  `json:"last_tool"`

	// Worktree fields (Claude Code v2.1.69+)
	WorktreePath   string `json:"worktree_path,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	OriginalRepo   string `json:"original_repo,omitempty"`

	// Effort level (Claude Code v2.1.62+): "low", "medium", "high"
	EffortLevel string `json:"effort_level,omitempty"`

	// Sandbox/container type: "docker", "kubernetes", or "" for bare metal
	SandboxType string `json:"sandbox_type,omitempty"`
}

// Cached status directory path (computed once).
var (
	statusDirPath     string
	statusDirPathOnce sync.Once
)

// statusDir returns the directory where hook status files are stored.
func statusDir() string {
	statusDirPathOnce.Do(func() {
		home := state.CachedHomeDir()
		if home != "" {
			statusDirPath = filepath.Join(home, ".claude-tmux")
		}
	})
	return statusDirPath
}

// sanitizePaneID converts a pane ID to a safe filename component.
func sanitizePaneID(paneID string) string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			return r
		}
		return '_'
	}, paneID)
}

// ReadHookData reads full hook data from a hook-generated file for a given pane.
// Returns nil if no recent data is available.
func ReadHookData(paneID string) *HookData {
	dir := statusDir()
	if dir == "" {
		return nil
	}

	safePaneID := sanitizePaneID(paneID)
	statusFile := filepath.Join(dir, "status-"+safePaneID+".json")
	data, err := os.ReadFile(statusFile)
	if err != nil {
		return nil
	}

	var hd HookData
	if err := json.Unmarshal(data, &hd); err != nil {
		return nil
	}

	// Check if data is recent (within last 60 seconds for cost/model, they don't change as fast)
	if time.Now().Unix()-hd.Timestamp > 60 {
		return nil
	}

	return &hd
}

// ReadHookStatus reads status from a hook-generated file for a given pane.
// Returns StatusUnknown if no recent status is available.
func ReadHookStatus(paneID string) core.Status {
	hd := ReadHookData(paneID)
	if hd == nil {
		return core.StatusUnknown
	}

	// For status, use a tighter time window (30 seconds)
	if time.Now().Unix()-hd.Timestamp > 30 {
		return core.StatusUnknown
	}

	switch hd.Status {
	case "idle":
		return core.StatusIdle
	case "working":
		return core.StatusWorking
	case "waiting":
		return core.StatusWaitingInput
	default:
		return core.StatusUnknown
	}
}

// ReadHookCost reads cost from a hook-generated file for a given pane.
// Returns 0 if no data is available.
func ReadHookCost(paneID string) float64 {
	hd := ReadHookData(paneID)
	if hd == nil {
		return 0
	}
	return hd.Cost
}

// ReadHookModel reads model name from a hook-generated file for a given pane.
// Returns empty string if no data is available.
func ReadHookModel(paneID string) string {
	hd := ReadHookData(paneID)
	if hd == nil {
		return ""
	}
	return hd.Model
}

// EnrichWithHook extracts status, cost, and model from a single hook data read.
// This consolidates three separate ReadHookData calls into one for better performance.
// Falls back to terminal parsing if hook data is unavailable or stale.
func EnrichWithHook(paneID, content string) (status core.Status, cost float64, model, lastTool string) {
	hd := ReadHookData(paneID)

	// Status check (30 second window for real-time accuracy)
	if hd != nil && time.Now().Unix()-hd.Timestamp <= 30 {
		switch hd.Status {
		case "idle":
			status = core.StatusIdle
		case "working":
			status = core.StatusWorking
		case "waiting":
			status = core.StatusWaitingInput
		default:
			status = core.StatusUnknown
		}
	} else {
		status = Status(content)
	}

	// Cost, model and last tool (from hook data)
	if hd != nil {
		cost = hd.Cost
		model = hd.Model
		lastTool = hd.LastTool
	}

	// Fall back to terminal parsing if hook data unavailable
	if cost == 0 {
		cost = ParseCost(content)
	}
	if model == "" {
		model = ParseModel(content)
	}

	return
}

// ReadSubagents reads active subagents from a JSON array file.
// Used by both Cursor and CLI detection paths.
func ReadSubagents(filePath string) []core.Subagent {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var agents []core.Subagent
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil
	}
	return agents
}

// ReadCLISubagents reads subagents for a CLI session by pane ID.
func ReadCLISubagents(paneID string) []core.Subagent {
	dir := statusDir()
	if dir == "" {
		return nil
	}
	safePaneID := sanitizePaneID(paneID)
	return ReadSubagents(filepath.Join(dir, "status-"+safePaneID+".subagents.json"))
}

// ReadCursorSubagents reads subagents for a Cursor session by conversation ID.
func ReadCursorSubagents(conversationID string) []core.Subagent {
	dir := statusDir()
	if dir == "" {
		return nil
	}
	return ReadSubagents(filepath.Join(dir, "cursor-"+conversationID+".subagents.json"))
}

// claudeTodo mirrors the JSON structure of a Claude Code TodoWrite todo item.
type claudeTodo struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"` // "pending", "in_progress", "completed"
	Priority string `json:"priority"`
}

// ReadCLITodos reads the native Claude Code todo list for a CLI session.
// Returns nil when no todos file exists or TodoWrite has never been called.
func ReadCLITodos(paneID string) []core.PlanTodo {
	dir := statusDir()
	if dir == "" {
		return nil
	}
	safePaneID := sanitizePaneID(paneID)
	data, err := os.ReadFile(filepath.Join(dir, "status-"+safePaneID+".todos.json"))
	if err != nil {
		return nil
	}

	// TodoWrite input is { "todos": [...] }
	var wrapper struct {
		Todos []claudeTodo `json:"todos"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		// Try as a bare array too.
		var items []claudeTodo
		if err2 := json.Unmarshal(data, &items); err2 != nil {
			return nil
		}
		wrapper.Todos = items
	}

	todos := make([]core.PlanTodo, len(wrapper.Todos))
	for i, t := range wrapper.Todos {
		status := t.Status
		// Normalise Claude Code status values to PlanTodo values.
		switch status {
		case "in_progress":
			status = "in_progress"
		case "completed":
			status = "completed"
		default:
			status = "pending"
		}
		todos[i] = core.PlanTodo{Content: t.Content, Status: status}
	}
	return todos
}

// ReadCLITaskList reads the superpowers TaskCreate/TaskUpdate task list for a CLI session.
// Returns nil when no tasklist file exists.
func ReadCLITaskList(paneID string) []core.PlanTodo {
	dir := statusDir()
	if dir == "" {
		return nil
	}
	safePaneID := sanitizePaneID(paneID)
	data, err := os.ReadFile(filepath.Join(dir, "status-"+safePaneID+".tasklist.json"))
	if err != nil {
		return nil
	}
	var items []struct {
		ID      string `json:"id"`
		Status  string `json:"status"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		return nil
	}
	todos := make([]core.PlanTodo, len(items))
	for i, t := range items {
		status := t.Status
		if status == "" {
			status = "pending"
		}
		todos[i] = core.PlanTodo{Content: t.Subject, Status: status}
	}
	return todos
}

// ReadCLIEventLog reads the JSONL event log for a CLI session and formats it for preview.
func ReadCLIEventLog(paneID string, maxEvents int) string {
	dir := statusDir()
	if dir == "" {
		return ""
	}
	safePaneID := sanitizePaneID(paneID)
	eventsFile := filepath.Join(dir, "status-"+safePaneID+".events.jsonl")
	events := readJSONLEvents(eventsFile, maxEvents)
	if len(events) > 0 {
		return formatCursorEvents(events)
	}
	return ""
}

// IsClaudeCommand checks if a tmux pane_current_command indicates Claude Code.
func IsClaudeCommand(cmd string) bool {
	if cmd == "claude" || strings.Contains(cmd, "claude") {
		return true
	}
	return versionRegex.MatchString(cmd)
}

// stripANSI removes ANSI escape sequences from text.
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// Status analyzes captured pane content to determine Claude's state.
// This is the fallback when hook-based detection is not available.
func Status(content string) core.Status {
	// Strip ANSI escape codes first for reliable text matching
	content = stripANSI(content)

	lines := strings.Split(content, "\n")

	// Get last 15 non-empty lines - focus on recent output
	var lastLines []string
	for i := len(lines) - 1; i >= 0 && len(lastLines) < 15; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			lastLines = append([]string{line}, lastLines...)
		}
	}

	text := strings.Join(lastLines, "\n")

	// Check for waiting input patterns FIRST (highest priority)
	// These indicate Claude is waiting for user selection/confirmation
	if strings.Contains(text, "Enter to select") ||
		strings.Contains(text, "↑/↓ to navigate") ||
		strings.Contains(text, "Esc to cancel") ||
		strings.Contains(text, "[y/n]") ||
		strings.Contains(text, "[Y/n]") ||
		strings.Contains(text, "Yes / No") ||
		strings.Contains(text, "(y/n)") ||
		strings.Contains(text, "Do you want to") {
		return core.StatusWaitingInput
	}

	// Check for working patterns SECOND
	// Be explicit about working indicators - Claude shows these when actively working
	workingPatterns := []string{
		// Interrupt indicators
		"ctrl+c to interrupt",
		"Ctrl+C to interrupt",
		"Ctrl-C to interrupt",
		// Activity indicators
		"Accessing workspace:",
		"Reading file",
		"Writing to",
		"Searching",
		"Analyzing",
		"Processing",
		"Generating",
		"Executing",
		"Running",
		// Thinking phrases
		"Thinking",
		"Let me",
		"I'll",
		"I will",
		"Looking at",
		"Checking",
		"Examining",
		// Tool use indicators
		"Using tool",
		"Calling",
		"Fetching",
		// Progress indicators
		"Working on",
		"In progress",
		"Loading",
		// Plan mode indicators
		"plan mode",
		"Plan Mode",
	}
	
	for _, pattern := range workingPatterns {
		if strings.Contains(text, pattern) {
			return core.StatusWorking
		}
	}
	
	// Check for spinner characters (braille patterns used by CLI spinners)
	spinnerChars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", "⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	for _, char := range spinnerChars {
		if strings.Contains(text, char) {
			return core.StatusWorking
		}
	}

	// Check for idle THIRD - Claude prompt at the end
	// The ❯ prompt should be on one of the last few lines when idle
	// It might have placeholder text after it like: ❯ Try "something"
	if len(lastLines) > 0 {
		// Check last 5 lines for the prompt
		checkCount := 5
		if len(lastLines) < checkCount {
			checkCount = len(lastLines)
		}
		for i := len(lastLines) - 1; i >= len(lastLines)-checkCount && i >= 0; i-- {
			line := strings.TrimSpace(lastLines[i])
			// Prompt starts with ❯ (idle state)
			if strings.HasPrefix(line, "❯") {
				return core.StatusIdle
			}
		}
	}

	// Check for status bar as fallback indicator of idle state
	// If we see "Model:" and "Cost:" in the output, Claude is likely idle
	if strings.Contains(text, "Model:") && strings.Contains(text, "Cost:") {
		// But make sure we're not in the middle of working
		// Look for the prompt somewhere in recent lines
		for _, line := range lastLines {
			if strings.HasPrefix(strings.TrimSpace(line), "❯") {
				return core.StatusIdle
			}
		}
	}

	return core.StatusUnknown
}

// StatusWithHook tries hook-based detection first, then falls back to terminal parsing.
func StatusWithHook(paneID, content string) core.Status {
	// Try hook-based status first (most reliable)
	if status := ReadHookStatus(paneID); status != core.StatusUnknown {
		return status
	}

	// Fall back to terminal parsing
	return Status(content)
}

// ParseCost extracts the cost value from pane content (e.g. "Cost: $1.24").
// This is the fallback when hook-based detection is not available.
func ParseCost(content string) float64 {
	// Strip ANSI for reliable parsing
	content = stripANSI(content)

	// Try "Cost: $X.XX" format
	matches := costRegex.FindStringSubmatch(content)
	if len(matches) >= 2 {
		cost, err := strconv.ParseFloat(matches[1], 64)
		if err == nil {
			return cost
		}
	}

	// Try finding any "$X.XX" pattern in the content (less reliable)
	dollarMatches := dollarRegex.FindAllStringSubmatch(content, -1)
	if len(dollarMatches) > 0 {
		// Take the last match (most recent cost display)
		lastMatch := dollarMatches[len(dollarMatches)-1]
		if len(lastMatch) >= 2 {
			cost, err := strconv.ParseFloat(lastMatch[1], 64)
			if err == nil {
				return cost
			}
		}
	}

	return 0
}

// SandboxTypeFromCommand infers the sandbox type from the tmux pane_current_command.
// Returns "local" for direct Claude invocations, "docker" for known wrapper commands.
func SandboxTypeFromCommand(cmd string) string {
	switch {
	case cmd == "miro-claude":
		return "docker"
	case cmd == "claude" || versionRegex.MatchString(cmd):
		return "local"
	default:
		return "local" // unknown wrappers treated as local
	}
}

// ParseSandboxType detects sandbox type from pane content when hook data is unavailable.
// Claude Code renders sandbox indicators using emoji in its status bar (e.g., "🐳 Docker").
func ParseSandboxType(content string) string {
	content = stripANSI(content)
	if strings.Contains(content, "🐳") {
		return "docker"
	}
	if strings.Contains(content, "☸") || strings.Contains(content, "⎈") {
		return "kubernetes"
	}
	return ""
}

// ParseModel extracts the model name from pane content (e.g. "Model: Opus 4.6").
// This is the fallback when hook-based detection is not available.
func ParseModel(content string) string {
	// Strip ANSI for reliable parsing
	content = stripANSI(content)

	// Try "Model: ..." format first
	matches := modelRegex.FindStringSubmatch(content)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1])
	}

	// Try startup screen format (e.g., "Opus 4.6 · API Usage")
	startupMatches := modelStartupRegex.FindStringSubmatch(content)
	if len(startupMatches) >= 1 {
		return startupMatches[0]
	}

	return ""
}

// CostWithHook tries hook-based cost first, then falls back to terminal parsing.
func CostWithHook(paneID, content string) float64 {
	// Try hook-based cost first (most reliable)
	if cost := ReadHookCost(paneID); cost > 0 {
		return cost
	}
	// Fall back to terminal parsing
	return ParseCost(content)
}

// ModelWithHook tries hook-based model first, then falls back to terminal parsing.
func ModelWithHook(paneID, content string) string {
	// Try hook-based model first (most reliable)
	if model := ReadHookModel(paneID); model != "" {
		return model
	}
	// Fall back to terminal parsing
	return ParseModel(content)
}
