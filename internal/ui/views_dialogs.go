package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
)

// renderActionMenu renders the action menu for a selected session.
func renderActionMenu(m Model, _ int) string {
	if m.actionWindow == nil {
		return ""
	}

	win := m.actionWindow
	p := win.PrimaryPane()
	if p == nil {
		return ""
	}

	var lines []string

	statusStr := m.styles.StatusStyle(p.Status).Render(p.Status.Symbol() + " " + p.Status.Label())

	lines = append(lines,
		m.styles.ActionLabelStyle.Render("  session    ")+m.styles.ActionValueStyle.Render(win.SessionName),
		m.styles.ActionLabelStyle.Render("  window     ")+m.styles.ActionValueStyle.Render(fmt.Sprintf("%d (%d panes)", win.WindowIndex, len(win.Panes))),
		m.styles.ActionLabelStyle.Render("  status     ")+statusStr,
	)
	if p.Model != "" {
		lines = append(lines, m.styles.ActionLabelStyle.Render("  model      ")+m.styles.ActionValueStyle.Render(p.Model))
	}
	lines = append(lines, m.styles.ActionLabelStyle.Render("  cost       ")+m.styles.CostStyle.Render(fmt.Sprintf("$%.2f", win.TotalCost())))
	if win.ActivePlanTitle != "" {
		planLine := m.styles.ActionLabelStyle.Render("  plan       ") + m.styles.ActionValueStyle.Render(win.ActivePlanTitle)
		if win.ActivePlanTotal > 0 {
			maxB := 10
			if win.ActivePlanTotal < maxB {
				maxB = win.ActivePlanTotal
			}
			filled := (win.ActivePlanDone * maxB) / win.ActivePlanTotal
			bar := m.styles.PlanBarFilledStyle.Render(strings.Repeat("■", filled)) +
				m.styles.PlanBarEmptyStyle.Render(strings.Repeat("□", maxB-filled))
			planLine += "  " + bar + m.styles.DimRowStyle.Render(fmt.Sprintf(" %d/%d", win.ActivePlanDone, win.ActivePlanTotal))
		}
		lines = append(lines, planLine)
	}
	lines = append(lines,
		m.styles.ActionLabelStyle.Render("  path       ")+m.styles.ActionValueStyle.Render(state.ShortenPath(p.WorkingDir)),
	)
	if p.HasGit {
		gitInfo := p.GitBranch
		if p.GitStaged {
			gitInfo += " +"
		}
		if p.GitDirty {
			gitInfo += " *"
		}
		lines = append(lines, m.styles.ActionLabelStyle.Render("  branch     ")+m.styles.GitBranchStyle.Render(gitInfo))
	}

	lines = append(lines, "")

	metadataLines := len(lines)
	maxActionLines := m.scroll.ViewHeight*2 - metadataLines
	if maxActionLines < 3 {
		maxActionLines = 3
	}

	visibleStart := 0
	visibleEnd := len(m.actions)

	if len(m.actions) > maxActionLines {
		center := maxActionLines / 2
		visibleStart = m.actionIdx - center
		if visibleStart < 0 {
			visibleStart = 0
		}
		visibleEnd = visibleStart + maxActionLines
		if visibleEnd > len(m.actions) {
			visibleEnd = len(m.actions)
			visibleStart = visibleEnd - maxActionLines
			if visibleStart < 0 {
				visibleStart = 0
			}
		}
	}

	hasActionsAbove := visibleStart > 0
	hasActionsBelow := visibleEnd < len(m.actions)

	for i := visibleStart; i < visibleEnd; i++ {
		action := m.actions[i]

		if action.IsSeparatorBefore() {
			lines = append(lines, m.styles.ActionSeparatorStyle.Render("  ──────────"))
		}

		prefix := "  "
		if i == m.actionIdx {
			prefix = "> "
			lines = append(lines, m.styles.ActionSelectedStyle.Render(prefix+action.Label()))
		} else {
			lines = append(lines, m.styles.ActionNormalStyle.Render(prefix+action.Label()))
		}
	}

	if hasActionsAbove {
		lines[metadataLines] = lines[metadataLines] + " " + m.styles.ScrollIndicatorStyle.Render("▲")
	}
	if hasActionsBelow && len(lines) > 0 {
		lines[len(lines)-1] = lines[len(lines)-1] + " " + m.styles.ScrollIndicatorStyle.Render("▼")
	}

	return strings.Join(lines, "\n")
}

