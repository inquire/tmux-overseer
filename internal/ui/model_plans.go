package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/db"
	"github.com/inquire/tmux-overseer/internal/exec"
	"github.com/inquire/tmux-overseer/internal/plans"
	"github.com/inquire/tmux-overseer/internal/state"
	"github.com/inquire/tmux-overseer/internal/tmux"
)

// planItemKind enumerates what a given flat index in the plans view refers to.
type planItemKind int

const (
	planItemGroup planItemKind = iota
	planItemPlan
)

// planItemRef resolves the flat scroll index to either a group header or a plan entry.
type planItemRef struct {
	Kind           planItemKind
	Group          *core.PlanGroup // non-nil when Kind == planItemGroup
	Plan           *core.PlanEntry // non-nil when Kind == planItemPlan
	GroupIdx       int             // index into m.planGroups
	PlanIdxInGroup int             // index within the group's Plans slice
}

// plansCacheTTL controls how long the plans disk cache is considered fresh.
// If the cache is older than this, a background rescan is triggered automatically.
const plansCacheTTL = 60 * time.Second

// loadPlansCmd asynchronously loads plans.
// It returns cached plans immediately if the cache is still fresh (< plansCacheTTL),
// otherwise it performs a full scan and saves the result to disk.
func loadPlansCmd() tea.Cmd {
	return func() tea.Msg {
		if cached := state.LoadPlansCache(); cached != nil && len(cached.Plans) > 0 {
			if time.Since(cached.SavedAt) < plansCacheTTL {
				var entries []core.PlanEntry
				if err := json.Unmarshal(cached.Plans, &entries); err == nil {
					return core.PlansMsg{Plans: entries, FromCache: true}
				}
			}
		}

		entries := plans.ScanAll(50)

		if raw, err := json.Marshal(entries); err == nil {
			state.SavePlansCache(raw)
		}

		// Background sync to DuckDB (best effort, no progress needed)
		_, _ = db.SyncPlans(entries, nil)

		return core.PlansMsg{Plans: entries}
	}
}

// reloadPlansCmd performs an unconditional rescan (ignores the disk cache).
// Used when the user explicitly presses R in the plans view.
func reloadPlansCmd() tea.Cmd {
	return func() tea.Msg {
		entries := plans.ScanAll(50)
		if raw, err := json.Marshal(entries); err == nil {
			state.SavePlansCache(raw)
		}
		_, _ = db.SyncPlans(entries, nil)
		return core.PlansMsg{Plans: entries}
	}
}

// planVisibleItemCount returns the total number of selectable rows in the plans view
// (group headers + plan rows for expanded groups + ungrouped plan rows).
func (m *Model) planVisibleItemCount() int {
	count := 0
	for _, g := range m.planGroups {
		if g.WorkspacePath == "" {
			count += len(g.Plans)
		} else {
			count++ // group header
			if m.expandedPlanGroups[g.WorkspacePath] {
				count += len(g.Plans)
			}
		}
	}
	return count
}

// planItemAt resolves the flat scroll index to a planItemRef.
func (m *Model) planItemAt(idx int) (planItemRef, bool) {
	cur := 0
	for gi := range m.planGroups {
		g := &m.planGroups[gi]
		if g.WorkspacePath == "" {
			for pi := range g.Plans {
				if cur == idx {
					return planItemRef{Kind: planItemPlan, Plan: &g.Plans[pi], Group: g, GroupIdx: gi, PlanIdxInGroup: pi}, true
				}
				cur++
			}
		} else {
			if cur == idx {
				return planItemRef{Kind: planItemGroup, Group: g, GroupIdx: gi}, true
			}
			cur++
			if m.expandedPlanGroups[g.WorkspacePath] {
				for pi := range g.Plans {
					if cur == idx {
						return planItemRef{Kind: planItemPlan, Plan: &g.Plans[pi], Group: g, GroupIdx: gi, PlanIdxInGroup: pi}, true
					}
					cur++
				}
			}
		}
	}
	return planItemRef{}, false
}

