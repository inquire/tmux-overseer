package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/db"
)

func renderActivityView(m Model) string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var sections []string

	sections = append(sections, renderHeader(m, w))

	sep := core.SectionSeparatorStyle.Render(strings.Repeat("━", w))
	sections = append(sections, sep)

	if m.activityLoading {
		spinnerStr := core.StatusStyle(core.StatusWorking).Render(m.spinner.View())
		sections = append(sections, "  "+core.EmptyMessageStyle.Render("loading activity ")+spinnerStr)
	} else if len(m.activityGrid) == 0 && len(m.activityProjects) == 0 {
		sections = append(sections, "  "+core.EmptyMessageStyle.Render("no activity data — press S to sync plans"))
	} else {
		// Fixed layout — nothing moves regardless of content length:
		//   header(3) + sep(1) = 4 lines above content
		//   sep(1) + footer(1) + flash(1) = 3 lines below content
		//   heatmap block = 1(blank) + 1(sep) + 1(blank) + 9(heatmap) = 12 lines
		//   breakdown viewport = remaining height, minimum 3
		totalFixed := 4 + 3 // outer chrome
		heatmapBlock := 12  // always rendered below breakdown
		viewportLines := m.height - totalFixed - heatmapBlock
		if viewportLines < 3 {
			viewportLines = 3
		}
		// Allow user to override viewport height with [ / ] keys.
		if m.activityBreakdownLines > 0 {
			viewportLines = m.activityBreakdownLines
		}

		// Render full breakdown content then slice a fixed viewport into it.
		allDashLines := strings.Split(renderDayDashboard(m, w), "\n")
		totalLines := len(allDashLines)

		// Clamp scroll offset (activityScroll.Selected drives this).
		offset := m.activityScroll.Selected
		maxOffset := totalLines - viewportLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}
		if offset < 0 {
			offset = 0
		}

		// Slice exactly viewportLines rows and pad to fill the fixed height.
		end := offset + viewportLines
		if end > totalLines {
			end = totalLines
		}
		visible := allDashLines[offset:end]
		// Pad with blank lines so the separator below never moves.
		for len(visible) < viewportLines {
			visible = append(visible, "")
		}
		// Bottom of viewport: show scroll hint when there's more content.
		if totalLines > viewportLines {
			remaining := totalLines - (offset + viewportLines)
			hint := ""
			if offset > 0 && remaining > 0 {
				hint = fmt.Sprintf("  ↑↓ scroll  %d above · %d below", offset, remaining)
			} else if offset > 0 {
				hint = fmt.Sprintf("  ↑ scroll up  %d above", offset)
			} else {
				hint = fmt.Sprintf("  ↓ scroll down  %d more", remaining)
			}
			visible[viewportLines-1] = core.DimRowStyle.Render(hint)
		}

		// Assemble the fixed layout: breakdown viewport + anchored heatmap block.
		var content []string
		content = append(content, strings.Join(visible, "\n"))
		content = append(content, "")
		content = append(content, "  "+core.SectionSeparatorStyle.Render(strings.Repeat("─", w-4)))
		content = append(content, "")
		content = append(content, renderHeatmap(m, w))

		sections = append(sections, strings.Join(content, "\n"))
	}

	sections = append(sections, sep)

	if m.syncInProgress {
		spinnerStr := core.StatusStyle(core.StatusWorking).Render(m.spinner.View())
		progress := fmt.Sprintf("%s %s %d/%d  %s",
			spinnerStr, m.syncPhase, m.syncCurrent, m.syncTotal, m.syncDetail)
		sections = append(sections, " "+progress)
	} else {
		sections = append(sections, renderActivityFooter(w))
	}

	if m.flashMessage != "" {
		style := core.SuccessStyle
		if m.flashIsError {
			style = core.ErrorStyle
		}
		sections = append(sections, style.Render(m.flashMessage))
	}

	return strings.Join(sections, "\n")
}

