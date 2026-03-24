package detect

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
)

// CursorHookData represents data from a Cursor hook file.
type CursorHookData struct {
	ConversationID string `json:"conversation_id"`
	Source         string `json:"source"`
	Status         string `json:"status"`
	Event          string `json:"event"`
	Model          string `json:"model"`
	Workspace      string `json:"workspace"`
	WorkspaceName  string `json:"workspace_name"`
	CursorVersion  string `json:"cursor_version"`
	SessionID      string `json:"session_id"`
	CWD            string `json:"cwd"`
	LastTool       string `json:"last_tool"`
	StopReason     string `json:"stop_reason"`
	PermissionMode string `json:"permission_mode"`
	AgentMode      string `json:"agent_mode"`
	PlanTitle      string `json:"plan_title"`
	TranscriptPath string `json:"transcript_path"`
	PromptCount    int    `json:"prompt_count"`
	ToolCount      int    `json:"tool_count"`
	SessionStartTS int64  `json:"session_start_ts"`
	SubagentCount  int    `json:"subagent_count"`
	Timestamp      int64  `json:"timestamp"`

	// Worktree fields (v2.1.69+)
	WorktreePath   string `json:"worktree_path,omitempty"`
	WorktreeBranch string `json:"worktree_branch,omitempty"`
	OriginalRepo   string `json:"original_repo,omitempty"`

	// Effort level (v2.1.62+): "low", "medium", "high"
	EffortLevel string `json:"effort_level,omitempty"`
}

// CursorStaleTimeout is how long a Cursor session can be inactive before
// it's considered closed. This handles crashes and force quits where the
// sessionEnd hook doesn't fire. Set to 5 minutes.
const CursorStaleTimeout = 20 * 60 // 20 minutes in seconds

// ReadCursorSessions scans ~/.claude-tmux for cursor-*.json files and returns
// ClaudeWindow entries for each active Cursor session.
// Sessions that haven't been updated in CursorStaleTimeout seconds are
// considered inactive and their files are removed.
func ReadCursorSessions() ([]core.ClaudeWindow, error) {
	dir := statusDir()
	if dir == "" {
		return nil, nil
	}

	pattern := filepath.Join(dir, "cursor-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	var sessions []core.ClaudeWindow
	now := time.Now().Unix()

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		var hd CursorHookData
		if err := json.Unmarshal(data, &hd); err != nil {
			continue
		}

		// Check if session is stale (no updates in CursorStaleTimeout seconds)
		// This handles cases where Cursor crashes or is force-quit without
		// the sessionEnd hook firing.
		if now-hd.Timestamp > CursorStaleTimeout {
			// Clean up stale file - session is likely closed
			os.Remove(file)
			continue
		}

		// Convert to ClaudeWindow
		win := cursorDataToWindow(hd)
		sessions = append(sessions, win)
	}

	return sessions, nil
}

// cursorDataToWindow converts CursorHookData to a ClaudeWindow.
func cursorDataToWindow(hd CursorHookData) core.ClaudeWindow {
	// Map status string to Status enum
	var status core.Status
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

	// Use cwd for working directory when available (more accurate than
	// workspace root since it reflects the agent's current directory).
	// Fall back to workspace root.
	workingDir := hd.CWD
	if workingDir == "" {
		workingDir = hd.Workspace
	}

	// Create a single pane for the Cursor session
	pane := core.ClaudePane{
		PaneID:     hd.ConversationID,
		Status:     status,
		WorkingDir: workingDir,
		Model:      hd.Model,
		// Git info will be populated by git detection
	}

	// Use workspace name as session name, fallback to conversation ID
	sessionName := hd.WorkspaceName
	if sessionName == "" {
		sessionName = hd.ConversationID
		if len(sessionName) > 8 {
			sessionName = sessionName[:8] // Truncate long UUIDs
		}
	}

	win := core.ClaudeWindow{
		SessionName:    sessionName,
		WindowIndex:    0,
		WindowName:     sessionName,
		Panes:          []core.ClaudePane{pane},
		Attached:       false,
		CreatedAt:      hd.Timestamp,
		Source:         core.SourceCursor,
		ConversationID: hd.ConversationID,
		WorkspacePath:  hd.Workspace,
		PermissionMode: hd.PermissionMode,
		AgentMode:      hd.AgentMode,
		CursorVersion:  hd.CursorVersion,
		PromptCount:    hd.PromptCount,
		ToolCount:      hd.ToolCount,
		SubagentCount:  hd.SubagentCount,
		SessionStartTS: hd.SessionStartTS,
	}

	if hd.PlanTitle != "" {
		win.ActivePlanTitle = hd.PlanTitle
	}

	return win
}

