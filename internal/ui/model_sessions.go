package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
	"github.com/inquire/tmux-overseer/internal/tmux"
)

func (m Model) handleSessionListKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		m.quitting = true
		return m, tea.Quit

	case "j", "down":
		m.scroll.MoveDown()
		m.skipSectionHeaders(1)
		m.saveSelection()
		return m, m.schedulePreview()

	case "k", "up":
		m.scroll.MoveUp()
		m.skipSectionHeaders(-1)
		m.saveSelection()
		return m, m.schedulePreview()

	case "l", "right":
		if w := m.selectedItemWindow(); w != nil {
			m.actionWindow = w
			m.actions = m.actionsForWindow(w)
			m.actionIdx = 0
			m.mode = core.ModeActionMenu
		}

	case "enter":
		// On a collapsed group or team: expand it.
		if item := m.selectedItem(); item != nil {
			if item.IsGroupHeader && !m.expandedGroups[item.GroupKey] {
				m.expandedGroups[item.GroupKey] = true
				m.rebuildItems()
				m.skipSectionHeaders(1)
				return m, nil
			}
			if item.IsTeamHeader && !m.expandedTeams[item.TeamKey] {
				m.expandedTeams[item.TeamKey] = true
				m.rebuildItems()
				m.skipSectionHeaders(1)
				return m, nil
			}
		}
		w, p := m.selectedWindowAndPane()
		if w != nil && p != nil {
			state.SaveSelection(p.PaneID)
			if w.Source == core.SourceCursor {
				if err := tmux.SwitchToSession(*w); err != nil {
					m.flashMessage = "Failed to open Cursor: " + err.Error()
					m.flashIsError = true
					m.flashTicks = 0
					return m, nil
				}
				m.flashMessage = "Opened " + w.SessionName + " in Cursor"
				m.flashIsError = false
				m.flashTicks = 0
				return m, nil
			}
			tmux.SwitchToTarget(w.SessionName, w.WindowIndex, p.PaneID)
			m.quitting = true
			return m, tea.Quit
		}

	case "tab":
		m.toggleExpand()
		m.rebuildItems()
		m.skipSectionHeaders(1)

	case "n":
		m.textInput.SetValue("")
		m.textInput.Placeholder = "session-name"
		m.textInput.Focus()
		m.mode = core.ModeNewSession

	case "i":
		if w := m.selectedItemWindow(); w != nil {
			m.actionWindow = w
			m.textInput.SetValue("")
			m.textInput.Placeholder = "message to send..."
			m.textInput.Focus()
			m.mode = core.ModeSendInput
		}

	case "d":
		if w := m.selectedItemWindow(); w != nil {
			m.actionWindow = w
			m.confirmMsg = fmt.Sprintf("Kill session %q? This will terminate all Claude instances.", w.SessionName)
			m.confirmAction = func() tea.Cmd {
				_ = tmux.KillSession(w.SessionName) // Best effort
				return refreshWindowsCmd()
			}
			m.confirmReturnMode = core.ModeSessionList
			m.mode = core.ModeConfirm
		}

	case "s":
		m.sortMode = m.sortMode.Next()
		m.applySort()
		m.rebuildItems()
		m.skipSectionHeaders(1)

	case "/":
		m.textInput.SetValue("")
		m.textInput.Placeholder = "filter..."
		m.textInput.Focus()
		m.mode = core.ModeFilter

	case "R":
		return m, refreshWindowsCmd()

	case "f":
		m.sourceFilter = m.sourceFilter.Next()
		m.applyFilter()
		return m, nil

	case "g":
		if m.groupMode == core.GroupBySource {
			m.groupMode = core.GroupByWorkspace
		} else {
			m.groupMode = core.GroupBySource
		}
		m.rebuildItems()
		m.skipSectionHeaders(1)

	case "[":
		// Shrink preview pane (minimum 4 lines)
		if m.previewHeight > 4 {
			m.previewHeight--
			m.previewOffset = 0
			listH := state.MaxInt(3, m.height-m.previewHeight-9)
			m.scroll.SetViewHeight(listH / 2)
		}

	case "]":
		// Grow preview pane (maximum 20 lines)
		if m.previewHeight < 20 {
			m.previewHeight++
			m.previewOffset = 0
			listH := state.MaxInt(3, m.height-m.previewHeight-9)
			m.scroll.SetViewHeight(listH / 2)
		}

	case "J":
		// Scroll preview down
		m.previewOffset++

	case "K":
		// Scroll preview up
		if m.previewOffset > 0 {
			m.previewOffset--
		}
	}

	return m, nil
}

