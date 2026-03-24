# Sessions View Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current tangled sessions view with a clean 3-line collapsed layout (name/status/cost, path/model/progress, activity line) and an organised expanded view with numbered orange tasks and cyan activity subgroups.

**Architecture:** Gut `renderSessionRow` in `views_sessions.go` (339 lines → ~120) into three pure functions: `renderSessionRowLine1`, `renderSessionRowLine2`, `renderSessionRowExpanded`. The expansion state collapses to a single bool. All data is already on `ClaudeWindow` — no model changes needed.

**Tech Stack:** Go, Bubble Tea, Lip Gloss v2. Key files: `internal/ui/views_sessions.go`, `internal/ui/model_sessions.go`, `internal/core/types.go`.

## Constraints

**Keyboard shortcuts — all must survive the refactor unchanged:**
| Key | Behaviour |
|-----|-----------|
| `↑↓` / `jk` | Navigate sessions |
| `Tab` | Cycle expansion: collapsed → tasks+activity → collapsed |
| `Enter` | Attach / open session |
| `l/→` | Open action menu |
| `s` | Cycle sort (name / status / recency) |
| `f` | Cycle source filter (All / CLI / Cursor / Cloud) |
| `g` | Toggle group mode (source vs workspace) |
| `/` | Text filter |
| `[/]` | Resize preview pane |
| `1/2/3` | Tab switch (Sessions / Plans / Activity) |
| `n` | New session |
| `d` | Delete session |
| `R` | Force refresh |
| `q/Esc` | Quit / back |

Every key must be verified working in Task 9 (regression test step) after the rewrite.

**Performance principles:**
- `renderSessionRow` and its helpers must be **pure functions** — same input → same output, no side effects, no map lookups inside the renderer. All expansion state is resolved by the caller and passed as plain bools.
- No allocations inside the hot render path (avoid `fmt.Sprintf` in tight loops — use `strings.Builder` instead).
- `lipgloss.Width()` is expensive — call it at most once per rendered string per frame. Cache widths in local vars.
- The scroll viewport (`maxLines`) must be recomputed on `WindowSizeMsg` only, not every frame.

**Extensibility principles:**
- Each section (line1, line2, expanded-tasks, expanded-activity) is its own function with a clear signature. Adding a new section (e.g. cost breakdown, PR link) means adding one function and one call site — nothing else changes.
- All magic numbers (truncation lengths, max bar blocks, max task items shown) are named constants at the top of `views_sessions.go`.
- No business logic inside renderers — renderers format data, they do not compute it. Progress counts, last tool, subagent status must already be on `ClaudeWindow` when `renderSessionRow` is called.

---

## File Map

| File | Change |
|------|--------|
| `internal/ui/views_sessions.go` | Full rewrite of `renderSessionRow` and helpers |
| `internal/ui/model_sessions.go` | Simplify `hasExpandablePlan` and `toggleExpand` logic |
| `internal/core/styles.go` | Add `TaskItemStyle`, `ActivityItemStyle` named styles |

---

### Task 1: Add named styles for tasks (orange) and activity (cyan)

**Files:**
- Modify: `internal/core/styles.go`

- [ ] **Step 1: Add the two new styles after the existing badge styles**

```go
// Task list styles (orange — Claude Code native tasks)
TaskSectionStyle = lipgloss.NewStyle().Foreground(ColorOrange)
TaskActiveStyle  = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
TaskPendingStyle = lipgloss.NewStyle().Foreground(ColorOrange)

// Activity styles (cyan — current tool use / subagents)
ActivitySectionStyle = lipgloss.NewStyle().Foreground(ColorCyan)
ActivityItemStyle    = lipgloss.NewStyle().Foreground(ColorCyan)
```

- [ ] **Step 2: Build and verify no errors**

```bash
go build ./internal/core/...
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add internal/core/styles.go
git commit -m "feat: add TaskSectionStyle and ActivitySectionStyle to palette"
```

---

### Task 2: Write tests for the new session row renderers

**Files:**
- Create: `internal/ui/views_sessions_test.go`

The current file has no tests. We add a table-driven test for each of the three line renderers before writing them.

- [ ] **Step 1: Create the test file with failing tests**

