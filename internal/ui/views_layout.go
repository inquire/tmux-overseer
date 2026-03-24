package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/inquire/tmux-overseer/internal/core"
)

// renderMainLayout renders the full 5-section vertical layout.
func renderMainLayout(m Model) string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var b strings.Builder
	b.Grow(4096)

	b.WriteString(renderHeader(m, w))

	b.WriteByte('\n')
	switch m.mode {
	case core.ModeActionMenu:
		b.WriteString(renderActionMenu(m, w))
	default:
		if len(m.windows) == 0 {
			b.WriteString(renderEmpty(m, w))
		} else {
			b.WriteString(renderSessionList(m, w))
		}
	}

	if m.mode == core.ModeSessionList || m.mode == core.ModeActionMenu {
		b.WriteByte('\n')
		b.WriteString(renderPreview(m, w))
	}

	b.WriteByte('\n')
	b.WriteString(renderStatusBar(m, w))

	b.WriteByte('\n')
	switch m.mode {
	case core.ModeFilter:
		b.WriteString(renderFilterBar(m, w))
	case core.ModeSendInput:
		b.WriteString(renderSendInputBar(m, w))
	case core.ModeConfirm:
		b.WriteString(renderConfirmBar(m, w))
	case core.ModeRename:
		b.WriteString(renderDialogBar(m, w, "Rename", "new name"))
	case core.ModeCommit:
		b.WriteString(renderDialogBar(m, w, "Commit", "message"))
	case core.ModeNewSession:
		b.WriteString(renderDialogBar(m, w, "New Session", "name"))
	case core.ModeNewWorktree:
		b.WriteString(renderDialogBar(m, w, "New Worktree", "branch-name"))
	default:
		b.WriteString(renderFooter(m, w))
	}

	if m.flashMessage != "" {
		style := core.SuccessStyle
		if m.flashIsError {
			style = core.ErrorStyle
		}
		b.WriteByte('\n')
		b.WriteString(style.Render(m.flashMessage))
	}

	return b.String()
}

// renderTabs renders the tab bar for the given active mode.
func renderTabs(activeMode core.ViewMode) string {
	type tab struct {
		label  string
		active bool
	}
	tabs := []tab{
		{"SESSIONS", activeMode == core.ModeSessionList || activeMode == core.ModeActionMenu ||
			activeMode == core.ModeFilter || activeMode == core.ModeConfirm ||
			activeMode == core.ModeRename || activeMode == core.ModeCommit ||
			activeMode == core.ModeNewSession || activeMode == core.ModeNewWorktree ||
			activeMode == core.ModeSendInput},
		{"PLANS", activeMode == core.ModePlans || activeMode == core.ModePlanFilter},
		{"ACTIVITY", activeMode == core.ModeActivity},
	}
	var parts []string
	for _, t := range tabs {
		if t.active {
			parts = append(parts, core.TabActiveStyle.Render(t.label))
		} else {
			parts = append(parts, core.TabInactiveStyle.Render(t.label))
		}
	}
	return strings.Join(parts, " ")
}

// renderHeader renders the mascot + title + tabs + attached session.
func renderHeader(m Model, w int) string {
	mascotLines := strings.Split(core.RenderMascot(), "\n")
	tabs := renderTabs(m.mode)
	titleLine := core.TitleStyle.Render("── tmux-overseer ──")

	right := ""
	if m.attachedSession != "" {
		right = core.AttachedStyle.Render("attached: " + m.attachedSession)
	}

	var lines []string
	for i, ml := range mascotLines {
		switch i {
		case 1:
			middle := titleLine + "  " + tabs
			mascotWidth := lipgloss.Width(ml)
			middleWidth := lipgloss.Width(middle)
			rightWidth := lipgloss.Width(right)

			totalUsed := mascotWidth + middleWidth + rightWidth + 2
			gap := w - totalUsed
			if gap < 1 {
				gap = 1
			}

			line := ml + " " + middle + strings.Repeat(" ", gap) + right
			lines = append(lines, line)
		default:
			lines = append(lines, ml)
		}
	}

	return strings.Join(lines, "\n")
}