// rebuildItems builds the flat selectable item list from windows + expanded panes.
// Sessions with active incomplete plans are pinned at the top under a section header.
// Cursor sessions with the same WorkspacePath are grouped under a collapsible header.
// CLI sessions and singleton Cursor sessions render ungrouped.
func (m *Model) rebuildItems() {
	m.items = m.items[:0]

	indices := m.filtered
	if indices == nil {
		indices = make([]int, len(m.windows))
		for i := range m.windows {
			indices[i] = i
		}
	}

	// Build per-source workspace groups. Cursor sessions only group with other
	// Cursor sessions; CLI sessions only group with other CLI sessions.
	// This prevents cross-source mixing inside a single group header.
	workspaceKey := func(w *core.ClaudeWindow) string {
		if w.OriginalRepo != "" {
			return w.OriginalRepo
		}
		if w.WorkspacePath != "" {
			return w.WorkspacePath
		}
		if p := w.PrimaryPane(); p != nil {
			return p.WorkingDir
		}
		return ""
	}

	cursorByPath := make(map[string][]int) // Cursor workspace → session indices
	cliByPath := make(map[string][]int)   // CLI workspace → session indices
	for _, i := range indices {
		if i >= len(m.windows) {
			continue
		}
		w := &m.windows[i]
		key := workspaceKey(w)
		if key == "" {
			continue
		}
		switch w.Source {
		case core.SourceCursor:
			cursorByPath[key] = append(cursorByPath[key], i)
		case core.SourceCLI:
			cliByPath[key] = append(cliByPath[key], i)
		}
	}

	grouped := make(map[int]bool)
	for _, idxs := range cursorByPath {
		if len(idxs) >= 2 {
			for _, i := range idxs {
				grouped[i] = true
			}
		}
	}
	for _, idxs := range cliByPath {
		if len(idxs) >= 2 {
			for _, i := range idxs {
				grouped[i] = true
			}
		}
	}

	// Unified lookup: workspace key → indices (for the group header renderer).
	allByPath := make(map[string][]int, len(cursorByPath)+len(cliByPath))
	for k, v := range cursorByPath {
		allByPath[k] = v
	}
	for k, v := range cliByPath {
		allByPath[k] = append(allByPath[k], v...)
	}

	// Partition sessions by TeamName for Agent Teams grouping.
	teamByName := make(map[string][]int)
	teamNameOrder := []string{}
	for _, i := range indices {
		if i >= len(m.windows) {
			continue
		}
		w := &m.windows[i]
		if w.TeamName != "" {
			if _, exists := teamByName[w.TeamName]; !exists {
				teamNameOrder = append(teamNameOrder, w.TeamName)
			}
			teamByName[w.TeamName] = append(teamByName[w.TeamName], i)
		}
	}
	_ = teamNameOrder // used in emitItems below
	inTeam := make(map[int]bool)
	for _, idxs := range teamByName {
		for _, i := range idxs {
			inTeam[i] = true
		}
	}

	showHeader := m.sourceFilter == core.FilterCLI || m.sourceFilter == core.FilterCursor
	if showHeader && len(indices) > 0 {
		label := fmt.Sprintf("%s SESSIONS (%d)", strings.ToUpper(m.sourceFilter.Label()), len(indices))
		m.items = append(m.items, core.ListItem{
			WindowIdx:       -1,
			PaneIdx:         -1,
			IsSectionHeader: true,
			SectionLabel:    label,
		})
	}

	// Split indices into active-plan vs rest for section headers.
	var activePlanIndices, restIndices []int
	for _, i := range indices {
		if i < len(m.windows) && m.windows[i].HasActivePlan() {
			activePlanIndices = append(activePlanIndices, i)
		} else {
			restIndices = append(restIndices, i)
		}
	}

	emittedGroups := make(map[string]bool)
	emittedTeams := make(map[string]bool)
	emitItems := func(idxSet []int) {
		for _, i := range idxSet {
			if i >= len(m.windows) {
				continue
			}
			w := &m.windows[i]

			// Agent Team group header.
			if inTeam[i] && !grouped[i] {
				teamName := w.TeamName
				if emittedTeams[teamName] {
					continue
				}
				emittedTeams[teamName] = true
				idxs := teamByName[teamName]

				if _, exists := m.expandedTeams[teamName]; !exists {
					m.expandedTeams[teamName] = true
				}

				m.items = append(m.items, core.ListItem{
					WindowIdx:        -1,
					PaneIdx:          -1,
					IsTeamHeader:     true,
					TeamKey:          teamName,
					TeamWindowIndices: idxs,
				})

				if m.expandedTeams[teamName] {
					for _, ci := range idxs {
						m.appendWindowItems(ci, false)
					}
				}
				continue
			}

			if grouped[i] {
				path := workspaceKey(w)
				if emittedGroups[path] {
					continue
				}
				emittedGroups[path] = true
				idxs := allByPath[path]

				if _, exists := m.expandedGroups[path]; !exists {
					m.expandedGroups[path] = true
				}

				m.items = append(m.items, core.ListItem{
					WindowIdx:          -1,
					PaneIdx:            -1,
					IsGroupHeader:      true,
					GroupKey:           path,
					GroupWindowIndices: idxs,
				})

				if m.expandedGroups[path] {
					for _, ci := range idxs {
						m.appendWindowItems(ci, true)
					}
				}
				continue
			}

			m.appendWindowItems(i, false)
		}
	}

	if len(activePlanIndices) > 0 {
		m.items = append(m.items, core.ListItem{
			WindowIdx:       -1,
			PaneIdx:         -1,
			IsSectionHeader: true,
			SectionLabel:    fmt.Sprintf("◆ ACTIVE PLANS (%d)", len(activePlanIndices)),
		})
		emitItems(activePlanIndices)
	}

	spacer := core.ListItem{WindowIdx: -1, PaneIdx: -1, IsSpacer: true}

	// GroupByWorkspace: all sessions under workspace headers regardless of source.
	if m.groupMode == core.GroupByWorkspace {
		// Build a cross-source workspace map for this view.
		wsAll := make(map[string][]int)
		var wsOrder []string
		for _, i := range restIndices {
			if i >= len(m.windows) {
				continue
			}
			key := workspaceKey(&m.windows[i])
			if key == "" {
				key = m.windows[i].DisplayName()
			}
			if _, exists := wsAll[key]; !exists {
				wsOrder = append(wsOrder, key)
			}
			wsAll[key] = append(wsAll[key], i)
		}
		emittedWS := make(map[string]bool)
		// Re-use emitItems but override grouping for this mode.
		for _, key := range wsOrder {
			if emittedWS[key] {
				continue
			}
			emittedWS[key] = true
			idxs := wsAll[key]
			if _, exists := m.expandedGroups[key]; !exists {
				m.expandedGroups[key] = true
			}
			m.items = append(m.items, spacer)
			m.items = append(m.items, core.ListItem{
				WindowIdx:          -1,
				PaneIdx:            -1,
				IsGroupHeader:      true,
				GroupKey:           key,
				GroupWindowIndices: idxs,
			})
			if m.expandedGroups[key] {
				for _, ci := range idxs {
					m.appendWindowItems(ci, true)
				}
			}
		}
	} else if m.sourceFilter == core.FilterAll {
		// GroupBySource (default): CURSOR section then CLAUDE CODE section.
		var cursorIndices, cliIndices []int
		for _, i := range restIndices {
			if i < len(m.windows) && m.windows[i].Source == core.SourceCursor {
				cursorIndices = append(cursorIndices, i)
			} else {
				cliIndices = append(cliIndices, i)
			}
		}

		if len(cursorIndices) > 0 {
			m.items = append(m.items, spacer)
			m.items = append(m.items, core.ListItem{
				WindowIdx:       -1,
				PaneIdx:         -1,
				IsSectionHeader: true,
				SectionLabel:    fmt.Sprintf("CURSOR (%d)", len(cursorIndices)),
			})
			emitItems(cursorIndices)
		}

		if len(cliIndices) > 0 {
			m.items = append(m.items, spacer)
			m.items = append(m.items, core.ListItem{
				WindowIdx:       -1,
				PaneIdx:         -1,
				IsSectionHeader: true,
				SectionLabel:    fmt.Sprintf("CLAUDE CODE (%d)", len(cliIndices)),
			})
			emitItems(cliIndices)
		}
	} else {
		emitItems(restIndices)
	}

	m.scroll.SetTotal(len(m.items))
}