// renderDayDashboard renders the "at a glance" today section:
// - One-line header with date, cost, session count, todos done
// - Per-project rows with cost, progress bar, source badges
// - Per-plan indented lines with progress and status
func renderDayDashboard(m Model, w int) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	// Determine whether we're showing today or a historical day.
	var displayDate time.Time
	isToday := true
	if len(m.activityGrid) > 0 {
		displayDate = m.activityGrid[m.activitySelectedDay].Date
		isToday = displayDate.Equal(today)
	} else {
		displayDate = today
	}

	// --- Header line ---
	dayLabel := ""
	if isToday {
		dayLabel = "TODAY"
	} else if displayDate.Equal(today.AddDate(0, 0, -1)) {
		dayLabel = "YESTERDAY"
	} else {
		dayLabel = strings.ToUpper(displayDate.Weekday().String())
	}

	dateStr := displayDate.Format("Jan 2")

	// Compute today's cost from live windows (only meaningful for today).
	var totalCost float64
	// Map workspace → {cost, sources}
	type wsInfo struct {
		cost    float64
		sources map[core.SessionSource]bool
	}
	wsByPath := make(map[string]*wsInfo)
	if isToday {
		for _, win := range m.windows {
			path := win.WorkspacePath
			if path == "" {
				if p := win.PrimaryPane(); p != nil {
					path = p.WorkingDir
				}
			}
			if path == "" {
				path = win.DisplayName()
			}
			c := win.TotalCost()
			totalCost += c
			if _, ok := wsByPath[path]; !ok {
				wsByPath[path] = &wsInfo{sources: make(map[core.SessionSource]bool)}
			}
			wsByPath[path].cost += c
			wsByPath[path].sources[win.Source] = true
		}
	}

	// Todos from day detail
	todosCompleted := m.activityDayDetail.TodosCompleted
	sessionCount := len(m.windows)
	if !isToday {
		sessionCount = m.activityDayDetail.PlansTouched // best proxy for historical
	}

	labelStyle := lipgloss.NewStyle().Foreground(core.ColorOrange).Bold(true)
	dimStyle := core.DimRowStyle

	left := "  " + labelStyle.Render(dayLabel) + "  " +
		core.NormalRowStyle.Render(dateStr)

	var rightParts []string
	if isToday && totalCost > 0 {
		rightParts = append(rightParts, core.CostStyle.Render(fmt.Sprintf("$%.2f", totalCost)))
	}
	if sessionCount > 0 {
		noun := "sessions"
		if sessionCount == 1 {
			noun = "session"
		}
		rightParts = append(rightParts, dimStyle.Render(fmt.Sprintf("%d %s", sessionCount, noun)))
	}
	if todosCompleted > 0 {
		rightParts = append(rightParts, core.SuccessStyle.Render(fmt.Sprintf("%d todos done", todosCompleted)))
	}
	right := strings.Join(rightParts, "  ")

	// Fill gap with dim dashes
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	dashCount := w - leftW - rightW - 4
	if dashCount < 1 {
		dashCount = 1
	}
	dashes := "  " + dimStyle.Render(strings.Repeat("─", dashCount)) + "  "
	headerLine := left + dashes + right

	var lines []string
	lines = append(lines, headerLine)
	lines = append(lines, "")

	// --- Per-project breakdown ---
	detail := m.activityDayDetail
	if len(detail.Projects) == 0 && !isToday {
		lines = append(lines, "  "+dimStyle.Render("no plan activity recorded for this day"))
		return strings.Join(lines, "\n")
	}

	// For today: combine DB detail with live cost data.
	// For history: use DB detail only.
	if len(detail.Projects) == 0 && isToday && len(m.windows) > 0 {
		lines = append(lines, "  "+dimStyle.Render("sessions active — press S to sync plan data"))
		return strings.Join(lines, "\n")
	}

	for _, proj := range detail.Projects {
		// Aggregate todos across all plans in this project
		var totalDone, totalAll int
		for _, pl := range proj.Plans {
			totalDone += pl.CompletedTodos
			totalAll += pl.TotalTodos
		}

		// Cost and sources from live data (today only)
		var projCost float64
		var projSources []string
		if isToday {
			// Match by project name or workspace
			for path, info := range wsByPath {
				if lastPathComponent(path) == proj.ProjectName || strings.HasSuffix(path, "/"+proj.ProjectName) {
					projCost = info.cost
					for src := range info.sources {
						switch src {
						case core.SourceCursor:
							projSources = append(projSources, core.CursorBadgeStyle.Render("[CURSOR]"))
						case core.SourceCLI:
							projSources = append(projSources, core.ClaudeBadgeStyle.Render("[CLAUDE]"))
						case core.SourceCloud:
							projSources = append(projSources, core.CloudBadgeStyle.Render("[CLOUD]"))
						}
					}
					break
				}
			}
		}

		// Project header line:
		// my-project           $2.83  ██████░░ 4/6 todos    [CURSOR][CLAUDE]
		projName := proj.ProjectName
		if len(projName) > 20 {
			projName = projName[:19] + "…"
		}
		nameStr := core.ActivityProjectStyle.Render(fmt.Sprintf("%-20s", projName))

		costStr := ""
		if projCost > 0 {
			costStr = core.CostStyle.Render(fmt.Sprintf("$%.2f", projCost)) + "  "
		}

		barWidth := 8
		filled := 0
		if totalAll > 0 {
			filled = (totalDone * barWidth) / totalAll
		}
		if filled > barWidth {
			filled = barWidth
		}
		bar := core.ActivityBarFilled.Render(strings.Repeat("█", filled)) +
			core.ActivityBarEmpty.Render(strings.Repeat("░", barWidth-filled))

		todoStr := dimStyle.Render(fmt.Sprintf("  %d/%d todos", totalDone, totalAll))
		badgeStr := ""
		if len(projSources) > 0 {
			badgeStr = "    " + strings.Join(projSources, "")
		}

		projLine := "  " + nameStr + "  " + costStr + bar + todoStr + badgeStr
		lines = append(lines, projLine)

		// Per-plan lines — skip noise (no todos, system/memory prompts, unnamed)
		var visiblePlans []db.DayPlanEntry
		for _, pl := range proj.Plans {
			t := strings.TrimSpace(pl.Title)
			if t == "" || t == "(untitled)" {
				continue
			}
			// Skip system/memory agent prompts and raw shell commands
			if strings.HasPrefix(t, "<") || strings.Contains(t, "memory agent") ||
				strings.HasPrefix(t, "Base directory for this skill") {
				continue
			}
			visiblePlans = append(visiblePlans, pl)
		}
		// If all plans were noise, skip the project entirely
		if len(visiblePlans) == 0 {
			lines = lines[:len(lines)-1] // remove project header
			continue
		}
		for i, pl := range visiblePlans {
			connector := "├─"
			if i == len(visiblePlans)-1 {
				connector = "└─"
			}

			planBarWidth := 8
			planFilled := 0
			if pl.TotalTodos > 0 {
				planFilled = (pl.CompletedTodos * planBarWidth) / pl.TotalTodos
			}
			if planFilled > planBarWidth {
				planFilled = planBarWidth
			}
			planBar := core.ActivityBarFilled.Render(strings.Repeat("█", planFilled)) +
				core.ActivityBarEmpty.Render(strings.Repeat("░", planBarWidth-planFilled))

			status := ""
			if pl.TotalTodos > 0 && pl.CompletedTodos == pl.TotalTodos {
				status = "  " + core.SuccessStyle.Render("✓ done")
			} else if pl.TotalTodos > 0 {
				status = "  " + lipgloss.NewStyle().Foreground(core.ColorYellow).Render("→ in progress")
			}

			countStr := dimStyle.Render(fmt.Sprintf(" %d/%d", pl.CompletedTodos, pl.TotalTodos))

			title := pl.Title
			maxTitle := w - 30
			if maxTitle < 15 {
				maxTitle = 15
			}
			if len(title) > maxTitle {
				title = title[:maxTitle-1] + "…"
			}

			planLine := "    " + dimStyle.Render(connector) + " " +
				core.NormalRowStyle.Render(title)

			// Right-align bar + count + status
			rightPart := planBar + countStr + status
			gap := w - lipgloss.Width(planLine) - lipgloss.Width(rightPart) - 2
			if gap < 1 {
				gap = 1
			}
			planLine += strings.Repeat(" ", gap) + rightPart
			lines = append(lines, planLine)
		}

		lines = append(lines, "")
	}

	// Remove trailing blank line
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

