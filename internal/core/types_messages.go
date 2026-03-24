package core

import "time"

// TickMsg is sent periodically by the spinner/refresh timer.
type TickMsg time.Time

// PreviewDebounceMsg is sent after the preview debounce delay.
type PreviewDebounceMsg struct {
	PaneID string
}

// FilterDebounceMsg is sent after the filter debounce delay.
type FilterDebounceMsg struct {
	FilterText string
}

// WindowsMsg carries the result of an async window list refresh.
type WindowsMsg struct {
	Windows         []ClaudeWindow
	AttachedSession string // current tmux session name
	Err             error
}

// PreviewMsg carries async preview content for the selected pane.
type PreviewMsg struct {
	Content string
	PaneID  string // which pane this preview is for (to avoid stale updates)
}

// GitResultMsg carries the result of an async git operation.
type GitResultMsg struct {
	Success bool
	Message string
}

// PlansMsg carries the result of an async plan scan.
type PlansMsg struct {
	Plans     []PlanEntry
	Err       error
	FromCache bool // true when served from disk cache; a background refresh may follow
}

// TitleGeneratedMsg carries the result of an async plan title generation.
type TitleGeneratedMsg struct {
	ConvID   string
	NewTitle string
	Err      error
}

// SyncProgressMsg reports progress during a DuckDB sync operation.
type SyncProgressMsg struct {
	Phase   string // "scanning", "syncing", "indexing"
	Current int
	Total   int
	Detail  string
	Done    bool
	Err     error
	// Set on completion
	NewPlans     int
	UpdatedPlans int
	ProjectCount int
	TotalPlans   int
	EventCount   int
	ActivityDays int
}

// ActivityDataMsg carries loaded activity data for the activity view.
type ActivityDataMsg struct {
	Grid        []ActivityDay
	Projects    []ActivityProject
	ActivePlans []PlanEntry // incomplete plans for the "Active Plans" strip
	Err         error
}

// ActivityDay is a single day's score for the heatmap grid.
type ActivityDay struct {
	Date  time.Time
	Score int
}

// ActivityProject is a project summary for the activity view.
type ActivityProject struct {
	WorkspacePath  string
	Name           string
	TotalPlans     int
	CompletedTodos int
	TotalTodos     int
	TotalScore     int
}

// RestructurePlanMsg is sent when a background `claude -p` restructure completes.
type RestructurePlanMsg struct {
	FilePath string
	Err      error
}

// ConvertConversationMsg is sent when a JSONL-to-plan conversion completes.
type ConvertConversationMsg struct {
	OriginalPath string // the .jsonl file that was converted
	NewPath      string // the generated .plan.md file
	Title        string // extracted plan title
	Err          error
}

// HookEventMsg is sent by the HTTP hook server when a hook event is received
// from Claude Code or Cursor. It enables real-time status updates without
// waiting for the 5s polling cycle.
type HookEventMsg struct {
	// Session identity: one of these will be set
	PaneID         string // for CLI sessions (TMUX_PANE value)
	ConversationID string // for Cursor sessions
	SessionID      string // Claude Code session_id (fallback when PaneID unknown)

	Source string // "cli" or "cursor"
	Event  string // hook_event_name (e.g. "UserPromptSubmit", "Stop")
	Status string // derived status: "idle", "working", "waiting"

	// Session metadata (update in-place when present)
	Model          string
	CWD            string
	AgentMode      string
	PermissionMode string
	PromptCount    int
	ToolCount      int
	SubagentCount  int

	// Worktree info (from hook payload v2.1.69+)
	WorktreePath   string
	WorktreeBranch string
	OriginalRepo   string

	// Effort level (from Claude Code v2.1.62+)
	EffortLevel string
}
