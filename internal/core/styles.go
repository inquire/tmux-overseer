package core

import (
	"image/color"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// Color palette
var (
	ColorOrange = lipgloss.Color("#D77757") // Claude brand color
	ColorCyan   = lipgloss.Color("#56B6C2")
	ColorGreen  = lipgloss.Color("#4EC994")
	ColorYellow = lipgloss.Color("#E5C07B")
	ColorRed    = lipgloss.Color("#E06C75")
	ColorGray   = lipgloss.Color("#636363")
	ColorWhite  = lipgloss.Color("#FFFFFF")
	ColorDim    = lipgloss.Color("#555555")
	ColorPurple = lipgloss.Color("#A855F7") // Cursor brand color (violet/purple)
)

// Clawd mascot rendered in Unicode block characters (small variant).
const ClawdSmall = " ▐▛███▜▌\n▝▜█████▛▘\n  ▘▘ ▝▝"

// ClaudeFlowerSpinner is the Claude Code "flower" spinner.
var ClaudeFlowerSpinner = spinner.Spinner{
	Frames: []string{"·", "✻", "✽", "✻"},
	FPS:    time.Second / 4,
}

// --- Styles ---

var (
	// Header
	TitleStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	MascotStyle = lipgloss.NewStyle().
			Foreground(ColorOrange)

	AttachedStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	// Session list rows
	SelectedRowStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Bold(true)

	NormalRowStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCCCCC"))

	DimRowStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	PaneCountBadge = lipgloss.NewStyle().
			Foreground(ColorCyan)

	// Git indicators
	GitBranchStyle = lipgloss.NewStyle().
			Foreground(ColorCyan)

	GitDirtyStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	GitStagedStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	// Cost
	CostStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	// Preview
	PreviewSeparator = lipgloss.NewStyle().
				Foreground(ColorGray)

	PreviewContentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AAAAAA"))

	// Status bar
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	StatusCountStyle = lipgloss.NewStyle().
				Foreground(ColorCyan)

	// Footer / help
	FooterStyle = lipgloss.NewStyle().
			Foreground(ColorDim)

	FooterKeyStyle = lipgloss.NewStyle().
			Foreground(ColorCyan)

	// Action menu
	ActionSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Bold(true)

	ActionNormalStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AAAAAA"))

	ActionSeparatorStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	ActionLabelStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	ActionValueStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	// Dialogs
	DialogBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorCyan).
				Padding(1, 2)

	DialogTitleStyle = lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true)

	// Messages / overlays
	SuccessStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	// Empty state
	EmptyMessageStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	// Source badges
	ClaudeBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorOrange).
				Bold(true)

	CursorBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorPurple).
				Bold(true)

	CloudBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorCyan).
			Bold(true)

	// Session metadata
	PermissionModeStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	AgentModeBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorPurple)

	PlanModeBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorYellow)

	SessionStatsStyle = lipgloss.NewStyle().
				Foreground(ColorDim)

	SubagentCountStyle = lipgloss.NewStyle().
				Foreground(ColorPurple)

	// Filter indicator
	FilterActiveStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	// Model name
	ModelNameStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	// Section headers
	SectionHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				Bold(true)

	SectionSeparatorStyle = lipgloss.NewStyle().
				Foreground(ColorDim)

	// Tab bar (cyberpunk terminal style)
	TabActiveStyle = lipgloss.NewStyle().
			Background(ColorRed).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(ColorDim).
				Padding(0, 1)

	// Multi-selection marker (plans bulk operations)
	MarkedRowStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	// Plans view
	PlanTitleStyle = lipgloss.NewStyle().
		Foreground(ColorWhite).
		Bold(true)

	PlanOverviewStyle = lipgloss.NewStyle().
				Foreground(ColorDim)

	PlanDateStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	PlanProgressStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	PlanBarFilledStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	PlanBarEmptyStyle = lipgloss.NewStyle().
				Foreground(ColorDim)

	PlanHeaderStyle = lipgloss.NewStyle().
		Foreground(ColorCyan).
		Bold(true)

	// Workspace group headers (collapsible session/plan groups)
	GroupHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true)

	GroupHeaderDimStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	// Scroll indicators (used in session list and action menu)
	ScrollIndicatorStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	// Activity heatmap
	HeatLevel0 = lipgloss.NewStyle().Foreground(lipgloss.Color("#2d2d2d"))
	HeatLevel1 = lipgloss.NewStyle().Foreground(lipgloss.Color("#0e4429"))
	HeatLevel2 = lipgloss.NewStyle().Foreground(lipgloss.Color("#006d32"))
	HeatLevel3 = lipgloss.NewStyle().Foreground(lipgloss.Color("#26a641"))
	HeatLevel4 = lipgloss.NewStyle().Foreground(lipgloss.Color("#39d353"))

	HeatSelectedStyle = lipgloss.NewStyle().
				Foreground(ColorWhite).
				Bold(true)

	ActivityHeaderStyle = lipgloss.NewStyle().
				Foreground(ColorCyan).
				Bold(true)

	ActivityProjectStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#CCCCCC"))

	ActivityBarFilled = lipgloss.NewStyle().
			Foreground(ColorGreen)

	ActivityBarEmpty = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#2d2d2d"))

	// Filter match highlight (bold orange — used in plan search results)
	FilterMatchStyle = lipgloss.NewStyle().
			Foreground(ColorOrange).
			Bold(true)

	// Task list styles (orange — CLI native tasks via TaskCreate/TodoWrite)
	TaskSectionStyle = lipgloss.NewStyle().Foreground(ColorOrange)
	TaskActiveStyle  = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
	TaskPendingStyle = lipgloss.NewStyle().Foreground(ColorOrange)

	// Activity styles (cyan — current tool use and subagents)
	ActivitySectionStyle = lipgloss.NewStyle().Foreground(ColorCyan)
	ActivityItemStyle    = lipgloss.NewStyle().Foreground(ColorCyan)

	// Effort level display (○ ◐ ●) — muted so it doesn't compete with status dots
	EffortLevelStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	// Worktree badge (⎇ branch) — shown when a session runs in a --worktree
	WorktreeBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorCyan)

	// Automation badge ([AUTO]) — for Cursor Automation-triggered cloud agents
	AutomationBadgeStyle = lipgloss.NewStyle().
				Foreground(ColorPurple).
				Bold(true)

	// Team role badges ([LEAD] / [TEAM])
	LeadBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true)
	TeamBadgeStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	// Attached session star (★) in the session list
	AttachedStarStyle = lipgloss.NewStyle().
			Foreground(ColorYellow).
			Bold(true)

	// Progress bar colors (context-aware: green < 50%, yellow 50-70%, red > 70%)
	ProgressBarGreenStyle = lipgloss.NewStyle().Foreground(ColorGreen)
	ProgressBarYellowStyle = lipgloss.NewStyle().Foreground(ColorYellow)
	ProgressBarRedStyle = lipgloss.NewStyle().Foreground(ColorRed)
	ProgressBarEmptyStyle = lipgloss.NewStyle().Foreground(ColorDim)

	// Tag pill palette — initialized in init() because lipgloss.Color is a function, not a type.
	TagPalette []color.Color
)