func renderHeatmap(m Model, w int) string {
	grid := m.activityGrid
	if len(grid) == 0 {
		return "  " + core.EmptyMessageStyle.Render("no heatmap data")
	}

	// Organize into weeks (columns) x days (rows, 0=Sun..6=Sat)
	type cell struct {
		date     time.Time
		score    int
		dayIndex int
	}

	var weeks [][]cell
	var currentWeek []cell

	for i, day := range grid {
		wd := int(day.Date.Weekday())
		if wd == 0 && len(currentWeek) > 0 {
			weeks = append(weeks, currentWeek)
			currentWeek = nil
		}
		currentWeek = append(currentWeek, cell{date: day.Date, score: day.Score, dayIndex: i})
	}
	if len(currentWeek) > 0 {
		weeks = append(weeks, currentWeek)
	}

	// Fit to terminal width: each week is 2 chars wide, plus label column (5 chars)
	maxWeeks := (w - 8) / 2
	if maxWeeks < 4 {
		maxWeeks = 4
	}
	if len(weeks) > maxWeeks {
		weeks = weeks[len(weeks)-maxWeeks:]
	}

	weeksLabel := fmt.Sprintf("%d", len(weeks))
	header := "  " + core.ActivityHeaderStyle.Render("Activity (last "+weeksLabel+" weeks)")

	// Legend
	legend := "  " + core.DimRowStyle.Render("less ") +
		heatChar(0, false) + heatChar(1, false) + heatChar(2, false) +
		heatChar(3, false) + heatChar(5, false) +
		core.DimRowStyle.Render(" more")
	gap := w - lipgloss.Width(header) - lipgloss.Width(legend) - 1
	if gap < 1 {
		gap = 1
	}
	headerLine := header + strings.Repeat(" ", gap) + legend

	// Month labels
	monthLine := "      "
	prevMonth := time.Month(0)
	for _, week := range weeks {
		if len(week) == 0 {
			monthLine += "  "
			continue
		}
		mo := week[0].date.Month()
		if mo != prevMonth {
			label := week[0].date.Format("Jan")
			monthLine += label
			if len(label) < 2 {
				monthLine += " "
			}
			prevMonth = mo
		} else {
			monthLine += "  "
		}
	}
	monthLine = "  " + core.DimRowStyle.Render(monthLine)

	dayLabels := []struct {
		label   string
		weekday int
	}{
		{"Sun", 0},
		{"Mon", 1},
		{"Tue", 2},
		{"Wed", 3},
		{"Thu", 4},
		{"Fri", 5},
		{"Sat", 6},
	}

	var lines []string
	lines = append(lines, headerLine)
	lines = append(lines, monthLine)

	selectedDayIdx := m.activitySelectedDay
	for _, dl := range dayLabels {
		row := "  " + core.DimRowStyle.Render(dl.label+" ")
		for _, week := range weeks {
			found := false
			for _, c := range week {
				if int(c.date.Weekday()) == dl.weekday {
					isSelected := c.dayIndex == selectedDayIdx
					row += heatChar(c.score, isSelected)
					found = true
					break
				}
			}
			if !found {
				row += "  "
			}
		}
		lines = append(lines, row)
	}

	return strings.Join(lines, "\n")
}