// renderPreview renders the scrollable preview pane with separators.
// Height is m.previewHeight (configurable via [ and ]); offset via J/K.
func renderPreview(m Model, w int) string {
	maxPreview := m.previewHeight
	if maxPreview < 4 {
		maxPreview = 4
	}

	content := m.previewContent
	var allLines []string
	if content == "" {
		allLines = []string{core.DimRowStyle.Render("  (no preview available)")}
	} else {
		rawLines := strings.Split(content, "\n")
		allLines = make([]string, len(rawLines))
		for i, line := range rawLines {
			allLines[i] = "  " + line
		}
	}

	totalLines := len(allLines)

	// Default: show the last maxPreview lines (tail of the content).
	// With non-zero offset we scroll back from the tail.
	startIdx := totalLines - maxPreview - m.previewOffset
	if startIdx < 0 {
		startIdx = 0
	}
	endIdx := startIdx + maxPreview
	if endIdx > totalLines {
		endIdx = totalLines
	}
	visibleLines := allLines[startIdx:endIdx]

	hasAbove := startIdx > 0
	hasBelow := endIdx < totalLines

	// Top separator — include [/] hint and scroll indicator
	resizeHint := core.DimRowStyle.Render("[/] resize  JK scroll")
	sepWidth := w - lipgloss.Width(resizeHint) - 3
	if sepWidth < 1 {
		sepWidth = 1
	}
	topSepLine := core.PreviewSeparator.Render(strings.Repeat("─", sepWidth)) + " " + resizeHint
	if hasAbove {
		upIndicator := " " + core.ScrollIndicatorStyle.Render("▲")
		topSepLine = core.PreviewSeparator.Render(strings.Repeat("─", sepWidth-2)) + upIndicator + " " + resizeHint
	}

	bottomSep := core.PreviewSeparator.Render(strings.Repeat("─", w))
	if hasBelow {
		downIndicator := core.ScrollIndicatorStyle.Render("▼")
		bottomSepWidth := w - 2
		if bottomSepWidth < 1 {
			bottomSepWidth = 1
		}
		bottomSep = core.PreviewSeparator.Render(strings.Repeat("─", bottomSepWidth)) + " " + downIndicator
	}

	var b strings.Builder
	b.Grow(w*2 + maxPreview*80 + maxPreview + 2)
	b.WriteString(topSepLine)
	for i := 0; i < maxPreview; i++ {
		b.WriteByte('\n')
		if i < len(visibleLines) {
			b.WriteString(visibleLines[i])
		}
	}
	b.WriteByte('\n')
	b.WriteString(bottomSep)
	return b.String()
}

// renderStatusBar renders session/pane counts, source filter, and sort mode.
func renderStatusBar(m Model, w int) string {
	var working, waiting int
	var totalCost float64
	var cliCount, cursorCount, cloudCount int

	for _, win := range m.windows {
		totalCost += win.TotalCost()

		switch win.Source {
		case core.SourceCLI:
			cliCount++
		case core.SourceCursor:
			cursorCount++
		case core.SourceCloud:
			cloudCount++
		}

		for _, p := range win.Panes {
			switch p.Status {
			case core.StatusWorking:
				working++
			case core.StatusWaitingInput:
				waiting++
			}
		}
	}

	var left string
	if m.sourceFilter == core.FilterAll {
		parts := fmt.Sprintf(" %d cli  %d cursor", cliCount, cursorCount)
		if cloudCount > 0 {
			parts += fmt.Sprintf("  %d cloud", cloudCount)
		}
		left = parts
	} else {
		visibleCount := len(m.items)
		left = fmt.Sprintf(" %d %s sessions", visibleCount, m.sourceFilter.Label())
	}

	if working > 0 {
		left += core.StatusStyle(core.StatusWorking).Render(fmt.Sprintf("  ● %d working", working))
	}
	if waiting > 0 {
		left += core.StatusStyle(core.StatusWaitingInput).Render(fmt.Sprintf("  ◐ %d waiting", waiting))
	}
	dayTotal := m.dayCostTotal
	if totalCost > dayTotal {
		dayTotal = totalCost
	}
	left += core.CostStyle.Render(fmt.Sprintf("  $%.2f today", dayTotal))

	right := ""
	if m.sourceFilter != core.FilterAll {
		right += core.FilterActiveStyle.Render("filter: "+m.sourceFilter.Label()) + "  "
	}
	if m.groupMode != core.GroupBySource {
		right += core.FilterActiveStyle.Render("group: "+m.groupMode.Label()) + "  "
	}
	right += core.StatusBarStyle.Render("sort: " + m.sortMode.Label())

	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}

	return core.StatusBarStyle.Render(left) + strings.Repeat(" ", gap) + right
}

// renderFooter renders context-sensitive keybinding hints.
func renderFooter(m Model, _ int) string {
	var hints []string

	switch m.mode {
	case core.ModeSessionList:
		hints = []string{
			core.FooterKeyStyle.Render("↑↓") + " navigate",
			core.FooterKeyStyle.Render("l/→") + " actions",
			core.FooterKeyStyle.Render("enter") + " switch",
			core.FooterKeyStyle.Render("1/2/3") + " tabs",
			core.FooterKeyStyle.Render("f") + " source",
			core.FooterKeyStyle.Render("g") + " group",
			core.FooterKeyStyle.Render("/") + " filter",
			core.FooterKeyStyle.Render("?") + " help",
			core.FooterKeyStyle.Render("q") + " quit",
		}
	case core.ModeActionMenu:
		hints = []string{
			core.FooterKeyStyle.Render("↑↓") + " navigate",
			core.FooterKeyStyle.Render("enter") + " select",
			core.FooterKeyStyle.Render("h/←") + " back",
			core.FooterKeyStyle.Render("q") + " quit",
		}
	}

	return " " + core.FooterStyle.Render(strings.Join(hints, "  "))
}
