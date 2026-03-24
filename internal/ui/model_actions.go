package ui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/git"
	"github.com/inquire/tmux-overseer/internal/state"
	"github.com/inquire/tmux-overseer/internal/tmux"
)

func (m Model) handleActionMenuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		m.quitting = true
		return m, tea.Quit

	case "h", "left", "esc":
		m.mode = core.ModeSessionList

	case "j", "down":
		if m.actionIdx < len(m.actions)-1 {
			m.actionIdx++
		}

	case "k", "up":
		if m.actionIdx > 0 {
			m.actionIdx--
		}

	case "enter", "l", "right":
		if m.actionIdx < len(m.actions) {
			return m.executeAction(m.actions[m.actionIdx])
		}
	}

	return m, nil
}

func (m Model) executeAction(action core.SessionAction) (tea.Model, tea.Cmd) {
	w := m.actionWindow
	if w == nil {
		return m, nil
	}
	p := w.PrimaryPane()
	if p == nil {
		return m, nil
	}

	switch action {
	case core.ActionSwitchTo:
		state.SaveSelection(p.PaneID)
		if w.Source == core.SourceCursor {
			if err := tmux.SwitchToSession(*w); err != nil {
				m.flashMessage = "Failed to open Cursor: " + err.Error()
				m.flashIsError = true
				m.flashTicks = 0
				m.mode = core.ModeSessionList
				return m, nil
			}
			m.flashMessage = "Opened " + w.SessionName + " in Cursor"
			m.flashIsError = false
			m.flashTicks = 0
			m.mode = core.ModeSessionList
			return m, nil
		}
		tmux.SwitchToTarget(w.SessionName, w.WindowIndex, p.PaneID)
		m.quitting = true
		return m, tea.Quit

	case core.ActionSendInput:
		m.textInput.SetValue("")
		m.textInput.Placeholder = "message to send..."
		m.textInput.Focus()
		m.mode = core.ModeSendInput

	case core.ActionRename:
		m.textInput.SetValue(w.SessionName)
		m.textInput.Placeholder = "new name"
		m.textInput.Focus()
		m.mode = core.ModeRename

	case core.ActionStageAll:
		m.mode = core.ModeSessionList
		return m, git.StageAll(p.WorkingDir)

	case core.ActionCommit:
		m.textInput.SetValue("")
		m.textInput.Placeholder = "commit message"
		m.textInput.Focus()
		m.mode = core.ModeCommit

	case core.ActionPush:
		m.mode = core.ModeSessionList
		return m, git.Push(p.WorkingDir)

	case core.ActionFetch:
		m.mode = core.ModeSessionList
		return m, git.Fetch(p.WorkingDir)

	case core.ActionKillSession:
		m.confirmMsg = fmt.Sprintf("Kill session %q?", w.SessionName)
		m.confirmAction = func() tea.Cmd {
			_ = tmux.KillSession(w.SessionName) // Best effort
			return refreshWindowsCmd()
		}
		m.confirmReturnMode = core.ModeSessionList
		m.mode = core.ModeConfirm

	case core.ActionKillAndDeleteWorktree:
		m.confirmMsg = fmt.Sprintf("Kill session %q and delete worktree?", w.SessionName)
		m.confirmAction = func() tea.Cmd {
			_ = tmux.KillSession(w.SessionName) // Best effort
			return tea.Batch(
				git.WorktreeRemove(p.WorkingDir, p.WorkingDir),
				refreshWindowsCmd(),
			)
		}
		m.confirmReturnMode = core.ModeSessionList
		m.mode = core.ModeConfirm

	case core.ActionNewWorktree:
		m.textInput.SetValue("")
		m.textInput.Placeholder = "branch-name"
		m.textInput.Focus()
		m.mode = core.ModeNewWorktree

	case core.ActionOpenInTerminal:
		workspacePath := w.WorkspacePath
		if workspacePath == "" {
			workspacePath = p.WorkingDir
		}
		sessionName := w.SessionName
		if err := tmux.OpenInTerminal(sessionName, workspacePath); err != nil {
			m.flashMessage = "Failed to open in terminal: " + err.Error()
			m.flashIsError = true
		} else {
			m.flashMessage = fmt.Sprintf("Created tmux session %s with Claude", sessionName)
			m.flashIsError = false
		}
		m.flashTicks = 0
		m.mode = core.ModeSessionList
		return m, refreshWindowsCmd()

	case core.ActionCopyPath:
		workspacePath := w.WorkspacePath
		if workspacePath == "" {
			workspacePath = p.WorkingDir
		}
		m.flashMessage = "Copied: " + workspacePath
		m.flashIsError = false
		m.flashTicks = 0
		m.mode = core.ModeSessionList
		return m, tea.SetClipboard(workspacePath)

	case core.ActionEndSession:
		m.confirmMsg = fmt.Sprintf("End tracking for %q? (Cursor will keep running)", w.SessionName)
		m.confirmAction = func() tea.Cmd {
			if w.ConversationID != "" {
				_ = removeCursorSession(w.ConversationID) // Best effort
			}
			return refreshWindowsCmd()
		}
		m.confirmReturnMode = core.ModeSessionList
		m.mode = core.ModeConfirm
	}

	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterText = ""
		m.filterPending = ""
		m.filterDebounced = false
		m.filtered = nil
		m.applySort()
		m.rebuildItems()
		m.skipSectionHeaders(1)
		m.mode = core.ModeSessionList
		return m, nil

	case "enter":
		m.filterText = m.textInput.Value()
		m.filterPending = ""
		m.filterDebounced = false
		m.applyFilter()
		m.mode = core.ModeSessionList
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	newFilterText := m.textInput.Value()
	m.filterText = newFilterText
	m.filterPending = newFilterText

	if m.filterDebounced {
		return m, cmd
	}

	m.filterDebounced = true
	return m, tea.Batch(cmd, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return core.FilterDebounceMsg{FilterText: newFilterText}
	}))
}