// appendWindowItems adds a window (and its expanded panes) to the items list.
func (m *Model) appendWindowItems(i int, inGroup bool) {
	w := m.windows[i]
	m.items = append(m.items, core.ListItem{WindowIdx: i, PaneIdx: -1, InGroup: inGroup})
	key := fmt.Sprintf("%s:%d", w.SessionName, w.WindowIndex)
	if m.expandedWindows[key] && len(w.Panes) > 1 {
		for j := range w.Panes {
			m.items = append(m.items, core.ListItem{WindowIdx: i, PaneIdx: j, InGroup: inGroup})
		}
	}
}

// skipSectionHeaders advances the selection past any section header items.
// direction should be 1 (down) or -1 (up).
func (m *Model) skipSectionHeaders(direction int) {
	isSkippable := func(i int) bool {
		return m.items[i].IsSectionHeader || m.items[i].IsSpacer
	}
	for m.scroll.Selected >= 0 && m.scroll.Selected < len(m.items) &&
		isSkippable(m.scroll.Selected) {
		if direction > 0 {
			m.scroll.MoveDown()
		} else {
			m.scroll.MoveUp()
		}
		if m.scroll.Selected <= 0 || m.scroll.Selected >= len(m.items)-1 {
			if m.scroll.Selected >= 0 && m.scroll.Selected < len(m.items) &&
				isSkippable(m.scroll.Selected) {
				if direction > 0 {
					m.scroll.MoveDown()
				} else {
					m.scroll.MoveUp()
				}
			}
			break
		}
	}
}

