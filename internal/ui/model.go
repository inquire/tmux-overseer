package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/db"
	"github.com/inquire/tmux-overseer/internal/detect"
	"github.com/inquire/tmux-overseer/internal/hookserver"
	"github.com/inquire/tmux-overseer/internal/plans"
	"github.com/inquire/tmux-overseer/internal/state"
)

// Model is the main Bubble Tea model.
type Model struct {
	// Data
	windows         []core.ClaudeWindow
	items           []core.ListItem // flat list of selectable rows (windows + expanded panes)
	attachedSession string

	// View state
	mode            core.ViewMode
	scroll          state.ScrollState
	expandedWindows   map[string]bool // keyed by "session:windowIdx", all expanded by default
	expandedPlans     map[string]bool // keyed by ConversationID, shows plan todos when expanded
	expandedSubagents map[string]bool // keyed by ConversationID or "session:windowIdx"
	previewContent  string
	sortMode        core.SortMode
	lastSelectedID  string // remembers selected pane ID across refreshes

	// Spinner
	spinner     spinner.Model
	spinnerTick int // counts ticks for auto-refresh

	// Action menu
	actionIdx    int
	actions      []core.SessionAction
	actionWindow *core.ClaudeWindow // the window the action menu is for

	// Dialog state
	textInput         textinput.Model
	confirmMsg        string
	confirmAction     func() tea.Cmd
	confirmReturnMode core.ViewMode // mode to return to after confirm/cancel

	// Filter
	filterText   string
	filtered     []int // indices into windows that match filter
	sourceFilter core.SourceFilter // filter by session source (CLI/Cursor/All)

	// Messages
	flashMessage string
	flashIsError bool
	flashTicks   int

	// Preview pane configuration
	previewHeight int // number of visible content lines (default 10, range 4-20)
	previewOffset int // scroll offset within the preview content

	// Preview debouncing and cancellation
	previewPending   string             // paneID waiting for preview
	previewDebounced bool               // true if we're waiting for debounce timer
	previewCancel    context.CancelFunc // cancels the in-flight preview subprocess

	// Filter debouncing
	filterPending   string // filter text waiting to be applied
	filterDebounced bool   // true if we're waiting for filter debounce timer

	// Dimensions
	width  int
	height int

	// Mouse interaction tracking
	lastClickTime time.Time
	lastClickItem int

	// Cost tracking
	costTracker       *detect.CostTracker
	dayCostTotal      float64            // cached day total from last refresh
	dayCostPerSession map[string]float64 // cached per-session costs from last refresh

	// Session workspace groups (Cursor sessions sharing the same workspace)
	expandedGroups          map[string]bool // keyed by workspace path, true = expanded
	activityBreakdownLines  int             // height of the today breakdown viewport (0 = auto)
	expandedTeams  map[string]bool // keyed by team name, true = expanded
	groupMode      core.GroupMode  // how sessions are grouped (source vs workspace)

	// Plans view
	allPlans           []core.PlanEntry  // all loaded plans (unfiltered)
	planItems          []core.PlanEntry  // filtered/visible plans (flat, used for scroll index)
	planGroups         []core.PlanGroup  // grouped filtered plans (for rendering)
	planGroupOrder     []string          // workspace keys in display order
	expandedPlanGroups map[string]bool   // keyed by workspace path, true = expanded
	planScroll         state.ScrollState
	planSourceFilter   core.SourceFilter
	planGroupMode      core.PlanGroupMode
	planShowCompleted  bool
	planFilterText     string   // text filter for plan names
	planTagFilter      string   // active tag filter (empty = show all)
	planDiscoveredTags []string // unique tags across all plans
	plansLoading       bool
	plansLoaded        bool // true once plans have been loaded at least once

	// Plans multi-selection (for bulk delete)
	planMultiSelected map[int]bool // flat scroll indices marked for selection
	planSelectAnchor  int          // anchor index for shift-select ranges (-1 = none)

	// Plan preview pane (toggleable with 'v')
	planPreviewVisible bool
	planPreviewHeight  int      // number of preview lines (default 0, range 5-30)
	planPreviewOffset  int      // scroll offset within preview
	planPreviewLines   []string // file content lines for selected plan

	// Plan title generation
	titleOverrides    map[string]string // convID -> custom title (persisted across restarts)
	titlesGenerating  int              // number of in-flight title generation requests

	// Plan restructuring state (shared across plans view)
	planRestructuring map[string]bool

	// Activity view
	activityGrid        []core.ActivityDay
	activityProjects    []core.ActivityProject
	activePlans         []core.PlanEntry // incomplete plans for "Active Plans" strip
	activitySelectedDay int
	activityDayDetail   db.DayDetail
	activityScroll      state.ScrollState
	activityLoading     bool
	activityLoaded      bool

	// DuckDB sync
	syncInProgress bool
	syncPhase      string
	syncCurrent    int
	syncTotal      int
	syncDetail     string

	// HTTP hook server (real-time event delivery from Claude Code/Cursor hooks)
	hookEvents chan core.HookEventMsg // written by hookserver, read via waitForHookCmd
	hookServer *hookserver.Server    // nil if server failed to start

	// Terminal focus state
	focused bool // true when terminal has focus; used to throttle refresh

	// Styles (resolved for the terminal background; rebuilt on theme change)
	styles core.Styles

	// Lifecycle
	loading  bool // true while initial/async refresh is in-flight
	err      error
	quitting bool
}

