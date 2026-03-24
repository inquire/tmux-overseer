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

	statusStr := core.StatusStyle(p.Status).Render(p.Status.Symbol() + " " + p.Status.Label())

	lines = append(lines,
		core.ActionLabelStyle.Render("  session    ")+core.ActionValueStyle.Render(win.SessionName),
		core.ActionLabelStyle.Render("  window     ")+core.ActionValueStyle.Render(fmt.Sprintf("%d (%d panes)", win.WindowIndex, len(win.Panes))),
		core.ActionLabelStyle.Render("  status     ")+statusStr,
	)
	if p.Model != "" {
		lines = append(lines, core.ActionLabelStyle.Render("  model      ")+core.ActionValueStyle.Render(p.Model))
	}
	lines = append(lines, core.ActionLabelStyle.Render("  cost       ")+core.CostStyle.Render(fmt.Sprintf("$%.2f", win.TotalCost())))
	if win.ActivePlanTitle != "" {
		planLine := core.ActionLabelStyle.Render("  plan       ") + core.ActionValueStyle.Render(win.ActivePlanTitle)
		if win.ActivePlanTotal > 0 {
			maxB := 10
			if win.ActivePlanTotal < maxB {
				maxB = win.ActivePlanTotal
			}
			filled := (win.ActivePlanDone * maxB) / win.ActivePlanTotal
			bar := core.PlanBarFilledStyle.Render(strings.Repeat("■", filled)) +
				core.PlanBarEmptyStyle.Render(strings.Repeat("□", maxB-filled))
			planLine += "  " + bar + core.DimRowStyle.Render(fmt.Sprintf(" %d/%d", win.ActivePlanDone, win.ActivePlanTotal))
		}
		lines = append(lines, planLine)
	}
	lines = append(lines,
		core.ActionLabelStyle.Render("  path       ")+core.ActionValueStyle.Render(state.ShortenPath(p.WorkingDir)),
	)
	if p.HasGit {
		gitInfo := p.GitBranch
		if p.GitStaged {
			gitInfo += " +"
		}
		if p.GitDirty {
			gitInfo += " *"
		}
		lines = append(lines, core.ActionLabelStyle.Render("  branch     ")+core.GitBranchStyle.Render(gitInfo))
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
			lines = append(lines, core.ActionSeparatorStyle.Render("  ──────────"))
		}

		prefix := "  "
		if i == m.actionIdx {
			prefix = "> "
			lines = append(lines, core.ActionSelectedStyle.Render(prefix+action.Label()))
		} else {
			lines = append(lines, core.ActionNormalStyle.Render(prefix+action.Label()))
		}
	}

	if hasActionsAbove {
		lines[metadataLines] = lines[metadataLines] + " " + core.ScrollIndicatorStyle.Render("▲")
	}
	if hasActionsBelow && len(lines) > 0 {
		lines[len(lines)-1] = lines[len(lines)-1] + " " + core.ScrollIndicatorStyle.Render("▼")
	}

	return strings.Join(lines, "\n")
}

// renderEmpty renders the empty/loading state.
func renderEmpty(m Model, _ int) string {
	spinnerStr := core.StatusStyle(core.StatusWorking).Render(m.spinner.View())

	if m.loading {
		scanClaude := "      " + spinnerStr +
			core.EmptyMessageStyle.Render("  scanning ") +
			core.ClaudeBadgeStyle.Render("claude code") +
			core.EmptyMessageStyle.Render(" sessions")
		scanCursor := "      " + spinnerStr +
			core.EmptyMessageStyle.Render("  scanning ") +
			core.CursorBadgeStyle.Render("cursor ide") +
			core.EmptyMessageStyle.Render(" sessions")
		return strings.Join([]string{"", scanClaude, scanCursor, ""}, "\n")
	}

	lines := []string{
		"",
		core.EmptyMessageStyle.Render("      no sessions found"),
		"",
		core.EmptyMessageStyle.Render("      start " + core.ClaudeBadgeStyle.Render("Claude Code") + core.EmptyMessageStyle.Render(" in a tmux pane")),
		core.EmptyMessageStyle.Render("      or open " + core.CursorBadgeStyle.Render("Cursor IDE") + core.EmptyMessageStyle.Render(" with hooks enabled")),
		core.EmptyMessageStyle.Render("      and come back here " + spinnerStr),
		"",
		core.EmptyMessageStyle.Render("      press " + core.FooterKeyStyle.Render("n") + core.EmptyMessageStyle.Render(" to create a new session")),
		"",
	}

	return strings.Join(lines, "\n")
}