// selectedItem returns the currently selected core.ListItem, or nil.
// Group headers (IsGroupHeader) ARE selectable and are returned.
func (m *Model) selectedItem() *core.ListItem {
	if m.scroll.Selected < 0 || m.scroll.Selected >= len(m.items) {
		return nil
	}
	item := &m.items[m.scroll.Selected]
	if item.IsSectionHeader || item.IsSpacer {
		return nil
	}
	return item
}

// selectedItemWindow returns the core.ClaudeWindow for the currently selected item.
// For group headers, returns the first window in the group.
func (m *Model) selectedItemWindow() *core.ClaudeWindow {
	item := m.selectedItem()
	if item == nil {
		return nil
	}
	if item.IsGroupHeader {
		if len(item.GroupWindowIndices) > 0 {
			idx := item.GroupWindowIndices[0]
			if idx >= 0 && idx < len(m.windows) {
				return &m.windows[idx]
			}
		}
		return nil
	}
	if item.IsTeamHeader {
		if len(item.TeamWindowIndices) > 0 {
			idx := item.TeamWindowIndices[0]
			if idx >= 0 && idx < len(m.windows) {
				return &m.windows[idx]
			}
		}
		return nil
	}
	if item.WindowIdx < 0 || item.WindowIdx >= len(m.windows) {
		return nil
	}
	return &m.windows[item.WindowIdx]
}

