package ui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/inquire/tmux-overseer/internal/core"
)

// renderPlansView renders the full-screen plans browser.
func renderPlansView(m Model) string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var sections []string

	sections = append(sections, renderHeader(m, w))
	sections = append(sections, renderPlansSubHeader(m, w))

	sep := m.styles.SectionSeparatorStyle.Render(strings.Repeat("━", w))
	sections = append(sections, sep)

	if m.plansLoading {
		spinnerStr := m.styles.StatusStyle(core.StatusWorking).Render(m.spinner.View())
		sections = append(sections, "  "+m.styles.EmptyMessageStyle.Render("loading plans ")+spinnerStr)
	} else if len(m.planItems) == 0 {
		sections = append(sections, "  "+m.styles.EmptyMessageStyle.Render("no plans found"))
	} else {
		sections = append(sections, renderPlanList(m, w))
		if m.planPreviewVisible {
			if preview := renderPlanPreviewPane(m, w); preview != "" {
				sections = append(sections, preview)
			}
		} else {
			if preview := renderPlanMiniPreview(m, w); preview != "" {
				sections = append(sections, preview)
			}
		}
	}

	sections = append(sections, sep)

	if m.syncInProgress {
		spinnerStr := m.styles.StatusStyle(core.StatusWorking).Render(m.spinner.View())
		progress := fmt.Sprintf("%s %s %d/%d  %s",
			spinnerStr, m.syncPhase, m.syncCurrent, m.syncTotal, m.syncDetail)
		sections = append(sections, " "+progress)
	} else if m.mode == core.ModePlanFilter {
		sections = append(sections, " "+m.styles.FooterKeyStyle.Render("/")+" filter: "+m.textInput.View())
	} else {
		sections = append(sections, renderPlansFooter(m, w))
	}

	if m.flashMessage != "" {
		style := m.styles.SuccessStyle
		if m.flashIsError {
			style = m.styles.ErrorStyle
		}
		sections = append(sections, style.Render(m.flashMessage))
	}

	return strings.Join(sections, "\n")
}

// renderPlansSubHeader renders the filter/count status line below the shared header.
func renderPlansSubHeader(m Model, w int) string {
	title := m.styles.PlanHeaderStyle.Render("📋 plans")

	countLabel := fmt.Sprintf("%d items", len(m.planItems))
	if n := len(m.planMultiSelected); n > 0 {
		countLabel = fmt.Sprintf("%d items  ", len(m.planItems)) + m.styles.FilterActiveStyle.Render(fmt.Sprintf("(%d selected)", n))
	}
	left := " " + title + "  " + m.styles.StatusBarStyle.Render(countLabel)

	var filters []string
	if m.planFilterText != "" {
		filters = append(filters, m.styles.FilterActiveStyle.Render("search: "+m.planFilterText))
	}
	if m.planSourceFilter != core.FilterAll {
		filters = append(filters, m.styles.FilterActiveStyle.Render("source: "+m.planSourceFilter.Label()))
	}
	filters = append(filters, m.styles.FilterActiveStyle.Render("group: "+m.planGroupMode.Label()))
	if m.planTagFilter != "" {
		filters = append(filters, m.styles.FilterActiveStyle.Render("tag: "+m.planTagFilter))
	}
	if m.planShowCompleted {
		filters = append(filters, m.styles.FilterActiveStyle.Render("showing completed"))
	}
	right := strings.Join(filters, "  ")

	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}

	return left + strings.Repeat(" ", gap) + right
}