// renderFilterBar renders the filter input overlay.
func renderFilterBar(m Model, _ int) string {
	return " " + core.FooterKeyStyle.Render("/") + " filter: " + m.textInput.View()
}

// renderSendInputBar renders the send input overlay.
func renderSendInputBar(m Model, _ int) string {
	target := ""
	if m.actionWindow != nil {
		target = m.actionWindow.SessionName
	}
	return " " + core.FooterKeyStyle.Render("send to "+target+":") + " " + m.textInput.View() +
		"  " + core.FooterStyle.Render(core.FooterKeyStyle.Render("enter")+" send  "+core.FooterKeyStyle.Render("esc")+" cancel")
}

// renderConfirmBar renders the confirmation prompt.
func renderConfirmBar(m Model, _ int) string {
	return " " + core.ErrorStyle.Render(m.confirmMsg) + "  " +
		core.FooterStyle.Render(core.FooterKeyStyle.Render("y/enter")+" confirm  "+core.FooterKeyStyle.Render("n/esc")+" cancel")
}

// renderDialogBar renders a generic single-field dialog in the footer area.
func renderDialogBar(m Model, _ int, title, _ string) string {
	return " " + core.FooterKeyStyle.Render(title+":") + " " + m.textInput.View() +
		"  " + core.FooterStyle.Render(core.FooterKeyStyle.Render("enter")+" confirm  "+core.FooterKeyStyle.Render("esc")+" cancel")
}

// renderHelp renders the full keybinding help overlay.
func renderHelp(_ Model, w int) string {
	content := `
  Navigation
  ` + core.FooterKeyStyle.Render("↑/k") + `     move up
  ` + core.FooterKeyStyle.Render("↓/j") + `     move down
  ` + core.FooterKeyStyle.Render("l/→") + `     open actions
  ` + core.FooterKeyStyle.Render("tab") + `     expand/collapse panes
  ` + core.FooterKeyStyle.Render("enter") + `   switch to session
  
  Actions
  ` + core.FooterKeyStyle.Render("n") + `       new session
  ` + core.FooterKeyStyle.Render("i") + `       send input
  ` + core.FooterKeyStyle.Render("d") + `       kill session
  ` + core.FooterKeyStyle.Render("s") + `       cycle sort mode
  ` + core.FooterKeyStyle.Render("/") + `       filter
  ` + core.FooterKeyStyle.Render("f") + `       source filter
  ` + core.FooterKeyStyle.Render("p") + `       plans browser
  ` + core.FooterKeyStyle.Render("R") + `       refresh
  
  ` + core.FooterKeyStyle.Render("q/esc") + `   quit
  
  ` + core.DimRowStyle.Render("press any key to close")

	boxWidth := state.MinInt(42, w-4)
	box := core.DialogBorderStyle.Width(boxWidth).Render(
		core.DialogTitleStyle.Render("Keybindings") + content,
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

	spinnerStr := core.StatusStyle(core.StatusWorking).Render(m.spinner.View())
	dim := lipgloss.NewStyle().Foreground(core.ColorDim)
	gray := lipgloss.NewStyle().Foreground(core.ColorGray)

	title := lipgloss.NewStyle().
		Foreground(core.ColorCyan).
		Bold(true).
		Render("claude overseer")

	subtitle := dim.Render("session monitor")

	scanClaude := spinnerStr + gray.Render("  scanning ") +
		core.ClaudeBadgeStyle.Render("claude code") +
		gray.Render(" sessions")
	scanCursor := spinnerStr + gray.Render("  scanning ") +
		core.CursorBadgeStyle.Render("cursor ide") +
		gray.Render(" sessions")

	rule := dim.Render("─────────────────────")

	mascotLines := strings.Split(core.RenderMascot(), "\n")
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
