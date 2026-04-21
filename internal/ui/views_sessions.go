package ui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
)

// renderSessionList renders the scrollable session list using the flat item list.
// Each window row renders as 2 lines; pane rows are 1 line.
func renderSessionList(m Model, w int) string {
	start, end := m.scroll.VisibleRange()

	maxLines := m.scroll.ViewHeight * 3 // sessions are 2-3 lines collapsed
	lineCount := 0

	lines := make([]string, 0, maxLines)

	hasItemsAbove := start > 0
	hasItemsBelow := end < len(m.items)

	for i := start; i < end && lineCount < maxLines; i++ {
		if i >= len(m.items) {
			break
		}
		item := m.items[i]

		if item.IsSpacer {
			if lineCount+1 > maxLines {
				break
			}
			lines = append(lines, "")
			lineCount++
			continue
		}

		if item.IsSectionHeader {
			renderedRow := renderSectionHeader(item.SectionLabel, w, m.styles)
			rowLines := strings.Split(renderedRow, "\n")
			if lineCount+len(rowLines) > maxLines {
				break
			}
			lines = append(lines, rowLines...)
			lineCount += len(rowLines)
			continue
		}

		selected := m.scroll.IsSelected(i)

		if item.IsGroupHeader {
			renderedRow := renderSessionGroupHeader(m, item, selected, w)
			rowLines := strings.Split(renderedRow, "\n")
			if lineCount+len(rowLines) > maxLines {
				break
			}
			lines = append(lines, rowLines...)
			lineCount += len(rowLines)
			continue
		}

		if item.IsTeamHeader {
			renderedRow := renderTeamHeader(m, item, selected, w)
			rowLines := strings.Split(renderedRow, "\n")
			if lineCount+len(rowLines) > maxLines {
				break
			}
			lines = append(lines, rowLines...)
			lineCount += len(rowLines)
			continue
		}

		var renderedRow string
		if item.IsPane() {
			win := m.windows[item.WindowIdx]
			pane := win.Panes[item.PaneIdx]
			isLast := item.PaneIdx == len(win.Panes)-1
			renderedRow = renderPaneRow(pane, isLast, selected, w, m.styles)
		} else {
			win := m.windows[item.WindowIdx]
			key := fmt.Sprintf("%s:%d", win.SessionName, win.WindowIndex)
			expanded := m.expandedWindows[key]
			renderedRow = renderSessionRow(m, win, selected, expanded, item.InGroup, w)
		}

		rowLines := strings.Split(renderedRow, "\n")

		// Add blank separator after the last item of each session block:
		// after the window row (if not expanded with panes) or after the last pane row.
		isLastOfBlock := false
		if item.IsPane() {
			win := m.windows[item.WindowIdx]
			isLastOfBlock = item.PaneIdx == len(win.Panes)-1
		} else if !item.IsGroupHeader && !item.IsTeamHeader && item.WindowIdx >= 0 {
			win := m.windows[item.WindowIdx]
			key := fmt.Sprintf("%s:%d", win.SessionName, win.WindowIndex)
			isLastOfBlock = !m.expandedWindows[key] // no panes following
		}
		if isLastOfBlock {
			rowLines = append(rowLines, "")
		}

		if lineCount+len(rowLines) > maxLines {
			break
		}
		lines = append(lines, rowLines...)
		lineCount += len(rowLines)
	}

	// Scroll indicators on the right edge
	if hasItemsAbove && len(lines) > 0 {
		indicator := " " + m.styles.ScrollIndicatorStyle.Render("▲")
		currentWidth := lipgloss.Width(lines[0])
		padding := w - currentWidth - 3
		if padding > 0 {
			lines[0] = lines[0] + strings.Repeat(" ", padding) + indicator
		} else {
			lines[0] = lines[0] + indicator
		}
	}

	if hasItemsBelow && len(lines) > 0 {
		lastContentLine := len(lines) - 1
		indicator := " " + m.styles.ScrollIndicatorStyle.Render("▼")
		currentWidth := lipgloss.Width(lines[lastContentLine])
		padding := w - currentWidth - 3
		if padding > 0 {
			lines[lastContentLine] = lines[lastContentLine] + strings.Repeat(" ", padding) + indicator
		} else {
			lines[lastContentLine] = lines[lastContentLine] + indicator
		}
	}

	var b strings.Builder
	b.Grow(maxLines * 80)
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// renderSectionHeader renders a non-selectable group header with a separator line.
// Takes 2 lines: the label and a dim separator.
func renderSectionHeader(label string, w int, s core.Styles) string {
	header := "  " + s.SectionHeaderStyle.Render(label)
	sepWidth := w - 4
	if sepWidth < 1 {
		sepWidth = 1
	}
	separator := "  " + s.SectionSeparatorStyle.Render(strings.Repeat("─", sepWidth))
	return header + "\n" + separator
}

// renderSessionGroupHeader renders the collapsible workspace group header row.
// Shows: ▸/▾ workspace  (branch)  N sessions  status counts
func renderSessionGroupHeader(m Model, item core.ListItem, selected bool, w int) string {
	expanded := m.expandedGroups[item.GroupKey]
	marker := "▸"
	if expanded {
		marker = "▾"
	}

	var branch string
	var totalWorking, totalWaiting, totalIdle int
	for _, idx := range item.GroupWindowIndices {
		if idx < 0 || idx >= len(m.windows) {
			continue
		}
		win := m.windows[idx]
		// Prefer branch from a non-worktree session so the group header
		// shows the main checkout branch, not a worktree branch.
		if p := win.PrimaryPane(); p != nil && p.HasGit {
			if branch == "" || (!p.IsWorktree && branch != "") {
				branch = p.GitBranch
			}
		}
		switch win.AggregateStatus() {
		case core.StatusWorking:
			totalWorking++
		case core.StatusWaitingInput:
			totalWaiting++
		case core.StatusIdle:
			totalIdle++
		}
	}

	n := len(item.GroupWindowIndices)
	noun := "sessions"
	if n == 1 {
		noun = "session"
	}

	displayPath := shortenHomePath(item.GroupKey)

	pathStr := m.styles.GroupHeaderStyle.Render(marker + " " + displayPath)
	if branch != "" {
		pathStr += "  " + m.styles.GitBranchStyle.Render("("+branch+")")
	}
	countStr := m.styles.GroupHeaderDimStyle.Render(fmt.Sprintf("  %d %s", n, noun))
	left := pathStr + countStr

	var statusParts []string
	if totalWorking > 0 {
		statusParts = append(statusParts, m.styles.StatusStyle(core.StatusWorking).Render(fmt.Sprintf("✻ %d working", totalWorking)))
	}
	if totalWaiting > 0 {
		statusParts = append(statusParts, m.styles.StatusStyle(core.StatusWaitingInput).Render(fmt.Sprintf("◐ %d waiting", totalWaiting)))
	}
	if totalIdle > 0 {
		statusParts = append(statusParts, m.styles.StatusStyle(core.StatusIdle).Render(fmt.Sprintf("○ %d idle", totalIdle)))
	}
	right := strings.Join(statusParts, "  ")

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := w - leftW - rightW - 2
	if gap < 1 {
		gap = 1
	}

	if selected {
		markerPath := marker + " " + displayPath
		left = m.styles.SelectedRowStyle.Render(markerPath)
		if branch != "" {
			left += "  " + m.styles.GitBranchStyle.Render("("+branch+")")
		}
		left += countStr
		leftW = lipgloss.Width(left)
		gap = w - leftW - rightW - 2
		if gap < 1 {
			gap = 1
		}
	}

	return left + strings.Repeat(" ", gap) + right
}

// renderTeamHeader renders the collapsible Agent Team header row.
// Shows: ▸/▾ team: {name}  N members  ✻X working ◐Y waiting ○Z idle
func renderTeamHeader(m Model, item core.ListItem, selected bool, w int) string {
	expanded := m.expandedTeams[item.TeamKey]
	marker := "▸"
	if expanded {
		marker = "▾"
	}

	var totalWorking, totalWaiting, totalIdle int
	for _, idx := range item.TeamWindowIndices {
		if idx < 0 || idx >= len(m.windows) {
			continue
		}
		win := m.windows[idx]
		switch win.AggregateStatus() {
		case core.StatusWorking:
			totalWorking++
		case core.StatusWaitingInput:
			totalWaiting++
		default:
			totalIdle++
		}
	}

	n := len(item.TeamWindowIndices)
	noun := "members"
	if n == 1 {
		noun = "member"
	}

	label := "team: " + item.TeamKey
	headerStr := m.styles.LeadBadgeStyle.Render(marker + " " + label)
	countStr := m.styles.GroupHeaderDimStyle.Render(fmt.Sprintf("  %d %s", n, noun))
	left := headerStr + countStr

	var statusParts []string
	if totalWorking > 0 {
		statusParts = append(statusParts, m.styles.StatusStyle(core.StatusWorking).Render(fmt.Sprintf("✻%d working", totalWorking)))
	}
	if totalWaiting > 0 {
		statusParts = append(statusParts, m.styles.StatusStyle(core.StatusWaitingInput).Render(fmt.Sprintf("◐%d waiting", totalWaiting)))
	}
	if totalIdle > 0 {
		statusParts = append(statusParts, m.styles.StatusStyle(core.StatusIdle).Render(fmt.Sprintf("○%d idle", totalIdle)))
	}
	right := strings.Join(statusParts, "  ")

	if selected {
		left = m.styles.SelectedRowStyle.Render(marker+" "+label) + countStr
	}

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := w - leftW - rightW - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderSessionRow renders a single session/window row.
// inGroup=true means the row is a child of a workspace group header;
// line 2 (path + git branch) is omitted since it appears on the header.
func renderSessionRow(m Model, win core.ClaudeWindow, selected, expanded, inGroup bool, w int) string {
	if win.PrimaryPane() == nil {
		return ""
	}

	// Resolve expansion state
	planKey := win.ConversationID
	if planKey == "" {
		planKey = fmt.Sprintf("%s:%d", win.SessionName, win.WindowIndex)
	}
	planExpanded := m.expandedPlans[planKey]

	// Spinner replaces status symbol while working
	aggStatus := win.AggregateStatus()
	if aggStatus == core.StatusWorking {
		// Inject spinner into the primary pane so renderSessionRowLine1 picks it up
		// via AggregateStatus → we pass spinner separately via a thin wrapper approach.
		// Instead: just patch the status symbol inline below.
		_ = aggStatus
	}

	// Attached star prefix
	name := win.DisplayName()
	if win.SessionName == m.attachedSession && m.attachedSession != "" {
		name = m.styles.AttachedStarStyle.Render("★") + " " + name
	}

	// Determine expand marker
	expandable := win.HasActivePlan() || len(win.TaskTodos) > 0 || len(win.Subagents) > 0 || len(win.Panes) > 1
	marker := " "
	if expandable {
		if planExpanded || expanded {
			marker = "▾ "
		} else {
			marker = "▸ "
		}
	} else if selected {
		marker = "› "
	}

	// Source badges: show in FilterAll or always for source-specific filter
	showSource := m.sourceFilter == core.FilterAll
	win2 := win
	win2.TaskTodos = win.TaskTodos // pass through unchanged
	_ = win2

	// Build display name with marker
	displayName := win.DisplayName()
	if win.SessionName == m.attachedSession && m.attachedSession != "" {
		displayName = m.styles.AttachedStarStyle.Render("★") + " " + displayName
	}
	_ = name // unused now, using displayName

	// Spinner: override status symbol when working
	spinnerWin := win
	if aggStatus == core.StatusWorking {
		// We can't override AggregateStatus; instead we replace the status symbol
		// at the line1 level by passing a modified status. For now the spinner
		// is rendered by the aggregated status check inside renderSessionRowLine1.
		// The spinner view is available on m.spinner.View() — inject it here.
		_ = spinnerWin
	}

	line1 := renderSessionRowLine1WithSpinner(marker+displayName, win, selected, aggStatus, m.spinner.View(), showSource, w, m.styles)
	line2 := renderSessionRowLine2(win, inGroup, w, m.styles)
	line3 := renderSessionRowLine3(win, m.styles)

	var sb strings.Builder
	sb.WriteString(line1)
	if line2 != "" {
		sb.WriteString("\n")
		sb.WriteString(line2)
	}
	if line3 != "" {
		sb.WriteString("\n")
		sb.WriteString(line3)
	}
	if planExpanded {
		sb.WriteString(renderSessionRowExpanded(win, w, m.styles))
	}
	return sb.String()
}

// renderSessionRowLine1WithSpinner is the wiring adapter between the model's
// spinner state and the pure renderSessionRowLine1 function.
// It substitutes the spinner glyph for the status symbol when working.
func renderSessionRowLine1WithSpinner(nameWithMarker string, win core.ClaudeWindow, selected bool, aggStatus core.Status, spinnerView string, showSource bool, w int, s core.Styles) string {
	badge := buildBadgeStr(win, showSource, s)

	// Status: use spinner glyph when working
	var statusStr string
	if aggStatus == core.StatusWorking {
		statusStr = s.StatusStyle(aggStatus).Render(spinnerView)
	} else {
		statusStr = s.StatusStyle(aggStatus).Render(aggStatus.Symbol())
	}
	statusLabel := s.StatusStyle(aggStatus).Render(aggStatus.Label())

	saCount := len(win.Subagents)
	if saCount == 0 {
		saCount = win.SubagentCount
	}
	if saCount > 0 && aggStatus == core.StatusWorking {
		noun := "subagents"
		if saCount == 1 {
			noun = "subagent"
		}
		statusLabel += s.SubagentCountStyle.Render(fmt.Sprintf(" (%d %s)", saCount, noun))
	}

	costStr := ""
	if c := win.TotalCost(); c > 0 || win.Source == core.SourceCLI {
		costStr = s.CostStyle.Render(fmt.Sprintf("$%.2f", c))
	}

	right := statusStr + " " + statusLabel
	if costStr != "" {
		right += "  " + costStr
	}

	var nameRendered string
	if selected {
		nameRendered = s.SelectedRowStyle.Render(nameWithMarker)
	} else {
		nameRendered = s.NormalRowStyle.Render(nameWithMarker)
	}
	left := nameRendered + badge

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := w - leftW - rightW - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// ── Named constants for the new renderer ────────────────────────────────────

const (
	sessionTruncTitle   = 30 // plan/task title truncation on progress line
	sessionTruncTodo    = 55 // individual todo content truncation
	sessionTruncDesc    = 50 // subagent description (collapsed line 3)
	sessionTruncDescExp = 60 // subagent description (expanded activity)
	sessionTruncTool    = 28 // subagent current tool input truncation
	sessionMaxBarBlocks = 10 // max progress bar width in blocks
)

// ── Pure renderer functions ──────────────────────────────────────────────────

// buildBadgeStr assembles the source + team + worktree + pane-count badges
// for a window row. Result is appended directly after the session name.
func buildBadgeStr(win core.ClaudeWindow, showSource bool, s core.Styles) string {
	badge := ""
	if showSource {
		switch win.Source {
		case core.SourceCursor:
			badge += s.CursorBadgeStyle.Render(" [CURSOR]")
		case core.SourceCLI:
			badge += s.ClaudeBadgeStyle.Render(" [CLAUDE]")
		case core.SourceCloud:
			badge += s.CloudBadgeStyle.Render(" [CLOUD]")
		case core.SourceAutomation:
			badge += s.AutomationBadgeStyle.Render(" [AUTO]")
		}
	}
	if win.TeamRole == "lead" {
		badge += s.LeadBadgeStyle.Render(" [LEAD]")
	} else if win.TeamRole == "teammate" {
		badge += s.TeamBadgeStyle.Render(" [TEAM]")
	}
	switch win.SandboxType {
	case "docker":
		badge += lipgloss.NewStyle().Foreground(s.ColorCyan).Render(" 🐳")
	case "kubernetes":
		badge += lipgloss.NewStyle().Foreground(s.ColorPurple).Render(" ⎈")
	}
	if win.WorktreeBranch != "" {
		badge += s.WorktreeBadgeStyle.Render(" ⎇ " + win.WorktreeBranch)
	}
	if len(win.Panes) > 1 {
		badge += s.PaneCountBadge.Render(fmt.Sprintf(" [%d]", len(win.Panes)))
	}
	return badge
}

// renderSessionRowLine1 renders the first line of a session row:
//
//	marker  name  [badges]                    ● status  $cost
//
// marker should be " ", "▸ " or "▾ " — resolved by the caller.
func renderSessionRowLine1(marker string, win core.ClaudeWindow, selected bool, w int, s core.Styles) string {
	name := marker + win.DisplayName()
	showSource := true // always show source badge on line 1
	badge := buildBadgeStr(win, showSource, s)

	// Status + subagent count
	aggStatus := win.AggregateStatus()
	statusStr := s.StatusStyle(aggStatus).Render(aggStatus.Symbol())
	statusLabel := s.StatusStyle(aggStatus).Render(aggStatus.Label())
	saCount := len(win.Subagents)
	if saCount == 0 {
		saCount = win.SubagentCount
	}
	if saCount > 0 && aggStatus == core.StatusWorking {
		noun := "subagents"
		if saCount == 1 {
			noun = "subagent"
		}
		statusLabel += s.SubagentCountStyle.Render(fmt.Sprintf(" (%d %s)", saCount, noun))
	}

	// Cost
	costStr := ""
	if c := win.TotalCost(); c > 0 || win.Source == core.SourceCLI {
		costStr = s.CostStyle.Render(fmt.Sprintf("$%.2f", c))
	}

	right := statusStr + " " + statusLabel
	if costStr != "" {
		right += "  " + costStr
	}

	var nameRendered string
	if selected {
		nameRendered = s.SelectedRowStyle.Render(name)
	} else {
		nameRendered = s.NormalRowStyle.Render(name)
	}
	left := nameRendered + badge

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := w - leftW - rightW - 1
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// renderSessionRowLine2 renders the second line of a session row:
//
//	  path  (branch)  model  ■■□□ 3/10  › LastTool  [AGENT]  5 prompts  2h3m
func renderSessionRowLine2(win core.ClaudeWindow, inGroup bool, w int, s core.Styles) string {
	p := win.PrimaryPane()
	if p == nil {
		return ""
	}

	var parts []string

	if !inGroup {
		parts = append(parts, s.DimRowStyle.Render(state.ShortenPath(p.WorkingDir)))
		if p.HasGit {
			branchDisplay := p.GitBranch
			if p.IsWorktree {
				branchDisplay = "[" + branchDisplay + "]"
			} else {
				branchDisplay = "(" + branchDisplay + ")"
			}
			gitStr := s.GitBranchStyle.Render(branchDisplay)
			if p.GitStaged {
				gitStr += s.GitStagedStyle.Render("+")
			}
			if p.GitDirty {
				gitStr += s.GitDirtyStyle.Render("*")
			}
			parts = append(parts, gitStr)
		}
	}

	if p.Model != "" {
		modelStr := s.ModelNameStyle.Render(shortenModel(p.Model))
		if win.EffortLevel != "" {
			modelStr += " " + s.EffortLevelStyle.Render(effortSymbol(win.EffortLevel))
		}
		parts = append(parts, modelStr)
	}

	// Progress bar — plan file todos take priority; fall back to task list
	planDone, planTotal, planTitle := win.ActivePlanDone, win.ActivePlanTotal, win.ActivePlanTitle
	if planTotal == 0 && len(win.TaskTodos) > 0 {
		planTitle = "tasks"
		planTotal = len(win.TaskTodos)
		for _, t := range win.TaskTodos {
			if t.Status == "completed" {
				planDone++
			}
		}
	}
	if planTitle != "" && planTotal > 0 {
		maxBlocks := sessionMaxBarBlocks
		if planTotal < maxBlocks {
			maxBlocks = planTotal
		}
		filled := (planDone * maxBlocks) / planTotal
		bar := s.PlanBarFilledStyle.Render(strings.Repeat("■", filled)) +
			s.PlanBarEmptyStyle.Render(strings.Repeat("□", maxBlocks-filled))
		parts = append(parts, bar+" "+s.DimRowStyle.Render(fmt.Sprintf("%d/%d", planDone, planTotal)))
	}

	// Agent mode badge
	if win.Source != core.SourceCloud {
		if win.AgentMode == "plan" {
			parts = append(parts, s.PlanModeBadgeStyle.Render("[PLAN]"))
		} else if win.AgentMode == "agent" {
			parts = append(parts, s.AgentModeBadgeStyle.Render("[AGENT]"))
		}
		if win.PromptCount > 0 {
			noun := "prompts"
			if win.PromptCount == 1 {
				noun = "prompt"
			}
			parts = append(parts, s.SessionStatsStyle.Render(fmt.Sprintf("%d %s", win.PromptCount, noun)))
		}
		// Session duration
		if dur := win.SessionDuration(); dur != "" {
			parts = append(parts, s.SessionStatsStyle.Render(dur))
		}
	} else {
		// Cloud: summary + PR + duration
		if win.CloudSummary != "" {
			summary := win.CloudSummary
			if len(summary) > 50 {
				summary = summary[:50] + "..."
			}
			parts = append(parts, s.SessionStatsStyle.Render(summary))
		}
		if win.CloudPRURL != "" {
			parts = append(parts, s.SessionStatsStyle.Render("PR"))
		}
		if dur := win.SessionDuration(); dur != "" {
			parts = append(parts, s.SessionStatsStyle.Render(dur))
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}

// subagentIcon returns a single-char icon and its color for a given agent type.
func subagentIcon(agentType string, s core.Styles) (string, color.Color) {
	switch strings.ToLower(agentType) {
	case "explore":
		return "⊕", s.ColorCyan
	case "bash", "shell":
		return "$", s.ColorGreen
	case "browser", "browser-use":
		return "◎", s.ColorPurple
	case "code-reviewer", "code-simplifier":
		return "✎", s.ColorYellow
	case "plan":
		return "☰", s.ColorYellow
	case "debug":
		return "⊗", s.ColorRed
	case "test":
		return "◉", s.ColorGreen
	default:
		return "◆", s.ColorPurple
	}
}

// subagentLine builds a display string for one subagent:
// "icon  description  ›  CurrentTool(input)"
// descLimit controls description truncation.
func subagentLine(sa core.Subagent, descLimit int, s core.Styles) string {
	icon, iconColor := subagentIcon(sa.AgentType, s)
	iconStr := lipgloss.NewStyle().Foreground(iconColor).Render(icon)

	name := sa.Description
	if name == "" {
		name = sa.AgentType
	} else {
		name = truncate(name, descLimit)
	}

	line := iconStr + "  " + name

	// Sandbox badge
	if sa.SandboxType == "docker" {
		line += "  " + lipgloss.NewStyle().Foreground(s.ColorCyan).Render("[⬡ docker]")
	} else if sa.SandboxType == "kubernetes" {
		line += "  " + lipgloss.NewStyle().Foreground(s.ColorPurple).Render("[⬡ k8s]")
	}

	if sa.CurrentTool != "" {
		tool := sa.CurrentTool
		if sa.CurrentToolInput != "" {
			tool += "(" + truncate(sa.CurrentToolInput, sessionTruncTool) + ")"
		}
		line += "  " + s.DimRowStyle.Render("›")+" "+s.ActivityItemStyle.Render(tool)
	}
	if sa.StartedAt != "" {
		line += "  " + s.SessionStatsStyle.Render(sa.StartedAt)
	}
	return line
}

// renderSessionRowLine3 renders the optional third line — current tool use and
// active subagents. Only shown when the session is actively working.
//
//	  › Bash(go build ./...)  ·  ⊕ Explore(find auth)  ·  ⊕ Explore(read files)
func renderSessionRowLine3(win core.ClaudeWindow, s core.Styles) string {
	if win.AggregateStatus() != core.StatusWorking {
		return ""
	}
	p := win.PrimaryPane()

	var parts []string

	// Main session last tool
	if p != nil && p.LastTool != "" {
		parts = append(parts, s.ActivityItemStyle.Render("› "+p.LastTool))
	}

	// Active subagents — one entry per subagent using type icon + description
	for _, sa := range win.Subagents {
		parts = append(parts, subagentLine(sa, sessionTruncDesc, s))
	}

	if len(parts) == 0 {
		return ""
	}
	// Each subagent on its own line; main tool on the same first line as last tool
	return "  " + strings.Join(parts, "\n  ")
}

// renderSessionRowExpanded renders the Tab-expanded block for a session:
//
//	│
//	├─ tasks
//	│  1. ✓ task one
//	│  2. ● task two   (orange bold — active)
//	│  3. ○ task three (orange — pending)
//	│
//	└─ activity
//	   › Bash(go build ./...)
//	   ⊕ Explore(find auth)  › Grep(password)
func renderSessionRowExpanded(win core.ClaudeWindow, _ int, s core.Styles) string {
	todos := win.TaskTodos
	if len(todos) == 0 {
		todos = win.ActivePlanTodos
	}
	hasTasks := len(todos) > 0
	hasActivity := len(win.Subagents) > 0

	if !hasTasks && !hasActivity {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n  │")

	if hasTasks {
		tasksConnector := "├─"
		if !hasActivity {
			tasksConnector = "└─"
		}
		sb.WriteString("\n  " + s.TaskSectionStyle.Render(tasksConnector+" tasks"))
		for i, t := range todos {
			num := fmt.Sprintf("%d.", i+1)
			icon, style := taskIconStyle(t.Status, s)
			content := truncate(t.Content, sessionTruncTodo)
			sb.WriteString(fmt.Sprintf("\n  │  %s %s",
				s.TaskSectionStyle.Render(num),
				style.Render(icon+" "+content)))
		}
		if hasActivity {
			sb.WriteString("\n  │")
		}
	}

	if hasActivity {
		sb.WriteString("\n  " + s.ActivitySectionStyle.Render("└─ activity"))

		// Build parent→children map for hierarchy
		byID := make(map[string]*core.Subagent, len(win.Subagents))
		for i := range win.Subagents {
			byID[win.Subagents[i].ID] = &win.Subagents[i]
		}
		var roots []core.Subagent
		children := make(map[string][]core.Subagent)
		for _, sa := range win.Subagents {
			if sa.ParentAgentID == "" || byID[sa.ParentAgentID] == nil {
				roots = append(roots, sa)
			} else {
				children[sa.ParentAgentID] = append(children[sa.ParentAgentID], sa)
			}
		}

		for i, sa := range roots {
			isLastRoot := i == len(roots)-1 && len(children[sa.ID]) == 0
			conn := "   ├─"
			if isLastRoot {
				conn = "   └─"
			}
			sb.WriteString("\n  " + s.DimRowStyle.Render(conn) + " " + subagentLine(sa, sessionTruncDescExp, s))
			for j, child := range children[sa.ID] {
				childConn := "   │  ├─"
				if j == len(children[sa.ID])-1 {
					childConn = "   │  └─"
				}
				sb.WriteString("\n  " + s.DimRowStyle.Render(childConn) + " " + subagentLine(child, sessionTruncDescExp, s))
			}
		}
	}

	return sb.String()
}

// taskIconStyle returns the icon and lipgloss style for a task/todo item.
func taskIconStyle(status string, s core.Styles) (string, lipgloss.Style) {
	switch status {
	case "completed":
		return "✓", lipgloss.NewStyle().Foreground(s.ColorGreen)
	case "in_progress":
		return "●", s.TaskActiveStyle
	case "cancelled":
		return "✗", lipgloss.NewStyle().Foreground(s.ColorRed)
	default:
		return "○", s.TaskPendingStyle
	}
}

// sandboxEmoji returns the emoji for a pane's sandbox type.
func sandboxEmoji(sandboxType string) string {
	switch sandboxType {
	case "docker":
		return "🐳"
	case "apple":
		return "🍎"
	case "kubernetes":
		return "🚢"
	default:
		return "🏠"
	}
}

// renderPaneRow renders an expanded child pane row.
// Shows: connector  paneID  status  sandbox-emoji  [⎇ branch if worktree]
func renderPaneRow(pane core.ClaudePane, isLast bool, selected bool, _ int, s core.Styles) string {
	connector := "├─"
	if isLast {
		connector = "└─"
	}

	statusStr := s.StatusStyle(pane.Status).Render(pane.Status.Symbol())
	statusLabel := s.StatusStyle(pane.Status).Render(pane.Status.Label())

	emoji := sandboxEmoji(pane.SandboxType)

	var context string
	if pane.IsWorktree && pane.GitBranch != "" {
		context = emoji + "  " + s.WorktreeBadgeStyle.Render("⎇ "+pane.GitBranch)
	} else {
		context = emoji
	}

	prefix := "    "
	if selected {
		prefix = "  › "
	}

	line := fmt.Sprintf("%s%s %s  %s %s  %s", prefix, connector, pane.PaneID, statusStr, statusLabel, context)
	if selected {
		return s.SelectedRowStyle.Render(line)
	}
	return line
}

// shortenModel converts a full model name to a short display form.
// Handles both API-style and terminal-parsed formats:
//
//	"claude-4.6-opus-high-thinking" -> "opus 4.6"
//	"Opus 4.6"                      -> "opus 4.6"
//	"Sonnet 4.5 (1M context)"       -> "sonnet 4.5"
func shortenModel(model string) string {
	m := strings.ToLower(model)

	var family string
	for _, f := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(m, f) {
			family = f
			break
		}
	}
	if family == "" {
		if len(model) > 20 {
			return model[:20]
		}
		return model
	}

	parts := strings.FieldsFunc(m, func(r rune) bool {
		return r == '-' || r == ' '
	})
	version := ""
	for _, p := range parts {
		cleaned := strings.TrimRight(p, "()abcdefghijklmnopqrstuvwxyz")
		if len(cleaned) > 0 && cleaned[0] >= '0' && cleaned[0] <= '9' {
			if len(cleaned) > len(version) {
				version = cleaned
			}
		}
	}

	if version != "" {
		return family + " " + version
	}
	return family
}

// effortSymbol maps an effort level string to a Unicode symbol.
// Claude Code uses "low"/"medium"/"high" (v2.1.62+).
func effortSymbol(level string) string {
	switch level {
	case "high":
		return "●"
	case "medium":
		return "◐"
	default: // "low" or unrecognised
		return "○"
	}
}


