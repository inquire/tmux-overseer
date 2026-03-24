package core

// ViewMode controls which view is currently displayed.
type ViewMode int

const (
	ModeSessionList ViewMode = iota
	ModeActionMenu
	ModeFilter
	ModeNewSession
	ModeRename
	ModeCommit
	ModeNewWorktree
	ModeConfirm
	ModeSendInput
	ModePlans
	ModePlanFilter
	ModeActivity
)

// SortMode controls how sessions are sorted.
type SortMode int

const (
	SortByName SortMode = iota
	SortByStatus
	SortByRecency
)

// GroupMode controls how sessions are grouped in the list.
type GroupMode int

const (
	GroupBySource    GroupMode = iota // CURSOR section + CLAUDE CODE section (default)
	GroupByWorkspace                  // all sessions grouped by workspace, source shown as badge
)

func (g GroupMode) Label() string {
	switch g {
	case GroupByWorkspace:
		return "workspace"
	default:
		return "source"
	}
}

func (s SortMode) Label() string {
	switch s {
	case SortByName:
		return "name"
	case SortByStatus:
		return "status"
	case SortByRecency:
		return "recent"
	default:
		return "name"
	}
}

// Next cycles to the next sort mode.
func (s SortMode) Next() SortMode {
	return (s + 1) % 3
}

// PlanGroupMode controls how plans are grouped in the plans view.
type PlanGroupMode int

const (
	PlanGroupByWorkspace PlanGroupMode = iota
	PlanGroupByDay
)

func (m PlanGroupMode) Label() string {
	switch m {
	case PlanGroupByWorkspace:
		return "workspace"
	case PlanGroupByDay:
		return "day"
	default:
		return "workspace"
	}
}

func (m PlanGroupMode) Next() PlanGroupMode {
	return (m + 1) % 2
}

// SessionAction represents an action available in the action menu.
type SessionAction int

const (
	ActionSwitchTo SessionAction = iota
	ActionSendInput
	ActionRename
	ActionStageAll
	ActionCommit
	ActionPush
	ActionFetch
	ActionNewWorktree
	ActionKillSession
	ActionKillAndDeleteWorktree
	ActionOpenInTerminal // Open a new tmux session in the Cursor workspace
	ActionCopyPath       // Copy the workspace path to clipboard
	ActionEndSession     // Remove the Cursor session file
)

// Label returns the display text for the action.
func (a SessionAction) Label() string {
	switch a {
	case ActionSwitchTo:
		return "Switch to session"
	case ActionSendInput:
		return "Send input"
	case ActionRename:
		return "Rename session"
	case ActionStageAll:
		return "Stage all changes"
	case ActionCommit:
		return "Commit"
	case ActionPush:
		return "Push"
	case ActionFetch:
		return "Fetch"
	case ActionNewWorktree:
		return "New worktree"
	case ActionKillSession:
		return "Kill session"
	case ActionKillAndDeleteWorktree:
		return "Kill & delete worktree"
	case ActionOpenInTerminal:
		return "Open in terminal"
	case ActionCopyPath:
		return "Copy path"
	case ActionEndSession:
		return "End session"
	default:
		return ""
	}
}

// IsSeparatorBefore returns true if a visual separator should be shown before this action.
func (a SessionAction) IsSeparatorBefore() bool {
	return a == ActionStageAll || a == ActionNewWorktree || a == ActionOpenInTerminal
}

// ListItem represents a selectable row in the session list.
// It can be a window header, a specific pane, a non-selectable section header,
// or a collapsible workspace group header.
type ListItem struct {
	WindowIdx       int    // index into model.windows (-1 for section/group headers)
	PaneIdx         int    // -1 for window-level row, 0+ for a specific pane
	IsSectionHeader bool   // true for non-selectable source-label rows
	SectionLabel    string // display text for section headers
	IsSpacer        bool   // true for blank breathing-room rows (non-selectable)
	// Workspace group fields
	IsGroupHeader      bool   // true for collapsible workspace group row (selectable)
	GroupKey           string // workspace path key for expand/collapse state
	GroupWindowIndices []int  // indices into model.windows belonging to this group
	InGroup            bool   // true for session rows that are children of a group
	// Agent Team fields
	IsTeamHeader      bool   // true for collapsible Agent Team row (selectable)
	TeamKey           string // team name key for expand/collapse state
	TeamWindowIndices []int  // indices into model.windows belonging to this team
	InTeam            bool   // true for session rows that are children of a team
}

// IsPane returns true if this item represents a specific pane.
func (li ListItem) IsPane() bool {
	return li.PaneIdx >= 0
}