// filterPlans applies source filter, completed filter, and text filter to allPlans.
// It also builds workspace groups for grouped rendering.
// Clears multi-selection since flat indices are invalidated by re-filtering.
func (m *Model) filterPlans() {
	m.planMultiSelected = make(map[int]bool)
	m.planSelectAnchor = -1
	needle := strings.ToLower(m.planFilterText)

	// Discover all unique tags across all plans
	tagSet := make(map[string]bool)
	for _, p := range m.allPlans {
		for _, tag := range p.Tags {
			tagSet[tag] = true
		}
	}
	m.planDiscoveredTags = nil
	for tag := range tagSet {
		m.planDiscoveredTags = append(m.planDiscoveredTags, tag)
	}
	sort.Strings(m.planDiscoveredTags)

	var filtered []core.PlanEntry
	for _, p := range m.allPlans {
		if !m.planSourceFilter.Matches(p.Source) {
			continue
		}
		if !m.planShowCompleted && p.IsCompleted() {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(p.Title), needle) &&
			!strings.Contains(strings.ToLower(p.Overview), needle) &&
			!strings.Contains(strings.ToLower(p.WorkspacePath), needle) {
			continue
		}
		// Tag filter (S3)
		if m.planTagFilter != "" {
			hasTag := false
			for _, tag := range p.Tags {
				if tag == m.planTagFilter {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}
		filtered = append(filtered, p)
	}
	m.planItems = filtered

	switch m.planGroupMode {
	case core.PlanGroupByDay:
		m.buildDayGroups(filtered)
	default:
		m.buildWorkspaceGroups(filtered)
	}

	m.planScroll = state.NewScrollState()
	m.planScroll.SetTotal(m.planVisibleItemCount())
}

// buildWorkspaceGroups groups filtered plans by workspace path (default mode).
func (m *Model) buildWorkspaceGroups(filtered []core.PlanEntry) {
	groupMap := make(map[string][]core.PlanEntry)
	var groupOrder []string
	var ungrouped []core.PlanEntry

	for _, p := range filtered {
		if p.WorkspacePath == "" {
			ungrouped = append(ungrouped, p)
			continue
		}
		if _, exists := groupMap[p.WorkspacePath]; !exists {
			groupOrder = append(groupOrder, p.WorkspacePath)
		}
		groupMap[p.WorkspacePath] = append(groupMap[p.WorkspacePath], p)
	}

	m.planGroups = nil
	for _, key := range groupOrder {
		m.planGroups = append(m.planGroups, core.PlanGroup{
			WorkspacePath: key,
			Plans:         groupMap[key],
		})
	}
	if len(ungrouped) > 0 {
		m.planGroups = append(m.planGroups, core.PlanGroup{
			WorkspacePath: "",
			Plans:         ungrouped,
		})
	}
	m.planGroupOrder = groupOrder
}

// buildDayGroups groups filtered plans by the day of LastActive, sorted newest first.
// Each plan row shows a best-effort workspace folder approximation.
func (m *Model) buildDayGroups(filtered []core.PlanEntry) {
	now := time.Now()
	todayKey := now.Format("2006-01-02")
	yesterdayKey := now.AddDate(0, 0, -1).Format("2006-01-02")

	dayLabel := func(t time.Time) (key, label string) {
		if t.IsZero() {
			return "unknown", "Unknown"
		}
		key = t.Format("2006-01-02")
		switch key {
		case todayKey:
			label = "Today"
		case yesterdayKey:
			label = "Yesterday"
		default:
			if t.Year() == now.Year() {
				label = t.Format("Mon, Jan 2")
			} else {
				label = t.Format("Mon, Jan 2 2006")
			}
		}
		return key, label
	}

	type dayBucket struct {
		key   string
		label string
		plans []core.PlanEntry
	}

	bucketMap := make(map[string]*dayBucket)
	var bucketOrder []string

	for _, p := range filtered {
		k, l := dayLabel(p.LastActive)
		if b, ok := bucketMap[k]; ok {
			b.plans = append(b.plans, p)
		} else {
			bucketMap[k] = &dayBucket{key: k, label: l, plans: []core.PlanEntry{p}}
			bucketOrder = append(bucketOrder, k)
		}
	}

	// Sort day keys newest-first (reverse lexicographic for YYYY-MM-DD)
	sort.Sort(sort.Reverse(sort.StringSlice(bucketOrder)))

	m.planGroups = nil
	var groupOrder []string
	for _, k := range bucketOrder {
		b := bucketMap[k]
		m.planGroups = append(m.planGroups, core.PlanGroup{
			WorkspacePath: k,
			Label:         b.label,
			Plans:         b.plans,
		})
		groupOrder = append(groupOrder, k)
	}
	m.planGroupOrder = groupOrder
}

// loadPlanPreview reads the selected plan's file content for the preview pane.
func (m *Model) loadPlanPreview() {
	m.planPreviewLines = nil
	m.planPreviewOffset = 0
	ref, ok := m.planItemAt(m.planScroll.Selected)
	if !ok || ref.Kind != planItemPlan || ref.Plan == nil {
		return
	}
	plan := ref.Plan
	if plan.FilePath == "" || !strings.HasSuffix(plan.FilePath, ".md") {
		return
	}
	data, err := os.ReadFile(plan.FilePath)
	if err != nil {
		m.planPreviewLines = []string{"(error reading: " + err.Error() + ")"}
		return
	}
	m.planPreviewLines = strings.Split(string(data), "\n")
}

// handlePlansKey handles keyboard input in the plans view.
func (m Model) handlePlansKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "v":
		m.planPreviewVisible = !m.planPreviewVisible
		if m.planPreviewVisible {
			if m.planPreviewHeight == 0 {
				m.planPreviewHeight = 15
			}
			m.loadPlanPreview()
		}
		return m, nil

	case "[":
		if m.planPreviewVisible && m.planPreviewHeight > 5 {
			m.planPreviewHeight--
		}
		return m, nil

	case "]":
		if m.planPreviewVisible && m.planPreviewHeight < 30 {
			m.planPreviewHeight++
		}
		return m, nil

	case "j", "down":
		m.clearPlanSelection()
		m.planScroll.MoveDown()
		if m.planPreviewVisible {
			m.loadPlanPreview()
		}
		return m, nil

	case "k", "up":
		m.clearPlanSelection()
		m.planScroll.MoveUp()
		if m.planPreviewVisible {
			m.loadPlanPreview()
		}
		return m, nil

	case "J", "shift+down":
		if m.planPreviewVisible {
			maxOffset := len(m.planPreviewLines) - m.planPreviewHeight
			if maxOffset < 0 {
				maxOffset = 0
			}
			if m.planPreviewOffset < maxOffset {
				m.planPreviewOffset++
			}
			return m, nil
		}
		if len(m.planMultiSelected) == 0 {
			m.planSelectAnchor = m.planScroll.Selected
			m.markPlanItem(m.planScroll.Selected)
		}
		m.planScroll.MoveDown()
		m.selectPlanRange(m.planSelectAnchor, m.planScroll.Selected)
		return m, nil

	case "K", "shift+up":
		if m.planPreviewVisible {
			if m.planPreviewOffset > 0 {
				m.planPreviewOffset--
			}
			return m, nil
		}
		if len(m.planMultiSelected) == 0 {
			m.planSelectAnchor = m.planScroll.Selected
			m.markPlanItem(m.planScroll.Selected)
		}
		m.planScroll.MoveUp()
		m.selectPlanRange(m.planSelectAnchor, m.planScroll.Selected)
		return m, nil

	case " ":
		m.togglePlanItemSelection(m.planScroll.Selected)
		m.planScroll.MoveDown()
		return m, nil

	case "f":
		m.clearPlanSelection()
		m.planSourceFilter = m.planSourceFilter.Next()
		m.filterPlans()
		return m, nil

	case "c":
		m.clearPlanSelection()
		m.planShowCompleted = !m.planShowCompleted
		m.filterPlans()
		return m, nil

	case "S":
		if m.syncInProgress {
			return m, nil
		}
		m.syncInProgress = true
		return m, forceSyncCmd()

	case "g":
		m.clearPlanSelection()
		m.planGroupMode = m.planGroupMode.Next()
		m.expandedPlanGroups = make(map[string]bool)
		m.filterPlans()
		for _, g := range m.planGroups {
			if g.WorkspacePath != "" {
				m.expandedPlanGroups[g.WorkspacePath] = true
			}
		}
		m.planScroll.SetTotal(m.planVisibleItemCount())
		return m, nil

	case "/":
		m.clearPlanSelection()
		m.textInput.SetValue(m.planFilterText)
		m.textInput.Placeholder = "filter plans..."
		m.textInput.Focus()
		m.mode = core.ModePlanFilter
		return m, nil

	case "tab":
		m.clearPlanSelection()
		ref, ok := m.planItemAt(m.planScroll.Selected)
		if ok && ref.Kind == planItemGroup {
			wp := ref.Group.WorkspacePath
			if m.expandedPlanGroups[wp] {
				delete(m.expandedPlanGroups, wp)
			} else {
				m.expandedPlanGroups[wp] = true
			}
			m.planScroll.SetTotal(m.planVisibleItemCount())
		}
		return m, nil

	case "r":
		// Restructure selected plan (or all multi-selected)
		if len(m.planMultiSelected) > 0 {
			return m.bulkRestructurePlans()
		}
		ref, ok := m.planItemAt(m.planScroll.Selected)
		if !ok || ref.Kind != planItemPlan || ref.Plan == nil {
			return m, nil
		}
		plan := *ref.Plan
		if plan.FilePath == "" || m.planRestructuring[plan.FilePath] {
			return m, nil
		}
		// JSONL conversations: offer to convert to a plan
		if strings.HasSuffix(plan.FilePath, ".jsonl") {
			m.confirmMsg = fmt.Sprintf("Convert conversation '%s' to a plan?", truncate(plan.Title, 40))
			filePath := plan.FilePath
			workspace := plan.WorkspacePath
			m.confirmAction = func() tea.Cmd {
				return func() tea.Msg {
					return startConvertMsg{FilePath: filePath, Workspace: workspace}
				}
			}
			m.confirmReturnMode = core.ModePlans
			m.mode = core.ModeConfirm
			return m, nil
		}
		// Only restructure .md plan files
		if !strings.HasSuffix(plan.FilePath, ".md") {
			m.flashMessage = "Cannot restructure this file type"
			m.flashIsError = true
			m.flashTicks = 0
			return m, nil
		}
		m.confirmMsg = fmt.Sprintf("Restructure '%s' using claude -p?", truncate(plan.Title, 40))
		filePath := plan.FilePath
		m.confirmAction = func() tea.Cmd {
			return func() tea.Msg {
				return startRestructureMsg{FilePath: filePath}
			}
		}
		m.confirmReturnMode = core.ModePlans
		m.mode = core.ModeConfirm
		return m, nil

	case "R":
		m.clearPlanSelection()
		m.plansLoading = true
		return m, reloadPlansCmd()

	case "T":
		// Cycle through discovered tags (S3)
		m.clearPlanSelection()
		if len(m.planDiscoveredTags) == 0 {
			return m, nil
		}
		if m.planTagFilter == "" {
			m.planTagFilter = m.planDiscoveredTags[0]
		} else {
			found := false
			for i, tag := range m.planDiscoveredTags {
				if tag == m.planTagFilter {
					if i+1 < len(m.planDiscoveredTags) {
						m.planTagFilter = m.planDiscoveredTags[i+1]
					} else {
						m.planTagFilter = "" // cycle back to "all"
					}
					found = true
					break
				}
			}
			if !found {
				m.planTagFilter = ""
			}
		}
		m.filterPlans()
		return m, nil

	case "t":
		return m.generatePlanTitle()

	case "d":
		return m.deletePlan()

	case "enter":
		m.clearPlanSelection()
		ref, ok := m.planItemAt(m.planScroll.Selected)
		if ok && ref.Kind == planItemGroup {
			if !m.expandedPlanGroups[ref.Group.WorkspacePath] {
				m.expandedPlanGroups[ref.Group.WorkspacePath] = true
				m.planScroll.SetTotal(m.planVisibleItemCount())
				return m, nil
			}
			if len(ref.Group.Plans) > 0 {
				return m.resumePlanEntry(ref.Group.Plans[0])
			}
			return m, nil
		}
		return m.resumePlan()
	}

	return m, nil
}