// renderEmpty renders the empty/loading state.
func renderEmpty(m Model, _ int) string {
	spinnerStr := m.styles.StatusStyle(core.StatusWorking).Render(m.spinner.View())

	if m.loading {
		scanClaude := "      " + spinnerStr +
			m.styles.EmptyMessageStyle.Render("  scanning ") +
			m.styles.ClaudeBadgeStyle.Render("claude code") +
			m.styles.EmptyMessageStyle.Render(" sessions")
		scanCursor := "      " + spinnerStr +
			m.styles.EmptyMessageStyle.Render("  scanning ") +
			m.styles.CursorBadgeStyle.Render("cursor ide") +
			m.styles.EmptyMessageStyle.Render(" sessions")
		return strings.Join([]string{"", scanClaude, scanCursor, ""}, "\n")
	}

	lines := []string{
		"",
		m.styles.EmptyMessageStyle.Render("      no sessions found"),
		"",
		m.styles.EmptyMessageStyle.Render("      start " + m.styles.ClaudeBadgeStyle.Render("Claude Code") + m.styles.EmptyMessageStyle.Render(" in a tmux pane")),
		m.styles.EmptyMessageStyle.Render("      or open " + m.styles.CursorBadgeStyle.Render("Cursor IDE") + m.styles.EmptyMessageStyle.Render(" with hooks enabled")),
		m.styles.EmptyMessageStyle.Render("      and come back here " + spinnerStr),
		"",
		m.styles.EmptyMessageStyle.Render("      press " + m.styles.FooterKeyStyle.Render("n") + m.styles.EmptyMessageStyle.Render(" to create a new session")),
		"",
	}

	return strings.Join(lines, "\n")
}

// renderFilterBar renders the filter input overlay.
func renderFilterBar(m Model, _ int) string {
	return " " + m.styles.FooterKeyStyle.Render("/") + " filter: " + m.textInput.View()
}

// renderSendInputBar renders the send input overlay.
func renderSendInputBar(m Model, _ int) string {
	target := ""
	if m.actionWindow != nil {
		target = m.actionWindow.SessionName
	}
	return " " + m.styles.FooterKeyStyle.Render("send to "+target+":") + " " + m.textInput.View() +
		"  " + m.styles.FooterStyle.Render(m.styles.FooterKeyStyle.Render("enter")+" send  "+m.styles.FooterKeyStyle.Render("esc")+" cancel")
}

// renderConfirmBar renders the confirmation prompt.
func renderConfirmBar(m Model, _ int) string {
	return " " + m.styles.ErrorStyle.Render(m.confirmMsg) + "  " +
		m.styles.FooterStyle.Render(m.styles.FooterKeyStyle.Render("y/enter")+" confirm  "+m.styles.FooterKeyStyle.Render("n/esc")+" cancel")
}

// renderDialogBar renders a generic single-field dialog in the footer area.
func renderDialogBar(m Model, _ int, title, _ string) string {
	return " " + m.styles.FooterKeyStyle.Render(title+":") + " " + m.textInput.View() +
		"  " + m.styles.FooterStyle.Render(m.styles.FooterKeyStyle.Render("enter")+" confirm  "+m.styles.FooterKeyStyle.Render("esc")+" cancel")
}