// renderPlanList renders the scrollable list of plan entries, grouped by workspace.
func renderPlanList(m Model, w int) string {
	// Reserve space for: header(3) + subheader(1) + sep(1) + sep(1) + footer(1) + flash(1) + mini preview(~12)
	visibleLines := m.height - 20
	if visibleLines < 3 {
		visibleLines = 3
	}

	selected := m.planScroll.Selected

	type flatRow struct {
		text  string
		lines int
	}
	var flat []flatRow

	dayMode := m.planGroupMode == core.PlanGroupByDay

	cur := 0
	for gi := range m.planGroups {
		g := &m.planGroups[gi]
		if g.WorkspacePath == "" && !dayMode {
			for pi := range g.Plans {
				marked := m.planMultiSelected[cur]
				row := renderPlanRowFiltered(m.styles, g.Plans[pi], cur == selected, marked, false, false, w, m.planFilterText)
				rowLines := 2
				if g.Plans[pi].NextTodo() != "" {
					rowLines = 3
				}
				flat = append(flat, flatRow{text: row, lines: rowLines})
				cur++
			}
		} else {
			expanded := m.expandedPlanGroups[g.WorkspacePath]
			row := renderPlanGroupHeader(m.styles, *g, expanded, cur == selected, w)
			flat = append(flat, flatRow{text: row, lines: 1})
			cur++

			if expanded {
				for pi := range g.Plans {
					marked := m.planMultiSelected[cur]
					row := renderPlanRowFiltered(m.styles, g.Plans[pi], cur == selected, marked, true, dayMode, w, m.planFilterText)
					flat = append(flat, flatRow{text: row, lines: 1})
					cur++
				}
			}
		}
	}

	totalRows := len(flat)

	selFlat := selected
	if selFlat >= totalRows {
		selFlat = totalRows - 1
	}
	if selFlat < 0 {
		selFlat = 0
	}

	// Walk backwards from selFlat to keep the selected item ~1/4 from the top.
	budgetForAbove := visibleLines / 4
	linesAbove := 0
	startFlat := selFlat
	for startFlat > 0 {
		prev := flat[startFlat-1].lines
		if linesAbove+prev > budgetForAbove {
			break
		}
		linesAbove += prev
		startFlat--
	}

	var lines []string
	usedLines := 0

	if startFlat > 0 {
		lines = append(lines, m.styles.DimRowStyle.Render(fmt.Sprintf("  ▲ %d more", startFlat)))
		usedLines++
	}

	endFlat := startFlat
	for fi := startFlat; fi < totalRows; fi++ {
		if usedLines+flat[fi].lines > visibleLines {
			break
		}
		lines = append(lines, flat[fi].text)
		usedLines += flat[fi].lines
		endFlat = fi + 1
	}

	if endFlat < totalRows {
		lines = append(lines, m.styles.DimRowStyle.Render(fmt.Sprintf("  ▼ %d more", totalRows-endFlat)))
	}

	return strings.Join(lines, "\n")
}

// renderPlanGroupHeader renders a collapsible workspace group header row in the plans view.
func renderPlanGroupHeader(s core.Styles, g core.PlanGroup, expanded, selected bool, w int) string {
	marker := "▸"
	if expanded {
		marker = "▾"
	}

	displayLabel := shortenHomePath(g.WorkspacePath)
	if g.Label != "" {
		displayLabel = g.Label
	}

	n := len(g.Plans)
	noun := "plans"
	if n == 1 {
		noun = "plan"
	}

	markerPath := marker + " " + displayLabel
	countStr := s.GroupHeaderDimStyle.Render(fmt.Sprintf("  %d %s", n, noun))

	completed := 0
	for _, p := range g.Plans {
		if p.IsCompleted() {
			completed++
		}
	}
	summaryStr := ""
	if n > 0 {
		summaryStr = s.GroupHeaderDimStyle.Render(fmt.Sprintf("  %d/%d done", completed, n))
	}

	var left string
	if selected {
		left = s.SelectedRowStyle.Render(markerPath) + countStr + summaryStr
	} else {
		left = s.GroupHeaderStyle.Render(markerPath) + countStr + summaryStr
	}

	pad := w - lipgloss.Width(left) - 1
	if pad > 0 {
		left += strings.Repeat(" ", pad)
	}
	return left
}

// renderPlanRow renders a single plan entry.
// inGroup=true → compact single line; inGroup=false → two lines.
// showWorkspace=true → append shortened workspace path (used in day grouping).
// marked=true → item is part of a multi-selection.
func renderPlanRow(s core.Styles, plan core.PlanEntry, selected, marked, inGroup, showWorkspace bool, w int) string {
	return renderPlanRowFiltered(s, plan, selected, marked, inGroup, showWorkspace, w, "")
}

