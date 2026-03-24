// Package core provides the core types and styles for claude-tmux.
package core

import (
	"strings"
	"time"
)

// SessionSource identifies where a Claude session is running.
type SessionSource int

const (
	SourceCLI        SessionSource = iota // Claude Code CLI in tmux
	SourceCursor                          // Cursor IDE
	SourceCloud                           // Cursor Cloud Agent (manual)
	SourceAutomation                      // Cursor Automation-triggered cloud agent
)

func (s SessionSource) String() string {
	switch s {
	case SourceCLI:
		return "CLAUDE"
	case SourceCursor:
		return "CURSOR"
	case SourceCloud:
		return "CLOUD"
	case SourceAutomation:
		return "AUTO"
	default:
		return "UNKNOWN"
	}
}

// Label returns a lowercase label for status bar display.
func (s SessionSource) Label() string {
	switch s {
	case SourceCLI:
		return "claude"
	case SourceCursor:
		return "cursor"
	case SourceCloud:
		return "cloud"
	case SourceAutomation:
		return "auto"
	default:
		return "unknown"
	}
}

// SourceFilter controls which session sources are displayed.
type SourceFilter int

const (
	FilterAll        SourceFilter = iota // Show all sessions
	FilterCLI                            // Show only CLI sessions
	FilterCursor                         // Show only Cursor sessions
	FilterCloud                          // Show only Cloud agent sessions
	FilterAutomation                     // Show only Automation-triggered sessions
)

func (f SourceFilter) Label() string {
	switch f {
	case FilterAll:
		return "all"
	case FilterCLI:
		return "cli"
	case FilterCursor:
		return "cursor"
	case FilterCloud:
		return "cloud"
	case FilterAutomation:
		return "auto"
	default:
		return "all"
	}
}

// Next cycles to the next filter mode.
func (f SourceFilter) Next() SourceFilter {
	return (f + 1) % 5
}

// Matches returns true if the given source matches this filter.
func (f SourceFilter) Matches(source SessionSource) bool {
	switch f {
	case FilterAll:
		return true
	case FilterCLI:
		return source == SourceCLI
	case FilterCursor:
		return source == SourceCursor
	case FilterCloud:
		return source == SourceCloud || source == SourceAutomation
	case FilterAutomation:
		return source == SourceAutomation
	default:
		return true
	}
}

// Status represents the detected state of a Claude instance.
type Status int

const (
	StatusUnknown Status = iota
	StatusIdle
	StatusWorking
	StatusWaitingInput
)

// Symbol returns the static Unicode indicator for this status.
func (s Status) Symbol() string {
	switch s {
	case StatusIdle:
		return "○"
	case StatusWorking:
		return "●"
	case StatusWaitingInput:
		return "◐"
	default:
		return "?"
	}
}

func (s Status) Label() string {
	switch s {
	case StatusIdle:
		return "idle"
	case StatusWorking:
		return "working"
	case StatusWaitingInput:
		return "waiting"
	default:
		return "unknown"
	}
}

// ClaudePane represents a single Claude instance running in a tmux pane.
type ClaudePane struct {
	PaneID      string
	Status      Status
	WorkingDir  string
	GitBranch   string
	GitDirty    bool   // has unstaged changes
	GitStaged   bool   // has staged changes
	IsWorktree  bool
	HasGit      bool
	Cost        float64 // session cost parsed from pane output
	Model       string  // model name parsed from pane output
	LastTool    string  // most recent tool call (from hook data)
	SandboxType string  // "local", "docker", "apple", "kubernetes", or ""
}

// ClaudeWindow represents a tmux window or Cursor session containing one or more Claude panes.
type ClaudeWindow struct {
	SessionName     string
	WindowIndex     int
	WindowName      string
	Panes           []ClaudePane
	Attached        bool
	CreatedAt       int64         // session_created timestamp
	Source          SessionSource // where this session is running (CLI, Cursor, or Cloud)
	ConversationID  string        // Cursor conversation ID (empty for CLI sessions)
	WorkspacePath   string        // Full workspace path (primarily for Cursor sessions)
	ActivePlanTitle string     // Plan title if session is executing a plan
	ActivePlanDone  int        // Completed todo count for active plan
	ActivePlanTotal int        // Total todo count for active plan
	ActivePlanTodos []PlanTodo // Individual todo items for expansion (collapsible)
	TaskTodos       []PlanTodo // Native task list (TaskCreate/TodoWrite) — always shown expanded

	// Cursor session metadata (populated from hook data)
	PermissionMode string // "auto", "yolo", "normal", etc.
	AgentMode      string // "agent" or "plan" (detected from system_reminder)
	CursorVersion  string
	PromptCount    int   // number of user prompts sent
	ToolCount      int   // number of tool calls made
	SubagentCount  int        // currently active async subagents
	Subagents      []Subagent // individual active subagents (expandable in UI)
	SessionStartTS int64      // when the session started (for duration calc)

	// Worktree metadata (populated from hook data for --worktree sessions)
	WorktreePath   string // path to the worktree directory
	WorktreeBranch string // branch checked out in the worktree
	OriginalRepo   string // path to the original (non-worktree) repo
	EffortLevel    string // effort level: "low", "medium", "high" (from Claude Code v2.1.62+)
	SandboxType    string // "docker", "kubernetes", or "" when running on bare metal

	// Agent Teams metadata (Claude Code v2.1.32+ experimental)
	TeamName string // name of the team this session belongs to (empty if not in a team)
	TeamRole string // "lead" or "teammate" (empty if not in a team)

	// Cloud agent metadata
	CloudAgentURL    string // URL to view the cloud agent
	CloudPRURL       string // associated pull request URL
	CloudSummary     string // agent summary text
	AutomationTrigger string // what triggered the automation (e.g. "github", "slack", "schedule")

	searchText string // precomputed lowercase search text
}