// handlePlanFilterKey handles keyboard input while filtering plans.
func (m Model) handlePlanFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.planFilterText = ""
		m.filterPlans()
		m.mode = core.ModePlans
		return m, nil

	case "enter":
		m.planFilterText = m.textInput.Value()
		m.filterPlans()
		m.mode = core.ModePlans
		return m, nil
	}

	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)
	m.planFilterText = m.textInput.Value()
	m.filterPlans()
	return m, cmd
}

// resumePlan handles resuming the currently selected plan item.
func (m Model) resumePlan() (tea.Model, tea.Cmd) {
	ref, ok := m.planItemAt(m.planScroll.Selected)
	if !ok || ref.Kind != planItemPlan || ref.Plan == nil {
		return m, nil
	}
	return m.resumePlanEntry(*ref.Plan)
}

// resumePlanEntry opens or resumes a specific plan entry.
func (m Model) resumePlanEntry(plan core.PlanEntry) (tea.Model, tea.Cmd) {
	if plan.Source == core.SourceCursor {
		workspacePath := state.ExpandTilde(plan.WorkspacePath)
		planFilePath := plan.FilePath
		return m, func() tea.Msg {
			if workspacePath != "" {
				_ = exec.Run(exec.DefaultTimeout, "cursor", "--new-window", workspacePath, "--goto", planFilePath)
			} else {
				_ = exec.Run(exec.DefaultTimeout, "cursor", "--goto", planFilePath)
			}
			msg := "Opened plan in Cursor"
			if workspacePath != "" {
				msg += " (" + plan.WorkspacePath + ")"
			}
			return core.GitResultMsg{Success: true, Message: msg}
		}
	}

	// Claude Code conversation: check for a live session first
	for _, w := range m.windows {
		if w.Source == core.SourceCLI {
			for _, p := range w.Panes {
				if strings.Contains(p.WorkingDir, lastPathComponent(plan.WorkspacePath)) {
					return m, m.switchToWindow(&w)
				}
			}
		}
	}

	// No live session: prompt to create one with claude --resume
	m.confirmMsg = fmt.Sprintf("Resume '%s' in %s?", truncate(plan.Title, 40), plan.WorkspacePath)
	m.confirmAction = func() tea.Cmd {
		return func() tea.Msg {
			workspace := state.ExpandTilde(plan.WorkspacePath)
			sessionName := lastPathComponent(workspace)
			if sessionName == "" {
				sessionName = "claude-resume"
			}
			err := tmux.CreateSessionWithCommand(sessionName, workspace, "claude --resume "+plan.ConvID)
			if err != nil {
				return core.GitResultMsg{Success: false, Message: "Failed to create session: " + err.Error()}
			}
			return core.GitResultMsg{Success: true, Message: "Resumed in " + sessionName}
		}
	}
	m.confirmReturnMode = core.ModePlans
	m.mode = core.ModeConfirm
	return m, nil
}