func heatChar(score int, selected bool) string {
	if selected {
		return core.HeatSelectedStyle.Render("[") + core.HeatSelectedStyle.Render("■") + core.HeatSelectedStyle.Render("]")
	}
	ch := "■"
	var style lipgloss.Style
	switch {
	case score == 0:
		style = core.HeatLevel0
	case score <= 2:
		style = core.HeatLevel1
	case score <= 5:
		style = core.HeatLevel2
	case score <= 10:
		style = core.HeatLevel3
	default:
		style = core.HeatLevel4
	}
	return style.Render(ch) + " "
}

// renderSelectedDayDetail renders the historical day detail when navigating
// to a day that is not today. Shows plan-level breakdown with progress.
func renderSelectedDayDetail(m Model, w int) string {
	if len(m.activityGrid) == 0 {
		return ""
	}

	detail := m.activityDayDetail
	if detail.PlansTouched == 0 && len(detail.Projects) == 0 {
		return ""
	}

	var lines []string

	for _, proj := range detail.Projects {
		lines = append(lines, "    "+core.GitBranchStyle.Render(proj.ProjectName))

		for _, dp := range proj.Plans {
			progress := ""
			if dp.TotalTodos > 0 {
				maxBlocks := 6
				filled := (dp.CompletedTodos * maxBlocks) / dp.TotalTodos
				progress = core.PlanProgressStyle.Render(
					strings.Repeat("▓", filled)+strings.Repeat("░", maxBlocks-filled)) +
					core.DimRowStyle.Render(fmt.Sprintf(" %d/%d", dp.CompletedTodos, dp.TotalTodos))
			}

			if dp.TotalTodos > 0 && dp.CompletedTodos == dp.TotalTodos {
				progress += " " + core.SuccessStyle.Render("✓ done!")
			}

			title := dp.Title
			maxTitle := w - lipgloss.Width(progress) - 16
			if maxTitle < 10 {
				maxTitle = 10
			}
			if lipgloss.Width(title) > maxTitle {
				title = title[:maxTitle-3] + "..."
			}

			line := "      " + core.NormalRowStyle.Render(title)
			if progress != "" {
				gap := w - lipgloss.Width(line) - lipgloss.Width(progress) - 2
				if gap < 1 {
					gap = 1
				}
				line += strings.Repeat(" ", gap) + progress
			}
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func renderActivityFooter(w int) string {
	keys := []string{
		core.FooterKeyStyle.Render("←→") + " navigate days",
		core.FooterKeyStyle.Render("↑↓") + " scroll",
		core.FooterKeyStyle.Render("[/]") + " resize",
		core.FooterKeyStyle.Render("S") + " sync",
		core.FooterKeyStyle.Render("R") + " reload",
		core.FooterKeyStyle.Render("p") + " plans",
		core.FooterKeyStyle.Render("s") + " sessions",
		core.FooterKeyStyle.Render("q") + " quit",
	}
	return " " + fitFooterKeys(keys, w)
}