// InitialModel creates the initial Bubble Tea model.
// If a sessions cache exists on disk, it is loaded immediately so the UI
// renders the session list on the very first frame instead of a loading screen.
// A background refresh is always fired from Init() to update stale data.
func InitialModel() Model {
	styles := core.NewStyles(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))

	s := spinner.New(
		spinner.WithSpinner(core.ClaudeFlowerSpinner),
		spinner.WithStyle(styles.StatusStyle(core.StatusWorking)),
	)

	ti := textinput.New()
	ti.CharLimit = 256

	lastSelected := state.LoadSelection()

	hookEvents := make(chan core.HookEventMsg, 64)
	statusDir := state.StatusDir()

	srv := hookserver.New(hookserver.DefaultPort, hookEvents, statusDir)
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel // held by the server goroutine; cleaned up via hookServer.RemovePortFile on quit
	if _, err := srv.Start(ctx); err != nil {
		// Non-fatal: fall back to polling-only mode.
		srv = nil
	}

	m := Model{
		mode:               core.ModeSessionList,
		scroll:             state.NewScrollState(),
		expandedWindows:    make(map[string]bool),
		expandedPlans:      make(map[string]bool),
		expandedSubagents:  make(map[string]bool),
		expandedGroups:     make(map[string]bool),
		expandedTeams:      make(map[string]bool),
		expandedPlanGroups: make(map[string]bool),
		sortMode:           core.SortByName,
		spinner:            s,
		textInput:          ti,
		focused:            true,
		loading:            true,
		lastSelectedID:     lastSelected,
		costTracker:        detect.NewCostTracker(),
		planMultiSelected:    make(map[int]bool),
		planRestructuring: make(map[string]bool),
		planSelectAnchor:   -1,
		titleOverrides:     make(map[string]string),
		hookEvents:         hookEvents,
		hookServer:         srv,
		previewHeight:      10,
		styles:             styles,
	}

	// Attempt to populate the model from a disk cache for instant first frame.
	if cached := state.LoadSessionsCache(); cached != nil && len(cached.Windows) > 0 {
		var windows []core.ClaudeWindow
		if err := json.Unmarshal(cached.Windows, &windows); err == nil && len(windows) > 0 {
			m.windows = windows
			m.attachedSession = cached.AttachedSession
			m.loading = false // skip loading screen; background refresh will still run
			for i := range m.windows {
				if len(m.windows[i].Panes) > 1 {
					key := fmt.Sprintf("%s:%d", m.windows[i].SessionName, m.windows[i].WindowIndex)
					m.expandedWindows[key] = true
				}
			}
			m.applySort()
			m.applyFilter()
			m.restoreSelection()
			m.skipSectionHeaders(1)
		}
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spinner.Tick, tickCmd(), refreshWindowsCmd(), tea.RequestBackgroundColor}
	if m.hookEvents != nil {
		cmds = append(cmds, waitForHookCmd(m.hookEvents))
	}
	return tea.Batch(cmds...)
}

