# Performance Improvement Plan

This document outlines performance optimizations for tmux-claude-go, focusing on runtime efficiency, goroutine management, memory leak prevention, and resource usage.

---

## Table of Contents

1. [Critical: Goroutine & Concurrency Issues](#1-critical-goroutine--concurrency-issues)
2. [High Priority: Redundant I/O Operations](#2-high-priority-redundant-io-operations)
3. [High Priority: Subprocess Overhead](#3-high-priority-subprocess-overhead)
4. [Medium Priority: Memory Allocation Patterns](#4-medium-priority-memory-allocation-patterns)
5. [Medium Priority: UI Performance](#5-medium-priority-ui-performance)
6. [Low Priority: Micro-optimizations](#6-low-priority-micro-optimizations)
7. [Implementation Roadmap](#7-implementation-roadmap)

---

## 1. Critical: Goroutine & Concurrency Issues

### 1.1 Unbounded Goroutine Spawning

**Location:** `internal/tmux/tmux.go:73-107`

**Problem:** Each pane spawns a goroutine without limit. With many panes (10+), this creates:
- Many concurrent `exec.Command` calls competing for OS resources
- Potential resource exhaustion under load
- No backpressure mechanism

**Current Code:**
```go
var wg sync.WaitGroup
for i, rp := range claudePanes {
    wg.Add(1)
    go func(idx int, rp rawPane) {
        defer wg.Done()
        // ... subprocess calls
    }(i, rp)
}
wg.Wait()
```

**Fix:** Use a worker pool with bounded concurrency:
```go
import "golang.org/x/sync/errgroup"

func ListClaudeWindows() ([]core.ClaudeWindow, error) {
    // ... parsing code ...

    enriched := make([]core.ClaudePane, len(claudePanes))
    var gitCache sync.Map

    g, _ := errgroup.WithContext(context.Background())
    g.SetLimit(8) // Cap concurrent workers

    for i, rp := range claudePanes {
        idx, rp := i, rp // capture loop vars
        g.Go(func() error {
            enriched[idx] = enrichPane(rp, &gitCache)
            return nil
        })
    }
    g.Wait()
    // ... grouping code ...
}
```

### 1.2 Potential Goroutine Leak in Preview Commands

**Location:** `internal/ui/model.go:683-695`

**Problem:** `previewCmd()` spawns a goroutine that executes `tmux capture-pane`. If the user rapidly navigates up/down, multiple preview goroutines are spawned concurrently. Results from stale previews are discarded, but goroutines still complete.

**Risk:** Not a memory leak per se, but wasteful under rapid navigation.

**Fix:** Implement cancellation via context or debouncing:
```go
// Option 1: Debounce preview requests
func (m *Model) previewCmdDebounced() tea.Cmd {
    return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
        return previewTickMsg(t)
    })
}

// Option 2: Track and cancel stale preview requests
type Model struct {
    // ...
    previewCancel context.CancelFunc
}

func (m *Model) previewCmd() tea.Cmd {
    if m.previewCancel != nil {
        m.previewCancel()
    }
    ctx, cancel := context.WithCancel(context.Background())
    m.previewCancel = cancel
    
    paneID := p.PaneID
    return func() tea.Msg {
        select {
        case <-ctx.Done():
            return nil // Cancelled, don't process
        default:
            content := tmux.CapturePaneContentCtx(ctx, paneID, 25)
            return core.PreviewMsg{Content: content, PaneID: paneID}
        }
    }
}
```

### 1.3 No Timeout on External Commands

**Location:** All `exec.Command` calls in `tmux/tmux.go`, `git/git.go`, `detect/detect.go`

**Problem:** No timeout protection. If tmux or git hangs (e.g., network git fetch), the entire application hangs.

**Fix:** Add context with timeout:
```go
func runWithTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, name, args...)
    return cmd.Output()
}

// Usage
out, err := runWithTimeout(5*time.Second, "git", "-C", path, "status", "--porcelain")
```

---

## 2. High Priority: Redundant I/O Operations

### 2.1 Triple ReadHookData Per Pane

**Location:** `internal/tmux/tmux.go:81-83` calling into `internal/detect/detect.go`

**Problem:** Each pane calls `StatusWithHook`, `CostWithHook`, and `ModelWithHook` separately. Each reads and parses the same JSON file:
```go
status := detect.StatusWithHook(rp.paneID, content)   // reads file
cost := detect.CostWithHook(rp.paneID, content)       // reads SAME file again
modelName := detect.ModelWithHook(rp.paneID, content) // reads SAME file AGAIN
```

**Impact:** 3x file reads per pane = 30 reads for 10 panes every 5 seconds.

**Fix:** Read once and pass HookData:
```go
// detect/detect.go - new function
func EnrichWithHook(paneID, content string) (status core.Status, cost float64, model string) {
    hd := ReadHookData(paneID)
    
    // Status (30s window)
    if hd != nil && time.Now().Unix()-hd.Timestamp <= 30 {
        status = parseHookStatus(hd.Status)
    } else {
        status = Status(content)
    }
    
    // Cost and model (60s window, already validated in ReadHookData)
    if hd != nil {
        cost = hd.Cost
        model = hd.Model
    }
    if cost == 0 {
        cost = ParseCost(content)
    }
    if model == "" {
        model = ParseModel(content)
    }
    
    return status, cost, model
}

// tmux/tmux.go - usage
status, cost, modelName := detect.EnrichWithHook(rp.paneID, content)
```

### 2.2 Uncached statusDir() Calls

**Location:** `internal/detect/detect.go:40-46`

**Problem:** `statusDir()` calls `os.UserHomeDir()` on every invocation. `ReadHookData` is called per pane, per refresh.

**Current Code:**
```go
func statusDir() string {
    home, err := os.UserHomeDir()  // syscall every time
    if err != nil {
        return ""
    }
    return filepath.Join(home, ".claude-tmux")
}
```

**Fix:** Cache the directory path:
```go
var (
    statusDirPath     string
    statusDirPathOnce sync.Once
)

func statusDir() string {
    statusDirPathOnce.Do(func() {
        home := state.CachedHomeDir() // reuse existing cache
        if home != "" {
            statusDirPath = filepath.Join(home, ".claude-tmux")
        }
    })
    return statusDirPath
}
```

### 2.3 ExpandTilde Uses Uncached UserHomeDir

**Location:** `internal/state/util.go:40-49`

**Problem:** `ExpandTilde` calls `os.UserHomeDir()` instead of using `CachedHomeDir()`.

**Fix:**
```go
func ExpandTilde(path string) string {
    if !strings.HasPrefix(path, "~") {
        return path
    }
    home := CachedHomeDir() // Use cached version
    if home == "" {
        return path
    }
    return filepath.Join(home, path[1:])
}
```

---

## 3. High Priority: Subprocess Overhead

### 3.1 Multiple Git Subprocesses in DetectInfo

**Location:** `internal/git/git.go:22-67`

**Problem:** 4-5 sequential subprocess calls per directory:
1. `git rev-parse --is-inside-work-tree`
2. `git branch --show-current`
3. `git status --porcelain`
4. `git rev-parse --git-common-dir`
5. `git rev-parse --git-dir`

Each subprocess has ~2-10ms overhead for fork/exec.

**Fix Option A - Combine rev-parse calls:**
```go
func DetectInfo(path string) Info {
    info := Info{}
    
    // Single call for multiple rev-parse queries
    out, err := exec.Command("git", "-C", path, "rev-parse",
        "--is-inside-work-tree",
        "--git-dir",
        "--git-common-dir",
    ).Output()
    if err != nil {
        return info
    }
    
    lines := strings.Split(strings.TrimSpace(string(out)), "\n")
    if len(lines) < 3 || lines[0] != "true" {
        return info
    }
    info.HasGit = true
    info.IsWorktree = lines[1] != lines[2]
    
    // Combine status -sb (includes branch)
    out, err = exec.Command("git", "-C", path, "status", "-sb", "--porcelain=v2").Output()
    // ... parse branch and status from single output
}
```

**Fix Option B - Use go-git library:**
```go
import "github.com/go-git/go-git/v5"

func DetectInfo(path string) Info {
    repo, err := git.PlainOpen(path)
    if err != nil {
        return Info{}
    }
    
    head, _ := repo.Head()
    worktree, _ := repo.Worktree()
    status, _ := worktree.Status()
    
    return Info{
        HasGit:    true,
        Branch:    head.Name().Short(),
        Dirty:     !status.IsClean(),
        // ... etc
    }
}
```

Note: go-git adds dependency but eliminates all subprocess overhead.

### 3.2 Preview Capture on Every Selection Change

**Location:** `internal/ui/model.go:235-236, 683-695`

**Problem:** Every up/down keystroke spawns a `tmux capture-pane` subprocess.

**Fix:** Debounce preview updates:
```go
// Add to Model
type Model struct {
    // ...
    previewDebounceTimer *time.Timer
    previewPending       string // paneID waiting for preview
}

func (m *Model) handleSessionListKey(key string) (tea.Model, tea.Cmd) {
    switch key {
    case "j", "down":
        m.scroll.MoveDown()
        m.saveSelection()
        return m, m.schedulePreview() // debounced
    // ...
    }
}

func (m *Model) schedulePreview() tea.Cmd {
    _, p := m.selectedWindowAndPane()
    if p == nil {
        return nil
    }
    
    // Return a tick that will trigger preview after delay
    return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
        return previewTriggerMsg{paneID: p.PaneID}
    })
}

// In Update, handle previewTriggerMsg and only fetch if paneID matches current selection
```

---

## 4. Medium Priority: Memory Allocation Patterns

### 4.1 Regex Compiled Inside ParseCost

**Location:** `internal/detect/detect.go:247-249`

**Problem:** Regex compiled on every call:
```go
func ParseCost(content string) float64 {
    // ...
    dollarRegex := regexp.MustCompile(`\$([0-9]+\.[0-9]{2,4})`) // compiled every call
}
```

**Fix:** Move to package-level:
```go
var (
    // ... existing regexes
    dollarRegex = regexp.MustCompile(`\$([0-9]+\.[0-9]{2,4})`)
)

func ParseCost(content string) float64 {
    // Use dollarRegex directly
    dollarMatches := dollarRegex.FindAllStringSubmatch(content, -1)
}
```

### 4.2 rebuildItems Allocates New Slice Every Time

**Location:** `internal/ui/model.go:573-601`

**Problem:** Creates new slice on every rebuild (every refresh, filter, sort):
```go
func (m *Model) rebuildItems() {
    var items []core.ListItem  // new slice every time
    // ... append items
    m.items = items
}
```

**Fix:** Reuse existing slice capacity:
```go
func (m *Model) rebuildItems() {
    // Reuse slice, reset length
    m.items = m.items[:0]
    
    indices := m.filtered
    if indices == nil {
        indices = make([]int, len(m.windows))
        for i := range m.windows {
            indices[i] = i
        }
    }
    
    for _, i := range indices {
        if i >= len(m.windows) {
            continue
        }
        w := m.windows[i]
        m.items = append(m.items, core.ListItem{WindowIdx: i, PaneIdx: -1})
        // ... rest
    }
    
    m.scroll.SetTotal(len(m.items))
}
```

### 4.3 applyFilter Creates New Lowercase Strings Repeatedly

**Location:** `internal/ui/model.go:745-776`

**Problem:** `strings.ToLower()` called multiple times per window per keystroke:
```go
func (m *Model) applyFilter() {
    filterLower := strings.ToLower(m.filterText)
    for i, w := range m.windows {
        if strings.Contains(strings.ToLower(w.SessionName), filterLower) ||
           strings.Contains(strings.ToLower(w.WindowName), filterLower) ||
           // ... more ToLower calls
    }
}
```

**Fix:** Pre-compute lowercase searchable text:
```go
// In core/types.go
type ClaudeWindow struct {
    // ... existing fields
    searchText string // precomputed lowercase "session window path branch status"
}

// Set during window creation/enrichment
func (w *ClaudeWindow) ComputeSearchText() {
    var parts []string
    parts = append(parts, strings.ToLower(w.SessionName), strings.ToLower(w.WindowName))
    for _, p := range w.Panes {
        parts = append(parts, strings.ToLower(p.WorkingDir), strings.ToLower(p.GitBranch))
    }
    w.searchText = strings.Join(parts, " ")
}

// In applyFilter - simple contains on precomputed text
func (m *Model) applyFilter() {
    if m.filterText == "" {
        m.filtered = nil
        m.rebuildItems()
        return
    }
    
    filterLower := strings.ToLower(m.filterText)
    m.filtered = nil
    for i, w := range m.windows {
        if strings.Contains(w.searchText, filterLower) {
            m.filtered = append(m.filtered, i)
        }
    }
    m.rebuildItems()
}
```

---

## 5. Medium Priority: UI Performance

### 5.1 Live Filter on Every Keystroke

**Location:** `internal/ui/model.go:434-439`

**Problem:** Filter recalculates on every keystroke, causing UI lag with many sessions.

**Fix:** Debounce filter application:
```go
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
    switch msg.String() {
    case "esc":
        // ... existing
    case "enter":
        // Apply filter immediately on enter
        m.filterText = m.textInput.Value()
        m.applyFilter()
        m.mode = core.ModeSessionList
        return m, nil
    }
    
    var cmd tea.Cmd
    m.textInput, cmd = m.textInput.Update(msg)
    
    // Debounce: schedule filter application after 100ms
    return m, tea.Batch(cmd, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
        return filterTriggerMsg{text: m.textInput.Value()}
    }))
}

// In Update:
case filterTriggerMsg:
    if msg.text == m.textInput.Value() { // Only apply if text hasn't changed
        m.filterText = msg.text
        m.applyFilter()
    }
```

### 5.2 View Rendering Can Be Optimized

**Location:** `internal/ui/views.go` (not shown but exists)

**Recommendation:** Profile view rendering if performance issues arise. Consider:
- Caching rendered strings that don't change between frames
- Using `strings.Builder` for concatenation
- Avoiding lipgloss style recomputation

---

## 6. Low Priority: Micro-optimizations

### 6.1 String Slice Allocation in CapturePaneContent

**Location:** `internal/tmux/tmux.go:154`

**Problem:** `strings.Split` creates a new slice with many elements.

**Possible optimization:** Use `bytes.Split` on the raw output to avoid string conversion overhead. Marginal gain.

### 6.2 status-hook.sh Multiple Tool Invocations

**Location:** `scripts/status-hook.sh:60-71`

**Problem:** Multiple `grep`, `sed`, `awk`, `bc` calls for transcript parsing.

**Fix:** Use a single `jq` invocation or move logic to Go entirely. Lower priority since hook runs asynchronously.

---

## 7. Implementation Roadmap

### Phase 1: Quick Wins (1-2 hours)
- [ ] Fix triple `ReadHookData` calls → single `EnrichWithHook` function
- [ ] Cache `statusDir()` path
- [ ] Use `CachedHomeDir()` in `ExpandTilde`
- [ ] Move `dollarRegex` to package level

### Phase 2: Concurrency Safety (2-3 hours)
- [ ] Add worker pool with `errgroup.SetLimit(8)` in `ListClaudeWindows`
- [ ] Add context timeout wrapper for all `exec.Command` calls
- [ ] Implement preview debouncing (150ms delay)

### Phase 3: Reduce Subprocess Overhead (2-4 hours)
- [ ] Combine git rev-parse calls
- [ ] Consider `git status -sb --porcelain=v2` for branch + status in one call
- [ ] Evaluate go-git dependency tradeoff

### Phase 4: Memory & Allocation (1-2 hours)
- [ ] Reuse `items` slice in `rebuildItems`
- [ ] Pre-compute lowercase search text for windows
- [ ] Debounce filter application

### Phase 5: Observability (Optional)
- [ ] Add pprof endpoints for debugging
- [ ] Add metrics for refresh duration
- [ ] Log goroutine count periodically in debug mode

---

## Metrics to Track

After implementing optimizations, measure:

1. **Refresh latency**: Time from `refreshWindowsCmd()` start to `WindowsMsg` received
2. **Memory usage**: `runtime.MemStats` before/after refresh cycles
3. **Goroutine count**: `runtime.NumGoroutine()` should remain stable
4. **File descriptor count**: `lsof -p <pid> | wc -l` should remain stable
5. **Subprocess count**: Profile with `strace` or similar

---

## Testing Recommendations

1. **Load test**: Create 20+ Claude sessions and verify stability
2. **Rapid navigation test**: Hold down j/k and verify no goroutine accumulation
3. **Memory leak test**: Run for 30+ minutes with periodic `pprof` snapshots
4. **Stress test**: Rapidly filter/unfilter while refresh is happening
