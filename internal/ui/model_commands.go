package ui

import (
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/detect"
	"github.com/inquire/tmux-overseer/internal/tmux"
)

// refreshWindowsCmd runs ListAllSessions asynchronously and sends core.WindowsMsg.
// ListAllSessions and CurrentSessionName run concurrently since they are independent.
func refreshWindowsCmd() tea.Cmd {
	return func() tea.Msg {
		var (
			windows  []core.ClaudeWindow
			attached string
			err      error
		)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			windows, err = tmux.ListAllSessions()
		}()
		go func() {
			defer wg.Done()
			attached = tmux.CurrentSessionName()
		}()
		wg.Wait()
		return core.WindowsMsg{Windows: windows, AttachedSession: attached, Err: err}
	}
}

// recordAndAugmentCosts persists live costs to the daily ledger and
// fills in persisted costs for sessions where live cost is zero.
func (m *Model) recordAndAugmentCosts() {
	perSession, dayTotal := detect.DayCosts()
	m.dayCostPerSession = perSession
	m.dayCostTotal = dayTotal

	for i := range m.windows {
		win := &m.windows[i]
		sessionID := m.windowSessionID(win)
		liveCost := win.TotalCost()

		if liveCost > 0 {
			model := ""
			if p := win.PrimaryPane(); p != nil {
				model = p.Model
			}
			m.costTracker.RecordCost(
				sessionID,
				win.Source.Label(),
				win.DisplayName(),
				model,
				liveCost,
			)
		}

		if liveCost == 0 {
			if persisted, ok := perSession[sessionID]; ok && persisted > 0 {
				if len(win.Panes) > 0 {
					win.Panes[0].Cost = persisted
				}
			}
		}
	}
}

// windowSessionID returns a stable identifier for cost tracking.
func (m *Model) windowSessionID(win *core.ClaudeWindow) string {
	if win.ConversationID != "" {
		return "cursor:" + win.ConversationID
	}
	return fmt.Sprintf("cli:%s:%d", win.SessionName, win.WindowIndex)
}

// switchToWindow returns a command to switch to the given window's session.
func (m *Model) switchToWindow(w *core.ClaudeWindow) tea.Cmd {
	return func() tea.Msg {
		if w.Source == core.SourceCursor {
			err := tmux.SwitchToSession(*w)
			if err != nil {
				return core.GitResultMsg{Success: false, Message: err.Error()}
			}
		} else {
			tmux.SwitchToTarget(w.SessionName, w.WindowIndex, "")
		}
		return core.GitResultMsg{Success: true, Message: "Switched"}
	}
}

// removeCursorSession removes a Cursor session file.
func removeCursorSession(conversationID string) error {
	return detect.RemoveCursorSession(conversationID)
}