func renderPlanRowFiltered(s core.Styles, plan core.PlanEntry, selected, marked, inGroup, showWorkspace bool, w int, filterText string) string {
	indent := ""
	if inGroup {
		indent = "    "
	}

	marker := indent + "  "
	titleStyle := s.NormalRowStyle
	if marked && selected {
		marker = indent + "●›"
		titleStyle = s.SelectedRowStyle
	} else if marked {
		marker = indent + "● "
		titleStyle = s.MarkedRowStyle
	} else if selected {
		marker = indent + "› "
		titleStyle = s.SelectedRowStyle
	}

	var badge string
	if inGroup {
		if plan.Source == core.SourceCLI {
			badge = " 🟠"
		} else {
			badge = " 🟣"
		}
	} else {
		if plan.Source == core.SourceCLI {
			badge = "🟠 " + s.ClaudeBadgeStyle.Render("CLAUDE")
		} else {
			badge = "🟣 " + s.CursorBadgeStyle.Render("CURSOR")
		}
	}

	// Tag pills (S3)
	tagStr := renderTagPills(s, plan.Tags)

	progress := plan.ProgressBar()
	if progress != "" {
		progress = "  " + s.PlanProgressStyle.Render(progress)
	}

	if inGroup {
		rightHint := ""
		if showWorkspace && plan.WorkspacePath != "" {
			folder := lastPathComponent(plan.WorkspacePath)
			rightHint = s.GitBranchStyle.Render(folder)
		} else {
			rightHint = s.PlanDateStyle.Render(relativeDate(plan.LastActive))
		}

		maxTitle := w - lipgloss.Width(marker) - lipgloss.Width(badge) - lipgloss.Width(tagStr) - lipgloss.Width(progress) - lipgloss.Width(rightHint) - 4
		if maxTitle < 8 {
			maxTitle = 8
		}
		title := plan.Title
		if lipgloss.Width(title) > maxTitle {
			title = title[:maxTitle-3] + "..."
		}
		left := marker + highlightMatch(s, title, filterText, titleStyle) + tagStr + badge + progress
		gap := w - lipgloss.Width(left) - lipgloss.Width(rightHint) - 1
		if gap < 1 {
			gap = 1
		}
		return left + strings.Repeat(" ", gap) + rightHint
	}

	// S1: 3-line card layout for ungrouped plans
	date := relativeDate(plan.LastActive)
	dateStr := s.PlanDateStyle.Render(date)

	maxTitle := w - lipgloss.Width(badge) - lipgloss.Width(tagStr) - lipgloss.Width(progress) - lipgloss.Width(marker) - 4
	if maxTitle < 10 {
		maxTitle = 10
	}
	title := plan.Title
	if lipgloss.Width(title) > maxTitle {
		title = title[:maxTitle-3] + "..."
	}
	line1 := marker + highlightMatch(s, title, filterText, titleStyle) + tagStr + "  " + badge + progress

	// Line 2: repo · progress · date
	repo := lastPathComponent(plan.WorkspacePath)
	if repo == "" {
		repo = plan.Overview
	}
	repoMaxW := w - lipgloss.Width(dateStr) - 8
	if lipgloss.Width(repo) > repoMaxW {
		repo = repo[:repoMaxW-3] + "..."
	}
	repoStr := s.GitBranchStyle.Render(repo)
	gap := w - lipgloss.Width("  "+repoStr) - lipgloss.Width(dateStr) - 2
	if gap < 1 {
		gap = 1
	}
	line2 := "  " + repoStr + strings.Repeat(" ", gap) + dateStr

	// Line 3: Next actionable todo
	nextTodo := plan.NextTodo()
	if nextTodo != "" {
		maxNext := w - 12
		if maxNext < 10 {
			maxNext = 10
		}
		if lipgloss.Width(nextTodo) > maxNext {
			nextTodo = nextTodo[:maxNext-3] + "..."
		}
		line3 := "  " + s.DimRowStyle.Render("Next: "+nextTodo)
		return line1 + "\n" + line2 + "\n" + line3
	}

	return line1 + "\n" + line2
}

// renderTagPills renders colored tag pills for a plan entry.
func renderTagPills(s core.Styles, tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	var pills []string
	for _, tag := range tags {
		pills = append(pills, " "+s.TagPillStyle(tag).Render(tag))
	}
	return strings.Join(pills, "")
}

// renderPlansFooter renders the keybindings for the plans view.
func renderPlansFooter(m Model, w int) string {
	n := len(m.planMultiSelected)
	if n > 0 {
		keys := []string{
			m.styles.FilterActiveStyle.Render(fmt.Sprintf("%d selected", n)),
			m.styles.FooterKeyStyle.Render("t") + " title",
			m.styles.FooterKeyStyle.Render("d") + " delete",
			m.styles.FooterKeyStyle.Render("esc") + " clear",
		}
		return " " + strings.Join(keys, "  ")
	}

	keys := []string{
		m.styles.FooterKeyStyle.Render("↑↓") + " nav",
		m.styles.FooterKeyStyle.Render("enter") + " resume",
		m.styles.FooterKeyStyle.Render("⇧↑↓") + " select",
		m.styles.FooterKeyStyle.Render("f") + " source",
		m.styles.FooterKeyStyle.Render("g") + " group",
		m.styles.FooterKeyStyle.Render("T") + " tag",
		m.styles.FooterKeyStyle.Render("r") + " restructure",
		m.styles.FooterKeyStyle.Render("t") + " title",
		m.styles.FooterKeyStyle.Render("v") + " preview",
		m.styles.FooterKeyStyle.Render("d") + " del",
		m.styles.FooterKeyStyle.Render("S") + " sync",
		m.styles.FooterKeyStyle.Render("R") + " reload",
		m.styles.FooterKeyStyle.Render("1/2/3") + " tabs",
	}

	return " " + fitFooterKeys(keys, w)
}