// waitForHookCmd returns a Cmd that blocks until a HookEventMsg arrives on ch,
// then delivers it to the Bubble Tea loop. The model re-arms this after each event.
func waitForHookCmd(ch <-chan core.HookEventMsg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second/4, func(t time.Time) tea.Msg {
		return core.TickMsg(t)
	})
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Session list gets: total height minus fixed chrome.
		// Fixed chrome = header(3) + newlines/seps + preview(previewHeight+2) + statusbar(1) + footer(1)
		listHeightInLines := state.MaxInt(3, msg.Height-m.previewHeight-9)
		// ViewHeight is in items. Sessions vary from 2 lines (minimal) to 12+ (with tasks).
		// Use line budget directly — renderSessionList uses ViewHeight*2 as maxLines so
		// set ViewHeight = listHeightInLines/3 to keep that budget equal to available lines.
		listHeightInItems := state.MaxInt(1, listHeightInLines/3)
		m.scroll.SetViewHeight(listHeightInItems)
		return m, nil

	case tea.FocusMsg:
		m.focused = true
		return m, refreshWindowsCmd()

	case tea.BlurMsg:
		m.focused = false
		return m, nil

	case tea.BackgroundColorMsg:
		m.styles = core.NewStyles(msg.IsDark())
		return m, nil

	case core.TickMsg:
		m.spinnerTick++
		// Auto-refresh every ~5 seconds (20 ticks at 4 FPS), skip when unfocused
		if m.spinnerTick%20 == 0 && m.mode == core.ModeSessionList && m.focused {
			return m, tea.Batch(tickCmd(), refreshWindowsCmd())
		}
		// Live preview: re-fetch every ~2 seconds when selected session is working
		if m.spinnerTick%8 == 0 && m.mode == core.ModeSessionList {
			if _, p := m.selectedWindowAndPane(); p != nil && p.Status == core.StatusWorking {
				return m, tea.Batch(tickCmd(), m.fetchPreview(p.PaneID))
			}
		}
		// Flash message timeout
		if m.flashMessage != "" {
			m.flashTicks++
			if m.flashTicks > 12 { // ~3 seconds
				m.flashMessage = ""
				m.flashTicks = 0
			}
		}
		return m, tickCmd()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case core.WindowsMsg:
		m.loading = false
		if msg.Err == nil {
			m.windows = msg.Windows

			// Annotate team membership from ~/.claude/teams/*/config.json.
			detect.AnnotateTeams(m.windows)

			m.recordAndAugmentCosts()

			if msg.AttachedSession != "" {
				m.attachedSession = msg.AttachedSession
			}

			// Persist session list to disk so the next launch can skip the loading screen.
			if raw, err := json.Marshal(m.windows); err == nil {
				state.SaveSessionsCache(m.attachedSession, raw)
			}
			// Auto-expand all multi-pane windows
			for i := range m.windows {
				if len(m.windows[i].Panes) > 1 {
					key := fmt.Sprintf("%s:%d", m.windows[i].SessionName, m.windows[i].WindowIndex)
					m.expandedWindows[key] = true
				}
				m.windows[i].ComputeSearchText()
			}
			m.applySort()
			m.applyFilter()
			m.restoreSelection()
			m.skipSectionHeaders(1)
		} else if m.windows == nil {
			m.err = msg.Err
		}
		cmds := []tea.Cmd{m.previewCmd()}
		if !m.plansLoaded && !m.plansLoading && msg.Err == nil {
			m.plansLoading = true
			cmds = append(cmds, loadPlansCmd())
		}
		// Background: link active sessions to plans in DuckDB
		go db.LinkSessionToPlans(m.windows)
		return m, tea.Batch(cmds...)

	case core.PreviewMsg:
		_, p := m.selectedWindowAndPane()
		if p != nil && p.PaneID == msg.PaneID {
			m.previewContent = msg.Content
		} else if p == nil {
			m.previewContent = ""
		}
		return m, nil

	case core.PreviewDebounceMsg:
		m.previewDebounced = false
		_, p := m.selectedWindowAndPane()
		if p != nil && p.PaneID == msg.PaneID {
			return m, m.fetchPreview(p.PaneID)
		}
		return m, nil

	case core.FilterDebounceMsg:
		m.filterDebounced = false
		if m.filterText == msg.FilterText {
			m.applyFilter()
		}
		return m, nil

	case core.HookEventMsg:
		// Re-arm the listener immediately so the next event isn't missed.
		var cmds []tea.Cmd
		if m.hookEvents != nil {
			cmds = append(cmds, waitForHookCmd(m.hookEvents))
		}
		// Apply the in-place update for Cursor sessions (conversation_id is available).
		// For CLI sessions (no pane_id in HTTP path), trigger a fast refresh instead.
		if msg.ConversationID != "" {
			m.applyHookEvent(msg)
		} else {
			// CLI session: schedule a rapid refresh (shell script has already updated the file).
			cmds = append(cmds, refreshWindowsCmd())
		}
		return m, tea.Batch(cmds...)

	case core.GitResultMsg:
		m.flashMessage = msg.Message
		m.flashIsError = !msg.Success
		m.flashTicks = 0
		if msg.Success {
			return m, refreshWindowsCmd()
		}
		return m, nil

	case core.PlansMsg:
		m.plansLoaded = true
		if msg.Err == nil {
			m.allPlans = msg.Plans
			if len(m.titleOverrides) == 0 {
				m.titleOverrides = plans.LoadTitleOverrides()
			}
			plans.ApplyTitleOverrides(m.allPlans, m.titleOverrides)
			m.filterPlans()
		}
		if msg.FromCache {
			// Stale cache was served — kick off a background rescan.
			m.plansLoading = true
			return m, reloadPlansCmd()
		}
		m.plansLoading = false
		return m, nil

	case core.TitleGeneratedMsg:
		m.titlesGenerating--
		if msg.Err != nil {
			m.flashMessage = "Title generation failed: " + msg.Err.Error()
			m.flashIsError = true
			m.flashTicks = 0
			return m, nil
		}
		m.titleOverrides[msg.ConvID] = msg.NewTitle
		for i := range m.allPlans {
			if m.allPlans[i].ConvID == msg.ConvID {
				m.allPlans[i].Title = msg.NewTitle
				break
			}
		}
		m.filterPlans()
		m.flashMessage = "Title: " + msg.NewTitle
		m.flashIsError = false
		m.flashTicks = 0
		return m, nil

	case startConvertMsg:
		m.planRestructuring[msg.FilePath] = true
		m.flashMessage = "Converting conversation to plan..."
		m.flashIsError = false
		m.flashTicks = 0
		return m, convertConversationCmd(msg.FilePath, msg.Workspace)

	case core.ConvertConversationMsg:
		delete(m.planRestructuring, msg.OriginalPath)
		if msg.Err != nil {
			errStr := msg.Err.Error()
			if len(errStr) > 200 {
				errStr = errStr[:200] + "..."
			}
			m.flashMessage = "Conversion failed: " + errStr
			m.flashIsError = true
			m.flashTicks = -20
		} else {
			m.flashMessage = "Created plan: " + msg.Title
			m.flashIsError = false
			m.flashTicks = 0
		}
		return m, reloadPlansCmd()

	case startRestructureMsg:
		m.planRestructuring[msg.FilePath] = true
		m.flashMessage = "Restructuring plan..."
		m.flashIsError = false
		m.flashTicks = 0
		return m, restructurePlanCmd(msg.FilePath)

	case core.RestructurePlanMsg:
		delete(m.planRestructuring, msg.FilePath)
		if msg.Err != nil {
			// Truncate error for display but keep it readable
			errStr := msg.Err.Error()
			if len(errStr) > 200 {
				errStr = errStr[:200] + "..."
			}
			m.flashMessage = "Restructure failed: " + errStr
			m.flashIsError = true
			m.flashTicks = -20 // persist longer (~8 seconds total)
		} else {
			m.flashMessage = "Plan restructured successfully"
			m.flashIsError = false
			m.flashTicks = 0
		}
		return m, reloadPlansCmd()

	case core.SyncProgressMsg:
		return m.handleSyncProgress(msg)

	case core.ActivityDataMsg:
		m.activityLoading = false
		m.activityLoaded = true
		if msg.Err == nil {
			m.activityGrid = msg.Grid
			m.activityProjects = msg.Projects
			m.activePlans = msg.ActivePlans
			m = m.initActivitySelectedDay()
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			return m.handleMouseClick(msg.Y, msg.X)
		}
		return m, nil

	case tea.MouseWheelMsg:
		if msg.Button == tea.MouseWheelUp {
			if m.mode == core.ModeSessionList {
				m.scroll.MoveUp()
				m.skipSectionHeaders(-1)
				m.saveSelection()
				return m, m.schedulePreview()
			}
			if m.mode == core.ModePlans {
				m.planScroll.MoveUp()
				return m, nil
			}
			if m.mode == core.ModeActivity {
				m.activityScroll.MoveUp()
				return m, nil
			}
		}
		if msg.Button == tea.MouseWheelDown {
			if m.mode == core.ModeSessionList {
				m.scroll.MoveDown()
				m.skipSectionHeaders(1)
				m.saveSelection()
				return m, m.schedulePreview()
			}
			if m.mode == core.ModePlans {
				m.planScroll.MoveDown()
				return m, nil
			}
			if m.mode == core.ModeActivity {
				m.activityScroll.MoveDown()
				return m, nil
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		m.quitting = true
		return m, tea.Quit
	}

	// Unified tab navigation from any main view
	if m2, cmd, handled := m.handleCommonTabNav(key); handled {
		return m2, cmd
	}

	switch m.mode {
	case core.ModeSessionList:
		return m.handleSessionListKey(key)
	case core.ModeActionMenu:
		return m.handleActionMenuKey(key)
	case core.ModeFilter:
		return m.handleFilterKey(msg)
	case core.ModeSendInput:
		return m.handleSendInputKey(msg)
	case core.ModeConfirm:
		return m.handleConfirmKey(key)
	case core.ModeRename:
		return m.handleRenameKey(msg)
	case core.ModeNewSession:
		return m.handleNewSessionKey(msg)
	case core.ModeCommit:
		return m.handleCommitKey(msg)
	case core.ModeNewWorktree:
		return m.handleNewWorktreeKey(msg)
	case core.ModePlans:
		return m.handlePlansKey(key)
	case core.ModePlanFilter:
		return m.handlePlanFilterKey(msg)
	case core.ModeActivity:
		return m.handleActivityKey(key)
	}

	return m, nil
}