// clearPlanSelection resets multi-selection state.
func (m *Model) clearPlanSelection() {
	m.planMultiSelected = make(map[int]bool)
	m.planSelectAnchor = -1
}

// markPlanItem adds a single flat index to the multi-selection (plan items only).
func (m *Model) markPlanItem(idx int) {
	ref, ok := m.planItemAt(idx)
	if ok && ref.Kind == planItemPlan {
		m.planMultiSelected[idx] = true
	}
}

// togglePlanItemSelection toggles a single item in/out of multi-selection.
func (m *Model) togglePlanItemSelection(idx int) {
	ref, ok := m.planItemAt(idx)
	if !ok || ref.Kind != planItemPlan {
		return
	}
	if m.planMultiSelected[idx] {
		delete(m.planMultiSelected, idx)
	} else {
		m.planMultiSelected[idx] = true
	}
	if m.planSelectAnchor < 0 {
		m.planSelectAnchor = idx
	}
}

// selectPlanRange replaces the multi-selection with all plan items between from and to (inclusive).
func (m *Model) selectPlanRange(from, to int) {
	m.planMultiSelected = make(map[int]bool)
	start, end := from, to
	if start > end {
		start, end = end, start
	}
	for i := start; i <= end; i++ {
		m.markPlanItem(i)
	}
}