// renderPlanPreviewPane renders a full markdown preview of the selected plan file.
func renderPlanPreviewPane(m Model, w int) string {
	if !m.planPreviewVisible || len(m.planPreviewLines) == 0 {
		return ""
	}

	h := m.planPreviewHeight
	allLines := m.planPreviewLines

	start := m.planPreviewOffset
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + h
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := allLines[start:end]

	sep := m.styles.SectionSeparatorStyle.Render(strings.Repeat("─", w))
	var lines []string
	lines = append(lines, sep)

	inFrontmatter := false
	inCodeBlock := false

	// Check state before visible window
	for i := 0; i < start && i < len(allLines); i++ {
		trimmed := strings.TrimSpace(allLines[i])
		if trimmed == "---" {
			inFrontmatter = !inFrontmatter
		}
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
		}
	}

	for _, line := range visible {
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			inFrontmatter = !inFrontmatter
			lines = append(lines, " "+m.styles.DimRowStyle.Render(line))
			continue
		}
		if inFrontmatter {
			lines = append(lines, " "+m.styles.DimRowStyle.Render(line))
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			lines = append(lines, " "+m.styles.DimRowStyle.Render(line))
			continue
		}
		if inCodeBlock {
			lines = append(lines, " "+m.styles.DimRowStyle.Render(line))
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "# "):
			lines = append(lines, " "+m.styles.PlanTitleStyle.Render(line))
		case strings.HasPrefix(trimmed, "## "):
			lines = append(lines, " "+m.styles.ActivityHeaderStyle.Render(line))
		case strings.HasPrefix(trimmed, "### "):
			lines = append(lines, " "+m.styles.GitBranchStyle.Render(line))
		case strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]"):
			lines = append(lines, " "+m.styles.SuccessStyle.Render("✓")+m.styles.NormalRowStyle.Render(line[5:]))
		case strings.HasPrefix(trimmed, "- [ ]"):
			lines = append(lines, " "+m.styles.DimRowStyle.Render("○")+m.styles.NormalRowStyle.Render(line[5:]))
		case strings.HasPrefix(trimmed, "- "):
			lines = append(lines, " "+m.styles.NormalRowStyle.Render(line))
		default:
			lines = append(lines, " "+m.styles.PreviewContentStyle.Render(line))
		}
	}

	return strings.Join(lines, "\n")
}