// handleCommonTabNav handles tab switching keys consistently from any main view.
// Returns handled=true if the key was a tab navigation key.
func (m Model) handleCommonTabNav(key string) (Model, tea.Cmd, bool) {
	switch m.mode {
	case core.ModeSessionList:
		switch key {
		case "p", "2":
			m.mode = core.ModePlans
			if !m.plansLoaded {
				m.plansLoading = true
				return m, loadPlansCmd(), true
			}
			return m, nil, true
		case "a", "3":
			m.mode = core.ModeActivity
			if !m.activityLoaded {
				m.activityLoading = true
				return m, loadActivityDataCmd(), true
			}
			return m, nil, true
		}

	case core.ModePlans:
		switch key {
		case "q":
			m.quitting = true
			return m, tea.Quit, true
		case "esc", "1":
			m.planFilterText = ""
			m.clearPlanSelection()
			m.mode = core.ModeSessionList
			return m, m.previewCmd(), true
		case "a", "3":
			m.mode = core.ModeActivity
			if !m.activityLoaded {
				m.activityLoading = true
				return m, loadActivityDataCmd(), true
			}
			return m, nil, true
		}

	case core.ModeActivity:
		switch key {
		case "q":
			m.quitting = true
			return m, tea.Quit, true
		case "esc", "1":
			m.mode = core.ModeSessionList
			return m, m.previewCmd(), true
		case "p", "2":
			m.mode = core.ModePlans
			if !m.plansLoaded {
				m.plansLoading = true
				return m, loadPlansCmd(), true
			}
			return m, nil, true
		}
	}
	return m, nil, false
}