func (m Model) handleSendInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = core.ModeSessionList
		return m, nil

	case "enter":
		text := m.textInput.Value()
		if text != "" && m.actionWindow != nil {
			if p := m.actionWindow.PrimaryPane(); p != nil {
				_ = tmux.SendKeys(p.PaneID, text) // Best effort
				m.flashMessage = fmt.Sprintf("Sent to %s", m.actionWindow.SessionName)
				m.flashIsError = false
				m.flashTicks = 0
			}
		}
		m.mode = core.ModeSessionList
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmKey(key string) (tea.Model, tea.Cmd) {
	returnMode := m.confirmReturnMode
	if returnMode == core.ModeConfirm {
		returnMode = core.ModeSessionList
	}
	switch key {
	case "y", "enter":
		m.mode = returnMode
		if m.confirmAction != nil {
			return m, m.confirmAction()
		}
	case "n", "esc":
		m.mode = returnMode
	}
	return m, nil
}

func (m Model) handleRenameKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = core.ModeSessionList
		return m, nil

	case "enter":
		newName := m.textInput.Value()
		if newName != "" && m.actionWindow != nil {
			_ = tmux.RenameSession(m.actionWindow.SessionName, newName) // Best effort
			m.flashMessage = fmt.Sprintf("Renamed to %s", newName)
			m.flashIsError = false
			m.flashTicks = 0
		}
		m.mode = core.ModeSessionList
		return m, refreshWindowsCmd()
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleNewSessionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = core.ModeSessionList
		return m, nil

	case "enter":
		name := m.textInput.Value()
		if name != "" {
			cwd := "."
			if home := state.CachedHomeDir(); home != "" {
				cwd = home
			}
			if err := tmux.StartClaudeInSession(name, cwd); err != nil {
				m.flashMessage = "Failed to create session: " + err.Error()
				m.flashIsError = true
				m.flashTicks = 0
			} else {
				m.flashMessage = fmt.Sprintf("Created session %s with Claude", name)
				m.flashIsError = false
				m.flashTicks = 0
			}
		}
		m.mode = core.ModeSessionList
		return m, refreshWindowsCmd()
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleCommitKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = core.ModeSessionList
		return m, nil

	case "enter":
		message := m.textInput.Value()
		if message != "" && m.actionWindow != nil {
			if p := m.actionWindow.PrimaryPane(); p != nil {
				m.mode = core.ModeSessionList
				return m, git.Commit(p.WorkingDir, message)
			}
		}
		m.mode = core.ModeSessionList
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}

func (m Model) handleNewWorktreeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = core.ModeSessionList
		return m, nil

	case "enter":
		branchName := m.textInput.Value()
		if branchName != "" && m.actionWindow != nil {
			if p := m.actionWindow.PrimaryPane(); p != nil {
				worktreePath := p.WorkingDir + "-" + branchName
				m.mode = core.ModeSessionList
				return m, git.WorktreeAdd(p.WorkingDir, worktreePath, branchName)
			}
		}
		m.mode = core.ModeSessionList
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	return m, cmd
}