// renderPlanMiniPreview renders a compact inline preview of the selected plan's
// overview, progress bar, and todos. Shown between the plan list and the bottom separator.
func renderPlanMiniPreview(m Model, w int) string {
	if m.planScroll.Selected < 0 || m.planScroll.Selected >= len(m.planItems) {
		return ""
	}
	plan := m.planItems[m.planScroll.Selected]

	sep := m.styles.SectionSeparatorStyle.Render(strings.Repeat("─", w))
	var lines []string
	lines = append(lines, sep)

	// Title + source badge + tags
	sourceBadge := m.styles.ClaudeBadgeStyle.Render("CLAUDE")
	if plan.Source == core.SourceCursor {
		sourceBadge = m.styles.CursorBadgeStyle.Render("CURSOR")
	}
	titleStr := m.styles.PlanTitleStyle.Render(truncatePlanStr(plan.Title, w-20))
	lines = append(lines, "  "+titleStr+"  "+sourceBadge)

	// Overview
	if plan.Overview != "" {
		ov := plan.Overview
		if lipgloss.Width(ov) > w-4 {
			ov = ov[:w-7] + "..."
		}
		lines = append(lines, "  "+m.styles.PlanOverviewStyle.Render(ov))
	}

	// Color-coded progress bar (S2)
	if len(plan.Todos) > 0 {
		pct := plan.CompletionPct()
		total := len(plan.Todos)
		done := plan.CompletedCount()
		barWidth := 20
		filled := 0
		if total > 0 {
			filled = (done * barWidth) / total
		}

		var barStyle lipgloss.Style
		switch {
		case pct < 50:
			barStyle = m.styles.ProgressBarGreenStyle
		case pct < 70:
			barStyle = m.styles.ProgressBarYellowStyle
		default:
			barStyle = m.styles.ProgressBarRedStyle
		}

		bar := barStyle.Render(strings.Repeat("█", filled)) +
			m.styles.ProgressBarEmptyStyle.Render(strings.Repeat("░", barWidth-filled))
		pctStr := m.styles.DimRowStyle.Render(fmt.Sprintf(" %d%%  (%d/%d)", pct, done, total))
		lines = append(lines, "")
		lines = append(lines, "  Progress "+bar+pctStr)
		lines = append(lines, "")
	}

	// Todos sorted by status (S2): up to 5 with ← current marker
	if len(plan.Todos) > 0 {
		sorted := plan.SortedTodos()
		maxShow := 5
		if len(sorted) < maxShow {
			maxShow = len(sorted)
		}
		firstInProgress := true
		for i := 0; i < maxShow; i++ {
			todo := sorted[i]
			var icon string
			var style lipgloss.Style
			suffix := ""
			switch todo.Status {
			case "completed":
				icon = "✓"
				style = lipgloss.NewStyle().Foreground(m.styles.ColorGreen)
			case "in_progress":
				icon = "●"
				style = lipgloss.NewStyle().Foreground(m.styles.ColorYellow)
				if firstInProgress {
					suffix = m.styles.DimRowStyle.Render("  ← current")
					firstInProgress = false
				}
			case "cancelled":
				icon = "✗"
				style = lipgloss.NewStyle().Foreground(m.styles.ColorDim)
			default:
				icon = "○"
				style = lipgloss.NewStyle().Foreground(m.styles.ColorDim)
			}
			content := truncatePlanStr(todo.Content, w-20)
			lines = append(lines, "    "+style.Render(icon+" "+content)+suffix)
		}
		if len(plan.Todos) > maxShow {
			lines = append(lines, "    "+m.styles.DimRowStyle.Render(fmt.Sprintf("... %d more todos", len(plan.Todos)-maxShow)))
		}
	}

	return strings.Join(lines, "\n")
}

// highlightMatch returns the title string with filterText highlighted using FilterMatchStyle.
// Falls back to the raw title if no match is found or filter is empty.
func highlightMatch(s core.Styles, title, filter string, baseStyle lipgloss.Style) string {
	if filter == "" {
		return baseStyle.Render(title)
	}
	lower := strings.ToLower(title)
	lowerFilter := strings.ToLower(filter)
	idx := strings.Index(lower, lowerFilter)
	if idx < 0 {
		return baseStyle.Render(title)
	}

	// Split into rune slices to handle multi-byte characters
	runes := []rune(title)
	filterRunes := []rune(filter)
	runesLower := []rune(lower)
	filterLower := []rune(lowerFilter)

	// Find the rune-level index
	runeIdx := -1
	for i := 0; i <= len(runesLower)-len(filterLower); i++ {
		match := true
		for j, fr := range filterLower {
			if runesLower[i+j] != fr {
				match = false
				break
			}
		}
		if match {
			runeIdx = i
			break
		}
	}
	if runeIdx < 0 {
		return baseStyle.Render(title)
	}

	before := string(runes[:runeIdx])
	matched := string(runes[runeIdx : runeIdx+len(filterRunes)])
	after := string(runes[runeIdx+len(filterRunes):])
	_ = utf8.RuneError // ensure unicode/utf8 import is used

	return baseStyle.Render(before) + s.FilterMatchStyle.Render(matched) + baseStyle.Render(after)
}

func truncatePlanStr(s string, max int) string {
	if max < 4 {
		max = 4
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// fitFooterKeys joins keys with spacing, truncating from the end if they exceed width.
func fitFooterKeys(keys []string, w int) string {
	if w <= 0 {
		w = 80
	}
	result := strings.Join(keys, "  ")
	if lipgloss.Width(result) <= w-2 {
		return result
	}
	var out string
	for i, k := range keys {
		sep := ""
		if i > 0 {
			sep = "  "
		}
		candidate := out + sep + k
		if lipgloss.Width(candidate) > w-4 {
			break
		}
		out = candidate
	}
	return out
}

// relativeDate returns a human-friendly date string relative to now.
func relativeDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		if m == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", h)
	case diff < 48*time.Hour:
		return "yesterday"
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}