// applyHookEvent updates a Cursor window's status in-place from a HookEventMsg.
// Called when the HTTP hook server delivers a real-time event. This avoids
// waiting for the next polling cycle for Cursor sessions.
func (m *Model) applyHookEvent(msg core.HookEventMsg) {
	for i := range m.windows {
		w := &m.windows[i]
		if w.Source != core.SourceCursor || w.ConversationID != msg.ConversationID {
			continue
		}
		// Update status on the primary pane.
		if len(w.Panes) > 0 {
			switch msg.Status {
			case "working":
				w.Panes[0].Status = core.StatusWorking
			case "waiting":
				w.Panes[0].Status = core.StatusWaitingInput
			case "idle":
				w.Panes[0].Status = core.StatusIdle
			}
			if msg.Model != "" {
				w.Panes[0].Model = msg.Model
			}
			if msg.CWD != "" {
				w.Panes[0].WorkingDir = msg.CWD
			}
		}
		// Update window-level fields.
		if msg.AgentMode != "" {
			w.AgentMode = msg.AgentMode
		}
		if msg.PermissionMode != "" {
			w.PermissionMode = msg.PermissionMode
		}
		if msg.WorktreePath != "" {
			w.WorktreePath = msg.WorktreePath
			w.WorktreeBranch = msg.WorktreeBranch
			w.OriginalRepo = msg.OriginalRepo
		}
		if msg.EffortLevel != "" {
			w.EffortLevel = msg.EffortLevel
		}
		break
	}
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	v := tea.NewView(renderApp(m))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion

	title := fmt.Sprintf("overseer · %d sessions", len(m.windows))
	for _, w := range m.windows {
		if w.HasActivePlan() {
			title = fmt.Sprintf("overseer · %s (%d/%d)", w.ActivePlanTitle, w.ActivePlanDone, w.ActivePlanTotal)
			break
		}
	}
	v.WindowTitle = title
	v.ReportFocus = true
	return v
}