// ReadCLIEventsRaw returns raw CursorEvent structs for a CLI session (pane-based).
// Used by the UI layer to render events with full lipgloss styling.
func ReadCLIEventsRaw(paneID string, maxEvents int) []core.CursorEvent {
	dir := statusDir()
	if dir == "" {
		return nil
	}
	safePaneID := sanitizePaneID(paneID)
	return readJSONLEvents(filepath.Join(dir, "status-"+safePaneID+".events.jsonl"), maxEvents)
}

// ReadCursorEventsRaw returns raw CursorEvent structs for a Cursor session.
// Used by the UI layer to render events with full lipgloss styling.
func ReadCursorEventsRaw(conversationID string, maxEvents int) []core.CursorEvent {
	dir := statusDir()
	if dir == "" {
		return nil
	}
	return readJSONLEvents(filepath.Join(dir, "cursor-"+conversationID+".events.jsonl"), maxEvents)
}

// StripANSI removes ANSI escape sequences from a string.
// Exported so the UI layer can clean raw tmux pane captures before display.
func StripANSI(s string) string {
	return stripANSI(s)
}

// ReadCursorEventLog reads the JSONL event log and formats it for the preview pane.
// Falls back to the legacy plain-text log if JSONL is unavailable.
func ReadCursorEventLog(conversationID string, maxEvents int) string {
	dir := statusDir()
	if dir == "" {
		return ""
	}

	eventsFile := filepath.Join(dir, "cursor-"+conversationID+".events.jsonl")
	events := readJSONLEvents(eventsFile, maxEvents)
	if len(events) > 0 {
		return formatCursorEvents(events)
	}

	// Fall back to legacy plain-text log
	return ReadCursorActivityLog(conversationID, maxEvents)
}

// readJSONLEvents reads the last N events from a JSONL file.
func readJSONLEvents(path string, maxEvents int) []core.CursorEvent {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []core.CursorEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var evt core.CursorEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			continue
		}
		all = append(all, evt)
	}

	if len(all) > maxEvents {
		all = all[len(all)-maxEvents:]
	}
	return all
}