func init() {
	TagPalette = []color.Color{
		lipgloss.Color("#61AFEF"), // blue
		lipgloss.Color("#C678DD"), // purple
		lipgloss.Color("#E06C75"), // red
		lipgloss.Color("#98C379"), // green
		lipgloss.Color("#E5C07B"), // yellow
		lipgloss.Color("#56B6C2"), // cyan
		lipgloss.Color("#D19A66"), // orange
		lipgloss.Color("#BE5046"), // dark red
	}
}

// TagPillStyle returns a styled pill for a tag name, with consistent color assignment.
func TagPillStyle(tagName string) lipgloss.Style {
	h := uint32(0)
	for _, c := range tagName {
		h = h*31 + uint32(c)
	}
	c := TagPalette[h%uint32(len(TagPalette))]
	return lipgloss.NewStyle().Foreground(c)
}

// Pre-cached status styles (avoids allocating a new Style per call).
var statusStyles = map[Status]lipgloss.Style{
	StatusUnknown:      lipgloss.NewStyle().Foreground(ColorGray),
	StatusIdle:         lipgloss.NewStyle().Foreground(ColorGreen),
	StatusWorking:      lipgloss.NewStyle().Foreground(ColorOrange),
	StatusWaitingInput: lipgloss.NewStyle().Foreground(ColorYellow),
}

// StatusStyle returns a pre-cached styled status symbol.
func StatusStyle(s Status) lipgloss.Style {
	if style, ok := statusStyles[s]; ok {
		return style
	}
	return statusStyles[StatusUnknown]
}

// RenderMascot returns the Clawd mascot colored in orange.
func RenderMascot() string {
	return MascotStyle.Render(ClawdSmall)
}