// selectedWindowAndPane resolves the window and specific pane for the current selection.
// For group headers, returns the first window's primary pane.
// For pane rows, returns that specific pane.
func (m *Model) selectedWindowAndPane() (*core.ClaudeWindow, *core.ClaudePane) {
	item := m.selectedItem()
	if item == nil {
		return nil, nil
	}
	w := m.selectedItemWindow()
	if w == nil {
		return nil, nil
	}
	if item.IsGroupHeader || item.IsTeamHeader {
		if p := w.PrimaryPane(); p != nil {
			return w, p
		}
		return w, nil
	}
	if item.IsPane() {
		if item.PaneIdx < len(w.Panes) {
			return w, &w.Panes[item.PaneIdx]
		}
		return w, nil
	}
	if p := w.PrimaryPane(); p != nil {
		return w, p
	}
	return w, nil
}

// saveSelection stores the currently selected pane ID for restoration after refresh.
func (m *Model) saveSelection() {
	_, p := m.selectedWindowAndPane()
	if p != nil {
		m.lastSelectedID = p.PaneID
	}
}

// restoreSelection attempts to re-select the previously selected pane after refresh.
func (m *Model) restoreSelection() {
	if m.lastSelectedID == "" {
		return
	}
	for i, item := range m.items {
		if item.WindowIdx < 0 || item.WindowIdx >= len(m.windows) {
			continue
		}
		w := &m.windows[item.WindowIdx]
		var paneID string
		if item.IsPane() {
			if item.PaneIdx < len(w.Panes) {
				paneID = w.Panes[item.PaneIdx].PaneID
			}
		} else {
			if p := w.PrimaryPane(); p != nil {
				paneID = p.PaneID
			}
		}
		if paneID == m.lastSelectedID {
			m.scroll.SetSelected(i)
			return
		}
	}
}

func (m *Model) toggleExpand() {
	item := m.selectedItem()
	if item == nil {
		return
	}

	if item.IsGroupHeader {
		if m.expandedGroups[item.GroupKey] {
			delete(m.expandedGroups, item.GroupKey)
		} else {
			m.expandedGroups[item.GroupKey] = true
		}
		return
	}

	if item.IsTeamHeader {
		if m.expandedTeams[item.TeamKey] {
			delete(m.expandedTeams, item.TeamKey)
		} else {
			m.expandedTeams[item.TeamKey] = true
		}
		return
	}

	winIdx := item.WindowIdx
	if winIdx < 0 || winIdx >= len(m.windows) {
		return
	}
	w := &m.windows[winIdx]

	// Multi-pane windows: expand/collapse panes
	if len(w.Panes) > 1 {
		key := fmt.Sprintf("%s:%d", w.SessionName, w.WindowIndex)
		if m.expandedWindows[key] {
			delete(m.expandedWindows, key)
		} else {
			m.expandedWindows[key] = true
		}
		return
	}

	key := w.ConversationID
	if key == "" {
		key = fmt.Sprintf("%s:%d", w.SessionName, w.WindowIndex)
	}

	hasPlanTodos := len(w.ActivePlanTodos) > 0
	hasSubagents := len(w.Subagents) > 0

	// If session has both plan todos and subagents, cycle through states:
	//   collapsed -> plan todos -> subagents -> collapsed
	if hasPlanTodos && hasSubagents {
		planExp := m.expandedPlans[key]
		subExp := m.expandedSubagents[key]
		switch {
		case !planExp && !subExp:
			m.expandedPlans[key] = true
		case planExp && !subExp:
			delete(m.expandedPlans, key)
			m.expandedSubagents[key] = true
		default:
			delete(m.expandedPlans, key)
			delete(m.expandedSubagents, key)
		}
		return
	}

	if hasPlanTodos {
		if m.expandedPlans[key] {
			delete(m.expandedPlans, key)
		} else {
			m.expandedPlans[key] = true
		}
		return
	}

	if hasSubagents {
		if m.expandedSubagents[key] {
			delete(m.expandedSubagents, key)
		} else {
			m.expandedSubagents[key] = true
		}
	}
}