// formatCursorEvents renders parsed events into a human-readable preview.
func formatCursorEvents(events []core.CursorEvent) string {
	var lines []string
	for _, e := range events {
		line := formatSingleEvent(e)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func formatSingleEvent(e core.CursorEvent) string {
	ts := e.Timestamp
	switch e.Type {
	case "session_start":
		return "[" + ts + "] session started"
	case "prompt":
		text := truncate(e.Text, 100)
		if text == "" {
			return "[" + ts + "] You: (prompt)"
		}
		return "[" + ts + "] You: " + text
	case "tool_start":
		if e.Input != "" {
			return "[" + ts + "] ▸ " + e.Tool + ": " + truncate(e.Input, 80)
		}
		return "[" + ts + "] ▸ " + e.Tool
	case "tool_result":
		out := truncate(e.Output, 120)
		if out == "" {
			return ""
		}
		return "[" + ts + "]   → " + out
	case "response":
		text := truncate(e.Text, 120)
		if text == "" {
			return ""
		}
		return "[" + ts + "] Claude: " + text
	case "thought":
		return "[" + ts + "] 💭 thinking..."
	case "file_edit":
		name := e.Path
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		return "[" + ts + "] ✏ " + name + " (" + e.Summary + ")"
	case "shell_result":
		return "[" + ts + "] $ " + truncate(e.Command, 60) + " → exit " + e.ExitCode
	case "subagent_start":
		agentType := e.Tool
		if agentType == "" {
			agentType = "subagent"
		}
		desc := truncate(e.Description, 60)
		if e.Model != "" {
			return "[" + ts + "] ◆ " + agentType + " (" + e.Model + "): " + desc
		}
		return "[" + ts + "] ◆ " + agentType + ": " + desc
	case "subagent_stop":
		agentType := e.Tool
		if agentType == "" {
			agentType = "subagent"
		}
		if e.Summary != "" {
			return "[" + ts + "] ◆ " + agentType + " done: " + truncate(e.Summary, 80)
		}
		return "[" + ts + "] ◆ " + agentType + " completed"
	case "compact":
		return "[" + ts + "] ⟳ compacting context"
	case "stop":
		if e.Reason != "" {
			return "[" + ts + "] ■ stopped (" + e.Reason + ")"
		}
		return "[" + ts + "] ■ stopped"
	default:
		return "[" + ts + "] " + e.Type
	}
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

// ReadCursorActivityLog reads the last N lines from the legacy plain-text activity log.
func ReadCursorActivityLog(conversationID string, maxLines int) string {
	dir := statusDir()
	if dir == "" {
		return ""
	}

	logFile := filepath.Join(dir, "cursor-"+conversationID+".log")
	data, err := os.ReadFile(logFile)
	if err != nil {
		return ""
	}

	content := strings.TrimRight(string(data), "\n")
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")

	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}

	return strings.Join(lines, "\n")
}

// SwitchToCursor opens a Cursor window for the given workspace using a deeplink.
// Returns nil if the command was executed successfully.
func SwitchToCursor(workspacePath string) error {
	// Use 'open' command with cursor:// URL scheme
	// cursor://file/{path} opens the workspace in Cursor
	url := "cursor://file/" + workspacePath

	// Use os/exec to run the open command
	return runOpenURL(url)
}

// runOpenURL is a variable to allow testing - executes 'open' on macOS.
var runOpenURL = func(url string) error {
	// This will be set by the tmux package which imports os/exec
	// For now, return nil - the actual implementation is in tmux.go
	return nil
}

// IsCursorSession returns true if the window is from Cursor IDE.
func IsCursorSession(win core.ClaudeWindow) bool {
	return win.Source == core.SourceCursor
}

// GetCursorSessionCount returns the number of Cursor sessions in the list.
func GetCursorSessionCount(windows []core.ClaudeWindow) int {
	count := 0
	for _, w := range windows {
		if w.Source == core.SourceCursor {
			count++
		}
	}
	return count
}

// GetCLISessionCount returns the number of CLI (tmux) sessions in the list.
func GetCLISessionCount(windows []core.ClaudeWindow) int {
	count := 0
	for _, w := range windows {
		if w.Source == core.SourceCLI {
			count++
		}
	}
	return count
}

// FilterBySource returns windows matching the given source filter.
func FilterBySource(windows []core.ClaudeWindow, filter core.SourceFilter) []core.ClaudeWindow {
	if filter == core.FilterAll {
		return windows
	}

	var filtered []core.ClaudeWindow
	for _, w := range windows {
		if filter.Matches(w.Source) {
			filtered = append(filtered, w)
		}
	}
	return filtered
}

// EnrichCursorWithGit adds git information to Cursor sessions.
func EnrichCursorWithGit(win *core.ClaudeWindow) {
	if win.Source != core.SourceCursor || len(win.Panes) == 0 {
		return
	}

	// The git package will handle enrichment - this is a placeholder
	// for where we'd call git.DetectInfo for the workspace path
	workDir := win.WorkspacePath
	if workDir == "" && len(win.Panes) > 0 {
		workDir = win.Panes[0].WorkingDir
	}

	// Git detection happens in tmux.go when loading sessions
	_ = workDir
}

// SourceBadge returns the display badge for a session source.
func SourceBadge(source core.SessionSource) string {
	switch source {
	case core.SourceCLI:
		return "[CLI]"
	case core.SourceCursor:
		return "[CURSOR]"
	default:
		return ""
	}
}

// ShortenConversationID truncates a UUID for display.
func ShortenConversationID(id string) string {
	if len(id) <= 8 {
		return id
	}
	// Return first 8 chars
	return id[:8]
}

// CleanupStaleCursorSessions removes Cursor session files older than maxAge.
func CleanupStaleCursorSessions(maxAge time.Duration) error {
	dir := statusDir()
	if dir == "" {
		return nil
	}

	pattern := filepath.Join(dir, "cursor-*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	now := time.Now()
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if now.Sub(info.ModTime()) > maxAge {
			os.Remove(file)
		}
	}

	return nil
}

// RemoveCursorSession removes the session file and activity log for a given conversation ID.
func RemoveCursorSession(conversationID string) error {
	dir := statusDir()
	if dir == "" {
		return nil
	}

	// Sanitize the conversation ID for filename
	safeID := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '_'
	}, conversationID)

	os.Remove(filepath.Join(dir, "cursor-"+safeID+".json"))
	os.Remove(filepath.Join(dir, "cursor-"+safeID+".log"))
	os.Remove(filepath.Join(dir, "cursor-"+safeID+".events.jsonl"))
	os.Remove(filepath.Join(dir, "cursor-"+safeID+".counters"))
	os.Remove(filepath.Join(dir, "cursor-"+safeID+".subagents.json"))
	return nil
}
