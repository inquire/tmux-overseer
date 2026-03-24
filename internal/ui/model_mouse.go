package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
)

// getLayoutOffsets returns the line positions for each layout section.
func (m Model) getLayoutOffsets() (headerLines, sessionListStart, previewStart, statusBarLine, footerLine int) {
	headerLines = 3
	sessionListStart = headerLines
	listHeightInLines := state.MaxInt(3, m.height-19)
	previewStart = sessionListStart + listHeightInLines
	statusBarLine = previewStart + 12 // preview is 12 lines (1 sep + 10 content + 1 sep)
	footerLine = statusBarLine + 1
	return
}

// handleMouseClick processes mouse click events and updates the model accordingly.
func (m Model) handleMouseClick(y, x int) (tea.Model, tea.Cmd) {
	headerLines, sessionListStart, previewStart, statusBarLine, _ := m.getLayoutOffsets()

	if y < headerLines {
		return m, nil
	}

	// Session list area (only when in session list mode)
	if m.mode == core.ModeSessionList && y >= sessionListStart && y < previewStart {
		relativeY := y - sessionListStart
		start, _ := m.scroll.VisibleRange()

		currentLine := 0
		for i := start; i < len(m.items); i++ {
			item := m.items[i]

			var itemLines int
			if item.IsSectionHeader {
				itemLines = 2
			} else if item.IsPane() {
				itemLines = 1
			} else {
				itemLines = 2
			}

			if relativeY >= currentLine && relativeY < currentLine+itemLines {
				if !item.IsSectionHeader {
					now := time.Now()
					if i == m.lastClickItem && now.Sub(m.lastClickTime) < 300*time.Millisecond {
						// Double-click: switch to session
						m.scroll.SetSelected(i)
						m.skipSectionHeaders(1)
						m.saveSelection()
						if w := m.selectedItemWindow(); w != nil {
							return m, m.switchToWindow(w)
						}
					} else {
						// Single click: select item
						m.scroll.SetSelected(i)
						m.skipSectionHeaders(1)
						m.saveSelection()
						m.lastClickTime = now
						m.lastClickItem = i
						return m, m.schedulePreview()
					}
				}
				break
			}
			currentLine += itemLines

			if currentLine >= relativeY+itemLines {
				break
			}
		}
		return m, nil
	}

	// Action menu area (when in ModeActionMenu)
	if m.mode == core.ModeActionMenu && y >= sessionListStart && y < previewStart {
		relativeY := y - sessionListStart
		metadataEstimate := 8
		if relativeY > metadataEstimate {
			actionLineY := relativeY - metadataEstimate - 1
			if actionLineY >= 0 && actionLineY < len(m.actions) {
				m.actionIdx = actionLineY
				if m.actionIdx >= len(m.actions) {
					m.actionIdx = len(m.actions) - 1
				}
				if m.actionIdx < len(m.actions) {
					return m.executeAction(m.actions[m.actionIdx])
				}
			}
		}
		return m, nil
	}

	// Plans view: click on plan items
	if m.mode == core.ModePlans && y >= 5 {
		planListStartY := 5
		linesPerItem := 3

		visibleLines := m.height - 10
		if visibleLines < 3 {
			visibleLines = 3
		}
		visibleItems := visibleLines / linesPerItem
		if visibleItems < 1 {
			visibleItems = 1
		}

		selected := m.planScroll.Selected
		total := len(m.planItems)
		startIdx := selected - visibleItems/2
		if startIdx < 0 {
			startIdx = 0
		}
		if startIdx+visibleItems > total {
			startIdx = total - visibleItems
		}
		if startIdx < 0 {
			startIdx = 0
		}

		relativeY := y - planListStartY
		if startIdx > 0 {
			relativeY--
		}
		if relativeY < 0 {
			return m, nil
		}

		clickedIdx := startIdx + relativeY/linesPerItem
		if clickedIdx >= 0 && clickedIdx < total {
			now := time.Now()
			if clickedIdx == m.lastClickItem && now.Sub(m.lastClickTime) < 300*time.Millisecond {
				// Double-click: resume plan
				m.planScroll.SetSelected(clickedIdx)
				return m.resumePlan()
			}
			m.planScroll.SetSelected(clickedIdx)
			m.lastClickTime = now
			m.lastClickItem = clickedIdx
		}
		return m, nil
	}

	// Status bar area: click cycles sort mode
	if y == statusBarLine && x > m.width/2 {
		m.sortMode = m.sortMode.Next()
		m.applySort()
		m.applyFilter()
		m.restoreSelection()
		m.skipSectionHeaders(1)
		return m, nil
	}

	return m, nil
}