// renderHelp renders the full keybinding help overlay.
func renderHelp(m Model, w int) string {
	content := `
  Navigation
  ` + m.styles.FooterKeyStyle.Render("↑/k") + `     move up
  ` + m.styles.FooterKeyStyle.Render("↓/j") + `     move down
  ` + m.styles.FooterKeyStyle.Render("l/→") + `     open actions
  ` + m.styles.FooterKeyStyle.Render("tab") + `     expand/collapse panes
  ` + m.styles.FooterKeyStyle.Render("enter") + `   switch to session

  Actions
  ` + m.styles.FooterKeyStyle.Render("n") + `       new session
  ` + m.styles.FooterKeyStyle.Render("i") + `       send input
  ` + m.styles.FooterKeyStyle.Render("d") + `       kill session
  ` + m.styles.FooterKeyStyle.Render("s") + `       cycle sort mode
  ` + m.styles.FooterKeyStyle.Render("/") + `       filter
  ` + m.styles.FooterKeyStyle.Render("f") + `       source filter
  ` + m.styles.FooterKeyStyle.Render("p") + `       plans browser
  ` + m.styles.FooterKeyStyle.Render("R") + `       refresh

  ` + m.styles.FooterKeyStyle.Render("q/esc") + `   quit

  ` + m.styles.DimRowStyle.Render("press any key to close")

	boxWidth := state.MinInt(42, w-4)
	box := m.styles.DialogBorderStyle.Width(boxWidth).Render(
		m.styles.DialogTitleStyle.Render("Keybindings") + content,
	)

	padding := (w - lipgloss.Width(box)) / 2
	if padding < 0 {
		padding = 0
	}

	return strings.Repeat("\n", 2) + strings.Repeat(" ", padding) + box
}

// renderLoadingScreen shows a centered loading spinner during initial load.
func renderLoadingScreen(m Model) string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}

	spinnerStr := m.styles.StatusStyle(core.StatusWorking).Render(m.spinner.View())
	dim := lipgloss.NewStyle().Foreground(m.styles.ColorDim)
	gray := lipgloss.NewStyle().Foreground(m.styles.ColorGray)

	title := lipgloss.NewStyle().
		Foreground(m.styles.ColorCyan).
		Bold(true).
		Render("claude overseer")

	subtitle := dim.Render("session monitor")

	scanClaude := spinnerStr + gray.Render("  scanning ") +
		m.styles.ClaudeBadgeStyle.Render("claude code") +
		gray.Render(" sessions")
	scanCursor := spinnerStr + gray.Render("  scanning ") +
		m.styles.CursorBadgeStyle.Render("cursor ide") +
		gray.Render(" sessions")

	rule := dim.Render("─────────────────────")

	mascotLines := strings.Split(m.styles.RenderMascot(), "\n")
	maxMascotWidth := 0
	for _, ml := range mascotLines {
		if lw := lipgloss.Width(ml); lw > maxMascotWidth {
			maxMascotWidth = lw
		}
	}
	mascotPad := (w - maxMascotWidth) / 2
	if mascotPad < 0 {
		mascotPad = 0
	}
	var centeredMascot []string
	for _, ml := range mascotLines {
		centeredMascot = append(centeredMascot, strings.Repeat(" ", mascotPad)+ml)
	}

	singleLines := []string{
		"",
		title,
		subtitle,
		"",
		rule,
		"",
		scanClaude,
		scanCursor,
		"",
	}

	centeredLines := []string{""}
	centeredLines = append(centeredLines, centeredMascot...)

	for _, line := range singleLines {
		lineWidth := lipgloss.Width(line)
		pad := (w - lineWidth) / 2
		if pad < 0 {
			pad = 0
		}
		centeredLines = append(centeredLines, strings.Repeat(" ", pad)+line)
	}

	content := strings.Join(centeredLines, "\n")

	contentHeight := len(centeredLines)
	topPadding := (h - contentHeight) / 2
	if topPadding < 0 {
		topPadding = 0
	}

	return strings.Repeat("\n", topPadding) + content
}