```go
package ui

import (
    "strings"
    "testing"
    "github.com/inquire/tmux-overseer/internal/core"
)

func makeTestWindow(source core.SessionSource, status core.Status, cost float64) core.ClaudeWindow {
    return core.ClaudeWindow{
        SessionName: "test-session",
        WindowName:  "test-session",
        Source:      source,
        Panes: []core.ClaudePane{{
            PaneID:     "%1",
            Status:     status,
            WorkingDir: "/home/user/project",
            Model:      "claude-sonnet-4-6",
            HasGit:     true,
            GitBranch:  "main",
            Cost:       cost,
        }},
    }
}

func TestRenderSessionRowLine1_CLI(t *testing.T) {
    win := makeTestWindow(core.SourceCLI, core.StatusWorking, 1.23)
    line := renderSessionRowLine1(win, false, 80)
    if !strings.Contains(line, "test-session") {
        t.Errorf("line1 missing session name: %q", line)
    }
    if !strings.Contains(line, "$1.23") {
        t.Errorf("line1 missing cost: %q", line)
    }
}

func TestRenderSessionRowLine2_ShowsProgress(t *testing.T) {
    win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
    win.ActivePlanDone = 3
    win.ActivePlanTotal = 10
    win.ActivePlanTitle = "tasks"
    line := renderSessionRowLine2(win, false, 80)
    if !strings.Contains(line, "3/10") {
        t.Errorf("line2 missing progress count: %q", line)
    }
}

func TestRenderSessionRowLine2_ShowsLastTool(t *testing.T) {
    win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
    win.Panes[0].LastTool = "Bash"
    line := renderSessionRowLine2(win, false, 80)
    if !strings.Contains(line, "Bash") {
        t.Errorf("line2 missing last tool: %q", line)
    }
}

func TestRenderSessionRowExpanded_TasksNumbered(t *testing.T) {
    win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
    win.TaskTodos = []core.PlanTodo{
        {Content: "first task", Status: "completed"},
        {Content: "second task", Status: "in_progress"},
        {Content: "third task", Status: "pending"},
    }
    expanded := renderSessionRowExpanded(win, 80)
    if !strings.Contains(expanded, "1.") {
        t.Errorf("expanded missing task number 1: %q", expanded)
    }
    if !strings.Contains(expanded, "2.") {
        t.Errorf("expanded missing task number 2: %q", expanded)
    }
    if !strings.Contains(expanded, "tasks") {
        t.Errorf("expanded missing tasks header: %q", expanded)
    }
}

func TestRenderSessionRowExpanded_ActivitySection(t *testing.T) {
    win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
    win.Subagents = []core.Subagent{
        {AgentType: "Explore", Description: "find auth", CurrentTool: "Grep"},
    }
    expanded := renderSessionRowExpanded(win, 80)
    if !strings.Contains(expanded, "activity") {
        t.Errorf("expanded missing activity header: %q", expanded)
    }
    if !strings.Contains(expanded, "Explore") {
        t.Errorf("expanded missing subagent type: %q", expanded)
    }
}
```