// deletePlan handles deleting selected plan(s). Uses bulk delete when
// any items are multi-selected, otherwise deletes the single focused item.
func (m Model) deletePlan() (tea.Model, tea.Cmd) {
	if len(m.planMultiSelected) > 0 {
		return m.bulkDeletePlans()
	}

	ref, ok := m.planItemAt(m.planScroll.Selected)
	if !ok || ref.Kind != planItemPlan || ref.Plan == nil {
		return m, nil
	}
	plan := *ref.Plan

	sourceLabel := "Cursor"
	if plan.Source == core.SourceCLI {
		sourceLabel = "Claude"
	}

	m.confirmMsg = fmt.Sprintf("Delete %s plan %q?", sourceLabel, truncate(plan.Title, 40))
	planFile := plan.FilePath
	planTitle := plan.Title
	m.confirmAction = func() tea.Cmd {
		if err := os.Remove(planFile); err != nil {
			return func() tea.Msg {
				return core.GitResultMsg{Success: false, Message: "Failed to delete: " + err.Error()}
			}
		}
		return tea.Batch(
			func() tea.Msg {
				return core.GitResultMsg{Success: true, Message: "Deleted " + planTitle}
			},
			reloadPlansCmd(),
		)
	}
	m.clearPlanSelection()
	m.confirmReturnMode = core.ModePlans
	m.mode = core.ModeConfirm
	return m, nil
}