func (m *Model) applySort() {
	sort.SliceStable(m.windows, func(i, j int) bool {
		wi, wj := &m.windows[i], &m.windows[j]

		// Tier 0: attached session always first
		if wi.Attached != wj.Attached {
			return wi.Attached
		}

		// Tier 1: active incomplete plans float above non-plan sessions
		api, apj := wi.HasActivePlan(), wj.HasActivePlan()
		if api != apj {
			return api
		}
		// Within the active-plan tier, sort by completion % ascending
		// (least complete first — needs the most attention)
		if api && apj {
			ci, cj := wi.PlanCompletionPct(), wj.PlanCompletionPct()
			if ci != cj {
				return ci < cj
			}
		}

		// Tier 2: normal sort by current mode
		switch m.sortMode {
		case core.SortByName:
			return wi.DisplayName() < wj.DisplayName()
		case core.SortByStatus:
			pi := wi.PrimaryPane()
			pj := wj.PrimaryPane()
			if pi == nil || pj == nil {
				return pi != nil
			}
			return pi.Status < pj.Status
		case core.SortByRecency:
			return wi.CreatedAt > wj.CreatedAt
		}
		return false
	})
}

// applyFilter filters windows based on filterText and sourceFilter.
// Uses precomputed lowercase strings to avoid repeated allocations.
func (m *Model) applyFilter() {
	if m.filterText == "" && m.sourceFilter == core.FilterAll {
		m.filtered = nil
		m.rebuildItems()
		m.skipSectionHeaders(1)
		return
	}

	filterLower := strings.ToLower(m.filterText)
	m.filtered = nil

	for i := range m.windows {
		w := &m.windows[i]

		if !m.sourceFilter.Matches(w.Source) {
			continue
		}

		if m.filterText != "" && !strings.Contains(w.SearchText(), filterLower) {
			continue
		}

		m.filtered = append(m.filtered, i)
	}

	m.rebuildItems()
	m.skipSectionHeaders(1)
}

func (m *Model) actionsForWindow(w *core.ClaudeWindow) []core.SessionAction {
	if w.Source == core.SourceCursor {
		actions := []core.SessionAction{core.ActionSwitchTo}

		p := w.PrimaryPane()
		if p != nil && p.HasGit {
			actions = append(actions,
				core.ActionStageAll,
				core.ActionCommit,
				core.ActionPush,
				core.ActionFetch,
			)
		}

		actions = append(actions,
			core.ActionOpenInTerminal,
			core.ActionCopyPath,
			core.ActionEndSession,
		)

		return actions
	}

	actions := []core.SessionAction{
		core.ActionSwitchTo,
		core.ActionSendInput,
		core.ActionRename,
		core.ActionStageAll,
		core.ActionCommit,
		core.ActionPush,
		core.ActionFetch,
	}

	p := w.PrimaryPane()
	if p != nil && p.IsWorktree {
		actions = append(actions, core.ActionNewWorktree, core.ActionKillAndDeleteWorktree)
	} else {
		actions = append(actions, core.ActionNewWorktree, core.ActionKillSession)
	}

	return actions
}