- [ ] **Step 2: Run to confirm they fail (functions don't exist yet)**

```bash
go test ./internal/ui/... 2>&1 | head -20
```
Expected: compile errors — `renderSessionRowLine1 undefined`, etc.

---

### Task 3: Implement `renderSessionRowLine1`

**Files:**
- Modify: `internal/ui/views_sessions.go`

Line 1: `▸/▾/space  name  [badges]  status  cost` — right-aligned status+cost.

- [ ] **Step 1: Add `renderSessionRowLine1` below the existing `renderPaneRow` function**

```go
// renderSessionRowLine1 renders the first line of a session row:
//   marker  name  [badges]                    status  $cost
func renderSessionRowLine1(win core.ClaudeWindow, selected bool, w int) string {
    name := win.DisplayName()
    badge := buildBadgeStr(win)

    // Status
    aggStatus := win.AggregateStatus()
    statusStr := core.StatusStyle(aggStatus).Render(aggStatus.Symbol())
    statusLabel := core.StatusStyle(aggStatus).Render(aggStatus.Label())
    if len(win.Subagents) > 0 || win.SubagentCount > 0 {
        n := len(win.Subagents)
        if n == 0 {
            n = win.SubagentCount
        }
        noun := "subagents"
        if n == 1 {
            noun = "subagent"
        }
        statusLabel += core.SubagentCountStyle.Render(fmt.Sprintf(" (%d %s)", n, noun))
    }

    // Cost
    costStr := ""
    if c := win.TotalCost(); c > 0 || win.Source == core.SourceCLI {
        costStr = core.CostStyle.Render(fmt.Sprintf("$%.2f", c))
    }

    right := statusStr + " " + statusLabel
    if costStr != "" {
        right += "  " + costStr
    }

    marker := " "
    if win.HasActivePlan() || len(win.TaskTodos) > 0 || len(win.Subagents) > 0 {
        // will be set by caller based on expansion state
    }

    var nameRendered string
    if selected {
        nameRendered = core.SelectedRowStyle.Render(marker + name)
    } else {
        nameRendered = core.NormalRowStyle.Render(marker+name)
    }
    left := nameRendered + badge

    gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
    if gap < 1 {
        gap = 1
    }
    return left + strings.Repeat(" ", gap) + right
}
```

> Note: the marker (`▸`/`▾`) is injected by the caller (`renderSessionRow`) which knows the expansion state. `renderSessionRowLine1` receives the already-substituted `name` with marker prepended. Refine the signature in the next step.

- [ ] **Step 2: Build to check for compilation errors**

```bash
go build ./internal/ui/... 2>&1
```

Fix any errors before continuing.

---

### Task 4: Implement `renderSessionRowLine2`

**Files:**
- Modify: `internal/ui/views_sessions.go`

Line 2: `  path  (branch)*  model  ■■□□ N/T  ·  LastTool`

- [ ] **Step 1: Add `renderSessionRowLine2`**

```go
// renderSessionRowLine2 renders the second line of a session row:
//   path  (branch)  model  ■■□□ 3/10  ·  Bash(cmd)
func renderSessionRowLine2(win core.ClaudeWindow, inGroup bool, w int) string {
    p := win.PrimaryPane()
    if p == nil {
        return ""
    }

    var parts []string

    if !inGroup {
        parts = append(parts, core.DimRowStyle.Render(state.ShortenPath(p.WorkingDir)))
        if p.HasGit {
            branch := "(" + p.GitBranch + ")"
            if p.IsWorktree {
                branch = "[" + p.GitBranch + "]"
            }
            gitStr := core.GitBranchStyle.Render(branch)
            if p.GitStaged {
                gitStr += core.GitStagedStyle.Render("+")
            }
            if p.GitDirty {
                gitStr += core.GitDirtyStyle.Render("*")
            }
            parts = append(parts, gitStr)
        }
    }

    if p.Model != "" {
        modelStr := core.ModelNameStyle.Render(shortenModel(p.Model))
        if win.EffortLevel != "" {
            modelStr += " " + core.EffortLevelStyle.Render(effortSymbol(win.EffortLevel))
        }
        parts = append(parts, modelStr)
    }

    // Progress bar (plan or tasks)
    title, done, total := win.ActivePlanTitle, win.ActivePlanDone, win.ActivePlanTotal
    if total == 0 && len(win.TaskTodos) > 0 {
        done = 0
        total = len(win.TaskTodos)
        title = "tasks"
        for _, t := range win.TaskTodos {
            if t.Status == "completed" {
                done++
            }
        }
    }
    if title != "" && total > 0 {
        maxBlocks := 10
        if total < maxBlocks {
            maxBlocks = total
        }
        filled := (done * maxBlocks) / total
        bar := core.PlanBarFilledStyle.Render(strings.Repeat("■", filled)) +
            core.PlanBarEmptyStyle.Render(strings.Repeat("□", maxBlocks-filled))
        parts = append(parts, bar+" "+core.DimRowStyle.Render(fmt.Sprintf("%d/%d", done, total)))
    }

    // Last tool / current activity (dim, only when working)
    if p.LastTool != "" && win.AggregateStatus() == core.StatusWorking {
        parts = append(parts, core.ActivityItemStyle.Render("› "+p.LastTool))
    }

    // Meta: prompt count + duration
    if win.Source != core.SourceCloud {
        if win.AgentMode == "plan" {
            parts = append(parts, core.PlanModeBadgeStyle.Render("[PLAN]"))
        } else if win.AgentMode == "agent" {
            parts = append(parts, core.AgentModeBadgeStyle.Render("[AGENT]"))
        }
        if win.PromptCount > 0 {
            noun := "prompts"
            if win.PromptCount == 1 {
                noun = "prompt"
            }
            parts = append(parts, core.SessionStatsStyle.Render(fmt.Sprintf("%d %s", win.PromptCount, noun)))
        }
        if dur := win.SessionDuration(); dur != "" {
            parts = append(parts, core.SessionStatsStyle.Render(dur))
        }
    }

    return "  " + strings.Join(parts, "  ")
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/ui/... 2>&1
```

---

### Task 5: Implement `renderSessionRowExpanded`

**Files:**
- Modify: `internal/ui/views_sessions.go`

The expanded block — shown when Tab is pressed. Renders tasks subgroup (numbered, orange) and activity subgroup (cyan), separated by `│` connectors.

- [ ] **Step 1: Add `renderSessionRowExpanded`**

```go
// renderSessionRowExpanded renders the expandable block for a session:
//   │
//   ├─ tasks
//   │  1. ✓ first task
//   │  2. ● second task   (orange bold — active)
//   │  3. ○ third task    (orange — pending)
//   │
//   └─ activity
//      › Bash(go build)
//      ⊕ Explore(find auth)  › Grep(password)
func renderSessionRowExpanded(win core.ClaudeWindow, w int) string {
    var sb strings.Builder

    hasTasks := len(win.TaskTodos) > 0 || len(win.ActivePlanTodos) > 0
    hasActivity := len(win.Subagents) > 0

    if !hasTasks && !hasActivity {
        return ""
    }

    sb.WriteString("\n  │")

    if hasTasks {
        activityConnector := "└─"
        if hasActivity {
            activityConnector = "├─"
        }
        _ = activityConnector

        sb.WriteString("\n  " + core.TaskSectionStyle.Render("├─ tasks"))

        todos := win.TaskTodos
        if len(todos) == 0 {
            todos = win.ActivePlanTodos
        }

        for i, t := range todos {
            num := fmt.Sprintf("%d.", i+1)
            icon, style := taskIconStyle(t.Status)
            content := truncate(t.Content, 55)
            line := fmt.Sprintf("\n  │  %s %s",
                core.TaskSectionStyle.Render(num),
                style.Render(icon+" "+content))
            sb.WriteString(line)
        }

        if hasActivity {
            sb.WriteString("\n  │")
        }
    }

    if hasActivity {
        sb.WriteString("\n  " + core.ActivitySectionStyle.Render("└─ activity"))
        for _, sa := range win.Subagents {
            label := sa.AgentType + "(" + truncate(sa.Description, 30) + ")"
            line := sa.AgentType
            if sa.Description != "" {
                line = sa.AgentType + "(" + truncate(sa.Description, 30) + ")"
            }
            _ = label
            actLine := "  › " + line
            if sa.CurrentTool != "" {
                tool := sa.CurrentTool
                if sa.CurrentToolInput != "" {
                    tool += "(" + truncate(sa.CurrentToolInput, 25) + ")"
                }
                actLine += "  › " + tool
            }
            sb.WriteString("\n  " + core.ActivityItemStyle.Render("   "+actLine))
        }
    }

    return sb.String()
}

// taskIconStyle returns icon and style for a task item.
func taskIconStyle(status string) (string, lipgloss.Style) {
    switch status {
    case "completed":
        return "✓", lipgloss.NewStyle().Foreground(core.ColorGreen)
    case "in_progress":
        return "●", core.TaskActiveStyle
    case "cancelled":
        return "✗", lipgloss.NewStyle().Foreground(core.ColorRed)
    default:
        return "○", core.TaskPendingStyle
    }
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/ui/... 2>&1
```

---

### Task 6: Rewrite `renderSessionRow` to use the three new functions

**Files:**
- Modify: `internal/ui/views_sessions.go`

This is the gutting step. Replace the 339-line `renderSessionRow` with a thin orchestrator that calls the three new pure functions.

- [ ] **Step 1: Replace `renderSessionRow` body**

```go
func renderSessionRow(m Model, win core.ClaudeWindow, selected, expanded, inGroup bool, w int) string {
    if win.PrimaryPane() == nil {
        return ""
    }

    planKey := win.ConversationID
    if planKey == "" {
        planKey = fmt.Sprintf("%s:%d", win.SessionName, win.WindowIndex)
    }
    planExpanded := m.expandedPlans[planKey]

    // Determine marker
    expandable := win.HasActivePlan() || len(win.TaskTodos) > 0 || len(win.Subagents) > 0 || len(win.Panes) > 1
    marker := " "
    if expandable {
        if planExpanded || expanded {
            marker = "▾ "
        } else {
            marker = "▸ "
        }
    }

    // Inject marker into display name
    name := marker + win.DisplayName()

    line1 := renderSessionRowLine1WithName(win, name, selected, w)
    line2 := renderSessionRowLine2(win, inGroup, w)

    result := line1
    if line2 != "" {
        result += "\n" + line2
    }

    if planExpanded {
        result += renderSessionRowExpanded(win, w)
    }

    // Blank line separator between sessions
    result += "\n"

    return result
}
```

- [ ] **Step 2: Build**

```bash
go build ./internal/ui/... 2>&1
```

Fix any compilation errors.

- [ ] **Step 3: Run tests**

```bash
go test ./internal/ui/... -v 2>&1 | tail -20
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/views_sessions.go internal/core/styles.go internal/ui/views_sessions_test.go
git commit -m "feat: rewrite session row with clean 3-line layout + task/activity subgroups"
```

---

### Task 7: Update scroll viewport to handle variable-height rows

**Files:**
- Modify: `internal/ui/model.go`
- Modify: `internal/ui/views_sessions.go` (maxLines calc)

Sessions now have 2-3 lines collapsed and N+3 lines expanded. The scroll viewport must accommodate this.

- [ ] **Step 1: Update `renderSessionList` maxLines calculation**

In `renderSessionList`, change:
```go
maxLines := m.scroll.ViewHeight * 2
```
to:
```go
maxLines := m.scroll.ViewHeight * 3  // sessions are now 2-3 lines each collapsed
```

- [ ] **Step 2: Update `WindowSizeMsg` handler in `model.go`**

Change:
```go
listHeightInItems := state.MaxInt(1, listHeightInLines/2)
```
to:
```go
listHeightInItems := state.MaxInt(1, listHeightInLines/3)
```

- [ ] **Step 3: Build and smoke-test visually**

```bash
go build ./... && go install ./cmd/tmux-overseer/
```
Launch tmux-overseer and verify sessions look right.

- [ ] **Step 4: Commit**

```bash
git add internal/ui/views_sessions.go internal/ui/model.go
git commit -m "fix: update scroll viewport for 3-line session rows"
```

---

### Task 8: Remove dead code from the old implementation

**Files:**
- Modify: `internal/ui/views_sessions.go`

The old `renderSessionRow` left behind several helpers that are now unused or duplicated.

- [ ] **Step 1: Identify and remove dead functions**

Check for unused functions:
```bash
go vet ./internal/ui/... 2>&1
```

Remove any of these if no longer called:
- `todoIconStyle` (replaced by `taskIconStyle`)
- `renderActivePlansStrip` (removed earlier)
- Any other flagged unused functions

- [ ] **Step 2: Build and test**

```bash
go build ./... && go test ./... 2>&1 | grep -E "FAIL|ok"
```

Expected: all packages pass.

- [ ] **Step 3: Final commit**

```bash
git add internal/ui/views_sessions.go
git commit -m "chore: remove dead code from sessions view rewrite"
```

---

## Verification

After all tasks complete:

1. `go test ./...` — all green
2. Launch tmux-overseer with multiple active sessions
3. Verify collapsed view: 2 lines (idle) or 3 lines (working — with activity on line 3)
4. Press Tab on a session with tasks — tasks subgroup appears numbered in orange
5. Press Tab on a session with subagents — activity subgroup appears in cyan
6. Verify blank line separates each session
7. Verify progress bar appears on line 2 for sessions with tasks or plan todos
8. Press `g` to toggle group mode — verify both views still work