// DisplayName returns a clean human-readable name for the session:
//   - Cursor/Cloud: just the workspace name
//   - CLI: "session:window", but if the session name is "session" (tmux default)
//     or matches the window name, show just the window name
func (w ClaudeWindow) DisplayName() string {
	if w.Source == SourceCursor || w.Source == SourceCloud || w.Source == SourceAutomation {
		return w.SessionName
	}
	// Skip redundant or default session prefix.
	if w.SessionName == w.WindowName || w.SessionName == "session" || w.SessionName == "" {
		return w.WindowName
	}
	return w.SessionName + ":" + w.WindowName
}

// PrimaryPane returns the first pane (used for display when not expanded).
func (w ClaudeWindow) PrimaryPane() *ClaudePane {
	if len(w.Panes) == 0 {
		return nil
	}
	return &w.Panes[0]
}

// TotalCost sums cost across all panes in this window.
func (w ClaudeWindow) TotalCost() float64 {
	var total float64
	for _, p := range w.Panes {
		total += p.Cost
	}
	return total
}

// AggregateStatus returns the "worst" status across all panes.
// Priority: Working > WaitingInput > Idle > Unknown
func (w ClaudeWindow) AggregateStatus() Status {
	if len(w.Panes) == 0 {
		return StatusUnknown
	}
	if len(w.Panes) == 1 {
		return w.Panes[0].Status
	}

	hasWorking := false
	hasWaiting := false
	hasIdle := false

	for _, p := range w.Panes {
		switch p.Status {
		case StatusWorking:
			hasWorking = true
		case StatusWaitingInput:
			hasWaiting = true
		case StatusIdle:
			hasIdle = true
		}
	}

	if hasWorking {
		return StatusWorking
	}
	if hasWaiting {
		return StatusWaitingInput
	}
	if hasIdle {
		return StatusIdle
	}
	return StatusUnknown
}

// HasActivePlan returns true if this window is executing an incomplete plan.
func (w ClaudeWindow) HasActivePlan() bool {
	if w.ActivePlanTitle != "" && w.ActivePlanDone < w.ActivePlanTotal {
		return true
	}
	for _, t := range w.TaskTodos {
		if t.Status != "completed" && t.Status != "cancelled" {
			return true
		}
	}
	return false
}

// PlanCompletionPct returns 0-100 plan completion percentage (0 if no plan).
func (w ClaudeWindow) PlanCompletionPct() int {
	if w.ActivePlanTotal == 0 {
		return 0
	}
	return (w.ActivePlanDone * 100) / w.ActivePlanTotal
}

// ComputeSearchText precomputes lowercase searchable text for filtering.
func (w *ClaudeWindow) ComputeSearchText() {
	var parts []string
	parts = append(parts, strings.ToLower(w.SessionName))
	parts = append(parts, strings.ToLower(w.WindowName))
	if w.ActivePlanTitle != "" {
		parts = append(parts, strings.ToLower(w.ActivePlanTitle))
	}
	for _, p := range w.Panes {
		parts = append(parts, strings.ToLower(p.WorkingDir))
		parts = append(parts, strings.ToLower(p.GitBranch))
		parts = append(parts, strings.ToLower(p.Status.Label()))
	}
	w.searchText = strings.Join(parts, " ")
}

// SearchText returns the precomputed search text.
func (w *ClaudeWindow) SearchText() string {
	return w.searchText
}

// SessionDuration returns a human-readable duration since session start.
func (w ClaudeWindow) SessionDuration() string {
	if w.SessionStartTS <= 0 {
		return ""
	}
	d := time.Since(time.Unix(w.SessionStartTS, 0))
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return Itoa(int(d.Minutes())) + "m"
	default:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return Itoa(h) + "h"
		}
		return Itoa(h) + "h" + Itoa(m) + "m"
	}
}

