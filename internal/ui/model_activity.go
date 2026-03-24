package ui

import (
	"encoding/json"
	"errors"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/db"
	"github.com/inquire/tmux-overseer/internal/plans"
	"github.com/inquire/tmux-overseer/internal/state"
)

const activityHeatmapWeeks = 26

func loadActivityDataCmd() tea.Cmd {
	return func() tea.Msg {
		grid := db.GetActivityGrid(activityHeatmapWeeks)
		projects := db.GetProjectSummaries()

		actGrid := make([]core.ActivityDay, len(grid))
		for i, d := range grid {
			actGrid[i] = core.ActivityDay{Date: d.Date, Score: d.Score}
		}

		actProjects := make([]core.ActivityProject, len(projects))
		for i, p := range projects {
			actProjects[i] = core.ActivityProject{
				WorkspacePath:  p.WorkspacePath,
				Name:           p.Name,
				TotalPlans:     p.TotalPlans,
				CompletedTodos: p.CompletedTodos,
				TotalTodos:     p.TotalTodos,
				TotalScore:     p.TotalScore,
			}
		}

		allPlans := plans.ScanAll(50)
		activePlans := plans.FilterIncomplete(allPlans)

		return core.ActivityDataMsg{Grid: actGrid, Projects: actProjects, ActivePlans: activePlans}
	}
}

// forceSyncCmd runs the full plan scan + DB resync synchronously.
func forceSyncCmd() tea.Cmd {
	return func() tea.Msg {
		entries := plans.ScanAll(50)
		if len(entries) == 0 {
			if cached := state.LoadPlansCache(); cached != nil && len(cached.Plans) > 0 {
				var cachedEntries []core.PlanEntry
				if err := json.Unmarshal(cached.Plans, &cachedEntries); err == nil {
					entries = cachedEntries
				}
			}
		}
		if len(entries) == 0 {
			return core.SyncProgressMsg{
				Done: true,
				Err:  errors.New("no plans found from scanner or cache"),
			}
		}

		result, err := db.FullResync(entries, nil)
		if err != nil {
			return core.SyncProgressMsg{Done: true, Err: err}
		}

		return core.SyncProgressMsg{
			Done:         true,
			TotalPlans:   result.TotalPlans,
			NewPlans:     result.NewPlans,
			UpdatedPlans: result.UpdatedPlans,
			ProjectCount: result.ProjectCount,
			EventCount:   result.EventCount,
			ActivityDays: result.ActivityDays,
		}
	}
}

func (m Model) handleActivityKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		if len(m.activityGrid) > 0 && m.activitySelectedDay > 0 {
			m.activitySelectedDay--
			m.activityDayDetail = db.GetDayDetail(m.activityGrid[m.activitySelectedDay].Date)
			m.activityScroll.SetSelected(0)
		}
		return m, nil

	case "right", "l":
		if len(m.activityGrid) > 0 && m.activitySelectedDay < len(m.activityGrid)-1 {
			m.activitySelectedDay++
			m.activityDayDetail = db.GetDayDetail(m.activityGrid[m.activitySelectedDay].Date)
			m.activityScroll.SetSelected(0)
		}
		return m, nil

	case "up", "k":
		if m.activityScroll.Selected > 0 {
			m.activityScroll.MoveUp()
		}
		return m, nil

	case "down", "j":
		m.activityScroll.MoveDown()
		return m, nil

	case "[":
		auto := m.height - 4 - 3 - 12
		if auto < 3 {
			auto = 3
		}
		cur := m.activityBreakdownLines
		if cur == 0 {
			cur = auto
		}
		if cur > 3 {
			m.activityBreakdownLines = cur - 1
		}
		return m, nil

	case "]":
		auto := m.height - 4 - 3 - 12
		if auto < 3 {
			auto = 3
		}
		cur := m.activityBreakdownLines
		if cur == 0 {
			cur = auto
		}
		m.activityBreakdownLines = cur + 1
		return m, nil

	case "S":
		if m.syncInProgress {
			return m, nil
		}
		m.syncInProgress = true
		return m, forceSyncCmd()

	case "R":
		m.activityLoading = true
		return m, loadActivityDataCmd()
	}

	return m, nil
}

func (m Model) initActivitySelectedDay() Model {
	if len(m.activityGrid) == 0 {
		m.activitySelectedDay = 0
		return m
	}
	m.activitySelectedDay = len(m.activityGrid) - 1
	m.activityDayDetail = db.GetDayDetail(m.activityGrid[m.activitySelectedDay].Date)
	m.activityScroll = state.NewScrollState()
	m.activityScroll.SetTotal(200)
	m.activityScroll.SetViewHeight(1)
	return m
}

func (m Model) handleSyncProgress(msg core.SyncProgressMsg) (tea.Model, tea.Cmd) {
	m.syncInProgress = false

	if msg.Err != nil {
		m.flashMessage = "Sync failed: " + msg.Err.Error()
		m.flashIsError = true
		m.flashTicks = 0
		return m, nil
	}

	if msg.TotalPlans > 0 {
		m.flashMessage = fmt.Sprintf("Synced %d plans, %d projects | %d events, %d activity days | %d new, %d upd",
			msg.TotalPlans, msg.ProjectCount, msg.EventCount, msg.ActivityDays, msg.NewPlans, msg.UpdatedPlans)
	} else {
		m.flashMessage = "Sync complete (0 plans)"
	}
	m.flashIsError = false
	m.flashTicks = 0
	m.activityLoading = true
	return m, loadActivityDataCmd()
}
