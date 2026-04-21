package core

import (
	"image/color"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// Clawd mascot rendered in Unicode block characters (small variant).
const ClawdSmall = " ▐▛███▜▌\n▝▜█████▛▘\n  ▘▘ ▝▝"

// ClaudeFlowerSpinner is the Claude Code "flower" spinner.
var ClaudeFlowerSpinner = spinner.Spinner{
	Frames: []string{"·", "✻", "✽", "✻"},
	FPS:    time.Second / 4,
}

// Styles holds all lipgloss styles, resolved for the current terminal background.
type Styles struct {
	// Adaptive colour palette
	ColorOrange color.Color
	ColorCyan   color.Color
	ColorGreen  color.Color
	ColorYellow color.Color
	ColorRed    color.Color
	ColorGray   color.Color
	ColorWhite  color.Color
	ColorDim    color.Color
	ColorPurple color.Color

	// Header
	TitleStyle    lipgloss.Style
	MascotStyle   lipgloss.Style
	AttachedStyle lipgloss.Style

	// Session list rows
	SelectedRowStyle lipgloss.Style
	NormalRowStyle   lipgloss.Style
	DimRowStyle      lipgloss.Style
	PaneCountBadge   lipgloss.Style

	// Git indicators
	GitBranchStyle lipgloss.Style
	GitDirtyStyle  lipgloss.Style
	GitStagedStyle lipgloss.Style

	// Cost
	CostStyle lipgloss.Style

	// Preview
	PreviewSeparator    lipgloss.Style
	PreviewContentStyle lipgloss.Style

	// Status bar
	StatusBarStyle   lipgloss.Style
	StatusCountStyle lipgloss.Style

	// Footer / help
	FooterStyle    lipgloss.Style
	FooterKeyStyle lipgloss.Style

	// Action menu
	ActionSelectedStyle  lipgloss.Style
	ActionNormalStyle     lipgloss.Style
	ActionSeparatorStyle lipgloss.Style
	ActionLabelStyle     lipgloss.Style
	ActionValueStyle     lipgloss.Style

	// Dialogs
	DialogBorderStyle lipgloss.Style
	DialogTitleStyle  lipgloss.Style

	// Messages / overlays
	SuccessStyle lipgloss.Style
	ErrorStyle   lipgloss.Style

	// Empty state
	EmptyMessageStyle lipgloss.Style

	// Source badges
	ClaudeBadgeStyle lipgloss.Style
	CursorBadgeStyle lipgloss.Style
	CloudBadgeStyle  lipgloss.Style

	// Session metadata
	PermissionModeStyle lipgloss.Style
	AgentModeBadgeStyle lipgloss.Style
	PlanModeBadgeStyle  lipgloss.Style
	SessionStatsStyle   lipgloss.Style
	SubagentCountStyle  lipgloss.Style

	// Filter indicator
	FilterActiveStyle lipgloss.Style

	// Model name
	ModelNameStyle lipgloss.Style

	// Section headers
	SectionHeaderStyle    lipgloss.Style
	SectionSeparatorStyle lipgloss.Style

	// Tab bar
	TabActiveStyle   lipgloss.Style
	TabInactiveStyle lipgloss.Style

	// Multi-selection marker
	MarkedRowStyle lipgloss.Style

	// Plans view
	PlanTitleStyle    lipgloss.Style
	PlanOverviewStyle lipgloss.Style
	PlanDateStyle     lipgloss.Style
	PlanProgressStyle lipgloss.Style
	PlanBarFilledStyle lipgloss.Style
	PlanBarEmptyStyle  lipgloss.Style
	PlanHeaderStyle    lipgloss.Style

	// Workspace group headers
	GroupHeaderStyle    lipgloss.Style
	GroupHeaderDimStyle lipgloss.Style

	// Scroll indicators
	ScrollIndicatorStyle lipgloss.Style

	// Activity heatmap
	HeatLevel0        lipgloss.Style
	HeatLevel1        lipgloss.Style
	HeatLevel2        lipgloss.Style
	HeatLevel3        lipgloss.Style
	HeatLevel4        lipgloss.Style
	HeatSelectedStyle lipgloss.Style

	ActivityHeaderStyle  lipgloss.Style
	ActivityProjectStyle lipgloss.Style
	ActivityBarFilled    lipgloss.Style
	ActivityBarEmpty     lipgloss.Style

	// Filter match highlight
	FilterMatchStyle lipgloss.Style

	// Task list styles
	TaskSectionStyle lipgloss.Style
	TaskActiveStyle  lipgloss.Style
	TaskPendingStyle lipgloss.Style

	// Activity styles
	ActivitySectionStyle lipgloss.Style
	ActivityItemStyle    lipgloss.Style

	// Effort level
	EffortLevelStyle lipgloss.Style

	// Worktree badge
	WorktreeBadgeStyle lipgloss.Style

	// Automation badge
	AutomationBadgeStyle lipgloss.Style

	// Team role badges
	LeadBadgeStyle lipgloss.Style
	TeamBadgeStyle lipgloss.Style

	// Attached session star
	AttachedStarStyle lipgloss.Style

	// Progress bar colors
	ProgressBarGreenStyle  lipgloss.Style
	ProgressBarYellowStyle lipgloss.Style
	ProgressBarRedStyle    lipgloss.Style
	ProgressBarEmptyStyle  lipgloss.Style

	// Tag pill palette
	TagPalette []color.Color

	// Pre-cached status styles
	statusStyles map[Status]lipgloss.Style
}

// NewStyles creates a complete Styles set resolved for the given background.
func NewStyles(hasDark bool) Styles {
	ld := lipgloss.LightDark(hasDark)

	// Adaptive colour palette — light values target ≥4.5:1 on white per WCAG AA.
	colorOrange := ld(lipgloss.Color("#9C3E1C"), lipgloss.Color("#D77757"))
	colorCyan := ld(lipgloss.Color("#0B5467"), lipgloss.Color("#56B6C2"))
	colorGreen := ld(lipgloss.Color("#166534"), lipgloss.Color("#4EC994"))
	colorYellow := ld(lipgloss.Color("#854D0E"), lipgloss.Color("#E5C07B"))
	colorRed := ld(lipgloss.Color("#991B1B"), lipgloss.Color("#E06C75"))
	colorGray := ld(lipgloss.Color("#4B5563"), lipgloss.Color("#636363"))
	colorWhite := ld(lipgloss.Color("#1F2937"), lipgloss.Color("#FFFFFF"))
	colorDim := ld(lipgloss.Color("#6B7280"), lipgloss.Color("#555555"))
	colorPurple := ld(lipgloss.Color("#6D28D9"), lipgloss.Color("#A855F7"))

	colorNormalRow := ld(lipgloss.Color("#374151"), lipgloss.Color("#CCCCCC"))
	colorPreviewContent := ld(lipgloss.Color("#374151"), lipgloss.Color("#AAAAAA"))
	colorTabActiveFg := ld(lipgloss.Color("#FFFFFF"), lipgloss.Color("#000000"))
	colorHeat0 := ld(lipgloss.Color("#D1D5DB"), lipgloss.Color("#2d2d2d"))
	colorHeat1 := ld(lipgloss.Color("#6EE7B7"), lipgloss.Color("#0e4429"))
	colorHeat2 := ld(lipgloss.Color("#34D399"), lipgloss.Color("#006d32"))
	colorHeat3 := ld(lipgloss.Color("#10B981"), lipgloss.Color("#26a641"))
	colorHeat4 := ld(lipgloss.Color("#059669"), lipgloss.Color("#39d353"))
	colorActivityProject := ld(lipgloss.Color("#1F2937"), lipgloss.Color("#CCCCCC"))

	s := Styles{
		ColorOrange: colorOrange,
		ColorCyan:   colorCyan,
		ColorGreen:  colorGreen,
		ColorYellow: colorYellow,
		ColorRed:    colorRed,
		ColorGray:   colorGray,
		ColorWhite:  colorWhite,
		ColorDim:    colorDim,
		ColorPurple: colorPurple,

		// Header
		TitleStyle:    lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		MascotStyle:   lipgloss.NewStyle().Foreground(colorOrange),
		AttachedStyle: lipgloss.NewStyle().Foreground(colorGray),

		// Session list rows
		SelectedRowStyle: lipgloss.NewStyle().Foreground(colorWhite).Bold(true),
		NormalRowStyle:   lipgloss.NewStyle().Foreground(colorNormalRow),
		DimRowStyle:      lipgloss.NewStyle().Foreground(colorDim),
		PaneCountBadge:   lipgloss.NewStyle().Foreground(colorCyan),

		// Git indicators
		GitBranchStyle: lipgloss.NewStyle().Foreground(colorCyan),
		GitDirtyStyle:  lipgloss.NewStyle().Foreground(colorYellow),
		GitStagedStyle: lipgloss.NewStyle().Foreground(colorGreen),

		// Cost
		CostStyle: lipgloss.NewStyle().Foreground(colorDim),

		// Preview
		PreviewSeparator:    lipgloss.NewStyle().Foreground(colorGray),
		PreviewContentStyle: lipgloss.NewStyle().Foreground(colorPreviewContent),

		// Status bar
		StatusBarStyle:   lipgloss.NewStyle().Foreground(colorGray),
		StatusCountStyle: lipgloss.NewStyle().Foreground(colorCyan),

		// Footer / help
		FooterStyle:    lipgloss.NewStyle().Foreground(colorDim),
		FooterKeyStyle: lipgloss.NewStyle().Foreground(colorCyan),

		// Action menu
		ActionSelectedStyle:  lipgloss.NewStyle().Foreground(colorWhite).Bold(true),
		ActionNormalStyle:     lipgloss.NewStyle().Foreground(colorNormalRow),
		ActionSeparatorStyle: lipgloss.NewStyle().Foreground(colorGray),
		ActionLabelStyle:     lipgloss.NewStyle().Foreground(colorGray),
		ActionValueStyle:     lipgloss.NewStyle().Foreground(colorWhite),

		// Dialogs
		DialogBorderStyle: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorCyan).Padding(1, 2),
		DialogTitleStyle:  lipgloss.NewStyle().Foreground(colorCyan).Bold(true),

		// Messages / overlays
		SuccessStyle: lipgloss.NewStyle().Foreground(colorGreen),
		ErrorStyle:   lipgloss.NewStyle().Foreground(colorRed),

		// Empty state
		EmptyMessageStyle: lipgloss.NewStyle().Foreground(colorGray),

		// Source badges
		ClaudeBadgeStyle: lipgloss.NewStyle().Foreground(colorOrange).Bold(true),
		CursorBadgeStyle: lipgloss.NewStyle().Foreground(colorPurple).Bold(true),
		CloudBadgeStyle:  lipgloss.NewStyle().Foreground(colorCyan).Bold(true),

		// Session metadata
		PermissionModeStyle: lipgloss.NewStyle().Foreground(colorGray),
		AgentModeBadgeStyle: lipgloss.NewStyle().Foreground(colorPurple),
		PlanModeBadgeStyle:  lipgloss.NewStyle().Foreground(colorYellow),
		SessionStatsStyle:   lipgloss.NewStyle().Foreground(colorDim),
		SubagentCountStyle:  lipgloss.NewStyle().Foreground(colorPurple),

		// Filter indicator
		FilterActiveStyle: lipgloss.NewStyle().Foreground(colorYellow),

		// Model name
		ModelNameStyle: lipgloss.NewStyle().Foreground(colorRed),

		// Section headers
		SectionHeaderStyle:    lipgloss.NewStyle().Foreground(colorGray).Bold(true),
		SectionSeparatorStyle: lipgloss.NewStyle().Foreground(colorDim),

		// Tab bar
		TabActiveStyle:   lipgloss.NewStyle().Background(colorRed).Foreground(colorTabActiveFg).Bold(true).Padding(0, 1),
		TabInactiveStyle: lipgloss.NewStyle().Foreground(colorDim).Padding(0, 1),

		// Multi-selection marker
		MarkedRowStyle: lipgloss.NewStyle().Foreground(colorYellow),

		// Plans view
		PlanTitleStyle:     lipgloss.NewStyle().Foreground(colorWhite).Bold(true),
		PlanOverviewStyle:  lipgloss.NewStyle().Foreground(colorDim),
		PlanDateStyle:      lipgloss.NewStyle().Foreground(colorGray),
		PlanProgressStyle:  lipgloss.NewStyle().Foreground(colorGreen),
		PlanBarFilledStyle: lipgloss.NewStyle().Foreground(colorGreen),
		PlanBarEmptyStyle:  lipgloss.NewStyle().Foreground(colorDim),
		PlanHeaderStyle:    lipgloss.NewStyle().Foreground(colorCyan).Bold(true),

		// Workspace group headers
		GroupHeaderStyle:    lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		GroupHeaderDimStyle: lipgloss.NewStyle().Foreground(colorGray),

		// Scroll indicators
		ScrollIndicatorStyle: lipgloss.NewStyle().Foreground(colorGray),

		// Activity heatmap
		HeatLevel0:        lipgloss.NewStyle().Foreground(colorHeat0),
		HeatLevel1:        lipgloss.NewStyle().Foreground(colorHeat1),
		HeatLevel2:        lipgloss.NewStyle().Foreground(colorHeat2),
		HeatLevel3:        lipgloss.NewStyle().Foreground(colorHeat3),
		HeatLevel4:        lipgloss.NewStyle().Foreground(colorHeat4),
		HeatSelectedStyle: lipgloss.NewStyle().Foreground(colorWhite).Bold(true),

		ActivityHeaderStyle:  lipgloss.NewStyle().Foreground(colorCyan).Bold(true),
		ActivityProjectStyle: lipgloss.NewStyle().Foreground(colorActivityProject),
		ActivityBarFilled:    lipgloss.NewStyle().Foreground(colorGreen),
		ActivityBarEmpty:     lipgloss.NewStyle().Foreground(colorHeat0),

		// Filter match highlight
		FilterMatchStyle: lipgloss.NewStyle().Foreground(colorOrange).Bold(true),

		// Task list styles
		TaskSectionStyle: lipgloss.NewStyle().Foreground(colorOrange),
		TaskActiveStyle:  lipgloss.NewStyle().Foreground(colorOrange).Bold(true),
		TaskPendingStyle: lipgloss.NewStyle().Foreground(colorOrange),

		// Activity styles
		ActivitySectionStyle: lipgloss.NewStyle().Foreground(colorCyan),
		ActivityItemStyle:    lipgloss.NewStyle().Foreground(colorCyan),

		// Effort level
		EffortLevelStyle: lipgloss.NewStyle().Foreground(colorGray),

		// Worktree badge
		WorktreeBadgeStyle: lipgloss.NewStyle().Foreground(colorCyan),

		// Automation badge
		AutomationBadgeStyle: lipgloss.NewStyle().Foreground(colorPurple).Bold(true),

		// Team role badges
		LeadBadgeStyle: lipgloss.NewStyle().Foreground(colorYellow).Bold(true),
		TeamBadgeStyle: lipgloss.NewStyle().Foreground(colorGray),

		// Attached session star
		AttachedStarStyle: lipgloss.NewStyle().Foreground(colorYellow).Bold(true),

		// Progress bar colors
		ProgressBarGreenStyle:  lipgloss.NewStyle().Foreground(colorGreen),
		ProgressBarYellowStyle: lipgloss.NewStyle().Foreground(colorYellow),
		ProgressBarRedStyle:    lipgloss.NewStyle().Foreground(colorRed),
		ProgressBarEmptyStyle:  lipgloss.NewStyle().Foreground(colorDim),

		// Tag pill palette
		TagPalette: []color.Color{
			ld(lipgloss.Color("#1D4ED8"), lipgloss.Color("#61AFEF")),
			ld(lipgloss.Color("#6D28D9"), lipgloss.Color("#C678DD")),
			ld(lipgloss.Color("#B91C1C"), lipgloss.Color("#E06C75")),
			ld(lipgloss.Color("#15803D"), lipgloss.Color("#98C379")),
			ld(lipgloss.Color("#854D0E"), lipgloss.Color("#E5C07B")),
			ld(lipgloss.Color("#0E7490"), lipgloss.Color("#56B6C2")),
			ld(lipgloss.Color("#9A3412"), lipgloss.Color("#D19A66")),
			ld(lipgloss.Color("#7F1D1D"), lipgloss.Color("#BE5046")),
		},
	}

	s.statusStyles = map[Status]lipgloss.Style{
		StatusUnknown:      lipgloss.NewStyle().Foreground(colorGray),
		StatusIdle:         lipgloss.NewStyle().Foreground(colorGreen),
		StatusWorking:      lipgloss.NewStyle().Foreground(colorOrange),
		StatusWaitingInput: lipgloss.NewStyle().Foreground(colorYellow),
	}

	return s
}

// StatusStyle returns a pre-cached styled status symbol.
func (s *Styles) StatusStyle(st Status) lipgloss.Style {
	if style, ok := s.statusStyles[st]; ok {
		return style
	}
	return s.statusStyles[StatusUnknown]
}

// TagPillStyle returns a styled pill for a tag name, with consistent color assignment.
func (s *Styles) TagPillStyle(tagName string) lipgloss.Style {
	h := uint32(0)
	for _, c := range tagName {
		h = h*31 + uint32(c)
	}
	c := s.TagPalette[h%uint32(len(s.TagPalette))]
	return lipgloss.NewStyle().Foreground(c)
}

// RenderMascot returns the Clawd mascot colored in orange.
func (s *Styles) RenderMascot() string {
	return s.MascotStyle.Render(ClawdSmall)
}