// CursorEvent represents a single event from the Cursor JSONL activity log.
type CursorEvent struct {
	Timestamp   string `json:"ts"`
	Type        string `json:"type"` // prompt, tool_start, tool_result, response, thought, file_edit, shell_result, subagent_start, subagent_stop, compact, stop, session_start
	Text        string `json:"text,omitempty"`
	Tool        string `json:"tool,omitempty"`
	Input       string `json:"input,omitempty"`
	Output      string `json:"output,omitempty"`
	Path        string `json:"path,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Command     string `json:"command,omitempty"`
	ExitCode    string `json:"exit_code,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Description string `json:"description,omitempty"`
	Model       string `json:"model,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	AgentType   string `json:"agent_type,omitempty"`
}

// Subagent represents an active subagent spawned by a parent session.
type Subagent struct {
	ID               string `json:"id"`
	AgentType        string `json:"agent_type"`
	Description      string `json:"description"`
	Status           string `json:"status"`
	Model            string `json:"model,omitempty"`
	StartedAt        string `json:"started_at"`
	ParentAgentID    string `json:"parent_agent_id,omitempty"` // empty for top-level subagents
	CurrentTool      string `json:"current_tool,omitempty"`      // tool currently being executed
	CurrentToolInput string `json:"current_tool_input,omitempty"` // truncated tool input summary
	LastActivityAt   string `json:"last_activity_at,omitempty"`   // time of last tool event
	SandboxType      string `json:"sandbox_type,omitempty"`       // "docker", "kubernetes", or "" if bare
}

// Team represents a Claude Code Agent Team — a named group of Claude sessions
// where one acts as team lead and others are teammates (v2.1.32+ experimental).
type Team struct {
	Name    string
	Lead    *ClaudeWindow
	Members []*ClaudeWindow
}

// PlanTodo represents a single TODO item in a plan.
type PlanTodo struct {
	Content string
	Status  string // "pending", "in_progress", "completed", "cancelled"
}

// PlanEntry represents a plan from Cursor or a conversation from Claude Code.
type PlanEntry struct {
	Source        SessionSource // SourceCLI or SourceCursor
	Title         string        // Plan name or first user prompt
	Overview      string        // Summary/overview text
	Todos         []PlanTodo    // TODO items with status
	Tags          []string      // Category tags (e.g., "refactor", "auth")
	WorkspacePath string        // Resolved workspace path
	FilePath      string        // Path to the plan file or JSONL
	LastActive    time.Time     // File modification time
	ConvID        string        // Conversation/session ID
}

// NextTodo returns the content of the first pending or in_progress todo.
func (p PlanEntry) NextTodo() string {
	for _, t := range p.Todos {
		if t.Status == "in_progress" {
			return t.Content
		}
	}
	for _, t := range p.Todos {
		if t.Status == "pending" {
			return t.Content
		}
	}
	return ""
}

// SortedTodos returns todos sorted by status: completed, in_progress, pending, cancelled.
func (p PlanEntry) SortedTodos() []PlanTodo {
	statusOrder := map[string]int{"completed": 0, "in_progress": 1, "pending": 2, "cancelled": 3}
	sorted := make([]PlanTodo, len(p.Todos))
	copy(sorted, p.Todos)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && statusOrder[sorted[j].Status] < statusOrder[sorted[j-1].Status]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	return sorted
}

// CompletionPct returns the percentage of completed todos (0-100).
func (p PlanEntry) CompletionPct() int {
	if len(p.Todos) == 0 {
		return 0
	}
	return (p.CompletedCount() * 100) / len(p.Todos)
}

// IsCompleted returns true if ALL todos are completed or cancelled.
// Plans with no todos are never considered completed.
func (p PlanEntry) IsCompleted() bool {
	if len(p.Todos) == 0 {
		return false
	}
	for _, t := range p.Todos {
		if t.Status != "completed" && t.Status != "cancelled" {
			return false
		}
	}
	return true
}

// CompletedCount returns the number of completed todos.
func (p PlanEntry) CompletedCount() int {
	n := 0
	for _, t := range p.Todos {
		if t.Status == "completed" {
			n++
		}
	}
	return n
}

// ProgressBar returns a progress bar string like "🟩🟩⬜⬜ 2/4".
// Returns empty string if no todos.
func (p PlanEntry) ProgressBar() string {
	total := len(p.Todos)
	if total == 0 {
		return ""
	}
	done := p.CompletedCount()

	maxBlocks := 10
	if total < maxBlocks {
		maxBlocks = total
	}
	filled := 0
	if total > 0 {
		filled = (done * maxBlocks) / total
	}

	bar := strings.Repeat("🟩", filled) + strings.Repeat("⬜", maxBlocks-filled)
	return bar + " " + strings.Join([]string{Itoa(done), Itoa(total)}, "/")
}

// Itoa is a simple int-to-string helper to avoid importing strconv.
func Itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// PlanGroup groups plans that share the same workspace path.
type PlanGroup struct {
	WorkspacePath string
	Label         string // display label; if empty, uses shortened WorkspacePath
	Plans         []PlanEntry
}