// bulkDeletePlans handles deleting all multi-selected plan files.
func (m Model) bulkDeletePlans() (tea.Model, tea.Cmd) {
	var toDelete []core.PlanEntry
	for idx := range m.planMultiSelected {
		ref, ok := m.planItemAt(idx)
		if ok && ref.Kind == planItemPlan && ref.Plan != nil {
			toDelete = append(toDelete, *ref.Plan)
		}
	}
	if len(toDelete) == 0 {
		return m, nil
	}

	if len(toDelete) == 1 {
		m.confirmMsg = fmt.Sprintf("Delete selected plan %q?", truncate(toDelete[0].Title, 40))
	} else {
		m.confirmMsg = fmt.Sprintf("Delete %d selected plans?", len(toDelete))
	}
	m.confirmAction = func() tea.Cmd {
		deleted := 0
		var lastErr error
		for _, p := range toDelete {
			if err := os.Remove(p.FilePath); err != nil {
				lastErr = err
			} else {
				deleted++
			}
		}
		noun := "plans"
		if deleted == 1 {
			noun = "plan"
		}
		msg := fmt.Sprintf("Deleted %d %s", deleted, noun)
		if lastErr != nil {
			if deleted == 0 {
				return func() tea.Msg {
					return core.GitResultMsg{Success: false, Message: "Failed to delete: " + lastErr.Error()}
				}
			}
			msg += fmt.Sprintf(" (%d failed)", len(toDelete)-deleted)
		}
		return tea.Batch(
			func() tea.Msg {
				return core.GitResultMsg{Success: true, Message: msg}
			},
			reloadPlansCmd(),
		)
	}
	m.clearPlanSelection()
	m.confirmReturnMode = core.ModePlans
	m.mode = core.ModeConfirm
	return m, nil
}

// generatePlanTitle triggers title generation for the selected plan(s).
// Supports multi-select: if plans are multi-selected, generates titles for all of them.
func (m Model) generatePlanTitle() (tea.Model, tea.Cmd) {
	if len(m.planMultiSelected) > 0 {
		return m.bulkGenerateTitles()
	}

	ref, ok := m.planItemAt(m.planScroll.Selected)
	if !ok || ref.Kind != planItemPlan || ref.Plan == nil {
		return m, nil
	}

	plan := *ref.Plan
	m.titlesGenerating++
	m.flashMessage = "Generating title..."
	m.flashIsError = false
	m.flashTicks = 0
	return m, plans.GenerateTitleCmd(plan)
}

// bulkGenerateTitles generates titles for all multi-selected plans sequentially.
func (m Model) bulkGenerateTitles() (tea.Model, tea.Cmd) {
	var toGenerate []core.PlanEntry
	for idx := range m.planMultiSelected {
		ref, ok := m.planItemAt(idx)
		if ok && ref.Kind == planItemPlan && ref.Plan != nil {
			toGenerate = append(toGenerate, *ref.Plan)
		}
	}
	if len(toGenerate) == 0 {
		return m, nil
	}

	m.titlesGenerating += len(toGenerate)
	noun := "titles"
	if len(toGenerate) == 1 {
		noun = "title"
	}
	m.flashMessage = fmt.Sprintf("Generating %d %s...", len(toGenerate), noun)
	m.flashIsError = false
	m.flashTicks = 0
	m.clearPlanSelection()

	var cmds []tea.Cmd
	for _, p := range toGenerate {
		cmds = append(cmds, plans.GenerateTitleCmd(p))
	}
	return m, tea.Batch(cmds...)
}

// bulkRestructurePlans restructures all multi-selected plans sequentially.
func (m Model) bulkRestructurePlans() (tea.Model, tea.Cmd) {
	var toRestructure []core.PlanEntry
	for idx := range m.planMultiSelected {
		ref, ok := m.planItemAt(idx)
		if ok && ref.Kind == planItemPlan && ref.Plan != nil && ref.Plan.FilePath != "" && strings.HasSuffix(ref.Plan.FilePath, ".md") {
			toRestructure = append(toRestructure, *ref.Plan)
		}
	}
	if len(toRestructure) == 0 {
		return m, nil
	}

	noun := "plans"
	if len(toRestructure) == 1 {
		noun = "plan"
	}
	m.confirmMsg = fmt.Sprintf("Restructure %d selected %s using claude -p?", len(toRestructure), noun)
	m.confirmAction = func() tea.Cmd {
		var cmds []tea.Cmd
		for _, p := range toRestructure {
			fp := p.FilePath
			cmds = append(cmds, func() tea.Msg {
				return startRestructureMsg{FilePath: fp}
			})
		}
		return tea.Batch(cmds...)
	}
	m.clearPlanSelection()
	m.confirmReturnMode = core.ModePlans
	m.mode = core.ModeConfirm
	return m, nil
}

// lastPathComponent returns the last element of a path.
func lastPathComponent(path string) string {
	path = strings.TrimRight(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// truncate shortens a string to maxLen, adding "..." if needed.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

