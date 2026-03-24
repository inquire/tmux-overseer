# 🔥 tmux-overseer

A TUI for managing **Claude Code** and **Cursor IDE** sessions from a single view. See status, costs, model info, and git context at a glance. Switch sessions, send input, manage git, browse plans, and track activity — without leaving your workflow.

```
 ▐▛███▜▌             ╭─ SESSIONS ──── PLANS ── ACTIVITY ─╮
▝▜█████▛▘ ── tmux-overseer ──
  ▘▘ ▝▝
  ▸ my-project:0             [CLAUDE] [2]  ● working     $3.41
    ~/go/src/my-project       opus 4.6  (better-load-detection)  +*

› tmux-overseer               [CURSOR]      ○ idle        $1.24
  ~/go/src/tmux-overseer      opus 4.6  (main)

  ▸ work:debug                [CLAUDE] [3]  ◐ waiting     $0.22
    ~/go/src/api-service       sonnet 4  (fix/timeout)  *
────────────────────────────────────────────────────────────
  [13:21:05] prompt submitted
  [13:21:06] using tool: Shell
  [13:21:08] finished tool: Shell
────────────────────────────────────────────────────────────
 2 cli  1 cursor  ● 1 working  ◐ 1 waiting  $4.87 today
 ↑↓/🖱 navigate  l/→ actions  enter/dblclick switch  f filter src  q quit
```

> [!WARNING]
> **This project is under heavy active development.** Features may break or behave unexpectedly due to frequent API and hook changes in both Claude Code and Cursor IDE. Hook event names, JSON payloads, and configuration formats can change between releases without notice. If something stops working, check the hook setup scripts and re-run them against the latest version.

## ✨ Features

### 🖥️ Session Management
- 🔍 **Multi-source discovery** — finds Claude Code instances across tmux + Cursor IDE + Cloud sessions in one unified list
- 🏷️ **Source badges** — `[CLAUDE]` (orange), `[CURSOR]` (purple), `[CLOUD]` (blue) badges distinguish session types
- 🔀 **Source filter** — press `f` to cycle between All / Claude / Cursor / Cloud views
- 📑 **Section headers** — sessions are grouped under styled headers when filtered
- 🔗 **Smart switching** — `enter` switches to tmux pane for CLI sessions, opens Cursor deeplink for IDE sessions
- 📋 **Action menu** — switch, send input, rename, git operations, kill — all from one place
- 🆕 **Create sessions** — press `n` to create a new tmux session with Claude automatically started
- 📂 **Workspace groups** — Cursor sessions sharing the same workspace are grouped together (collapsible with `tab`)
- 🖥️ **Cursor actions** — open Cursor workspace in terminal, copy workspace path, or end session tracking
- 🤖 **Subagent display** — active subagents (explore, shell, browser, code-reviewer, etc.) shown per session with distinct icons, expandable with `tab`

### 📋 Plans Browser
- 📋 **Plans view** — press `p` to browse all Cursor plans and Claude Code conversations
- 🟠🟣 **Source badges** — emoji badges distinguish Cursor plans (🟣) from Claude conversations (🟠)
- 🟩⬜ **Progress bars** — visual TODO completion progress per plan
- 📂 **Workspace / day grouping** — group plans by workspace path or by day (`g` to cycle)
- 🔍 **Filtering** — filter by source (`f`), hide completed plans (`c`), or text search (`/`)
- ✅ **Multi-select** — `Shift+J/K` for range selection, `space` to toggle individual items
- 🗑️ **Bulk delete** — press `d` to delete the selected plan(s) with confirmation
- ✨ **Title generation** — press `t` to auto-generate a title for a plan via Claude
- ▶️ **Resume** — `enter` on a Cursor plan opens it in Cursor plan mode; Claude conversations resume via `claude --resume`
- 📅 **Relative dates** — "just now", "2 hours ago", "yesterday", "Feb 15"
- 🔄 **DB sync** — press `S` to force a full DuckDB resync of all plans

### 📅 Activity View
- 📅 **Activity view** — press `a` from any view to open the activity dashboard
- 🟩 **Contribution heatmap** — 26-week calendar (Sun–Sat rows) with colour intensity by composite score
- ⬅️ **Day navigation** — `←/→` to move between days, `↑/↓` to scroll projects
- 🏆 **Project summaries** — top 8 projects ranked by score, with plan/todo counts and progress bars
- 📊 **Day detail** — plans touched and todos completed for the selected day, broken down by project
- ⚖️ **Scoring** — plan created (×3), todo completed (×2), plan modified (×1), conversation started (×1)

### 🚦 Status & Detection
- 🚦 **Status detection** — idle, working, waiting for input, with the Claude flower spinner (`· ✻ ✽`)
- 🎯 **Hook-based status** — optional hooks for both Claude Code and Cursor IDE for real-time accuracy
- 🔄 **Aggregate status** — multi-pane windows show the "worst case" status (working > waiting > idle)
- 🤖 **Model display** — shows the active model (e.g. `opus 4.6`, `sonnet 4`) on each session row
- ⏱️ **Auto-refresh** — sessions and statuses update every 5 seconds

### 💰 Cost Tracking
- 💰 **Always-on cost** — per-session cost displayed at all times (even `$0.00`)
- 📒 **Daily cost ledger** — costs persisted to `~/.claude-tmux/costs-YYYY-MM-DD.jsonl`
- 📊 **Day total** — status bar shows cumulative cost across all sessions for the day, including finished ones
- 💾 **Cost persistence** — if a session restarts or cost disappears from terminal, the last known value is preserved

### 👀 Preview
- 👀 **Live preview** — see the last 10 lines of any Claude CLI session's terminal output in full colour
- 📜 **Cursor activity log** — Cursor sessions show a timestamped activity feed (tools used, prompts, stops)

### 🌿 Git Integration
- 🌿 **Branch info** — branch name, staged/unstaged indicators on every session row
- 🌳 **Worktree support** — create/delete git worktrees with associated sessions
- 📥 **Git actions** — stage all, commit, push, fetch — directly from the action menu

### ☁️ Cloud Agents
- ☁️ **Cloud session detection** — sessions handed off to cloud agents appear as `[CLOUD]` entries
- 📡 **Local handoffs** — detected via `cloud-handoffs.jsonl` (prompts prefixed with `&`)
- 🔌 **API integration** — when `CURSOR_API_KEY` is set, live agent status is fetched from `api.cursor.com`
- 🔗 **Links** — cloud sessions show PR and agent URLs in the action menu

### 🦆 Data Persistence (DuckDB)
- 🗄️ **Local database** — all plan and activity data is stored in `~/.claude-tmux/plans.duckdb`
- 🔄 **Automatic sync** — plan scanner diffs and syncs to the DB on each refresh
- 📝 **Activity events** — every plan creation, modification, and todo completion is recorded for the activity view
- ⚡ **Plans cache** — plans are cached to disk with a 60-second TTL (stale-while-revalidate)

### 🖱️ Navigation & UI
- 🖱️ **Mouse support** — click to select, double-click to switch, scroll wheel to navigate, click status bar to cycle sort
- 🎯 **Center-locked scrolling** — selected item stays centred in the viewport with scroll indicators
- 🔎 **Filter** — live search across session names, paths, branches, and status (`/` to start)
- 🔀 **Sort modes** — cycle between name, status, and recency (`s`)
- 💾 **Persistent selection** — remembers which session you last visited across restarts
- 📦 **Auto-expand** — all multi-pane sessions are expanded by default

### ⚡ Performance
- 🚀 **Instant startup** — previous session data is cached to disk so the UI renders immediately, then refreshes in the background
- ⚡ **Parallel loading** — tmux and Cursor sessions are discovered concurrently for faster refresh
- 🗂️ **Git TTL cache** — git info per directory is cached for 10 seconds to avoid redundant subprocess calls
- 🎛️ **Debounced preview** — prevents subprocess spam during rapid navigation

## 📦 Installation

### Option 1: Go install (recommended)

```bash
go install github.com/inquire/tmux-overseer/cmd/tmux-overseer@latest
```

### Option 2: Build from source with Make

```bash
git clone https://github.com/inquire/tmux-overseer.git
cd tmux-overseer
make build
sudo cp tmux-overseer /usr/local/bin/
```

### Option 3: Build manually

```bash
git clone https://github.com/inquire/tmux-overseer.git
cd tmux-overseer
go build -o tmux-overseer ./cmd/tmux-overseer

# Install to /usr/local/bin (or anywhere in $PATH)
sudo cp tmux-overseer /usr/local/bin/
```

### Makefile targets

| Target | Description |
|--------|-------------|
| `make build` | Build the `tmux-overseer` binary |
| `make install` | Install `tmux-overseer` via `go install` |
| `make hook` | Build the `claude-hook` Go binary |
| `make install-hook` | Build and install `claude-hook` to `$GOPATH/bin` |
| `make install-all` | Install both `tmux-overseer` and `claude-hook` |
| `make test` | Run unit tests |
| `make test-race` | Run tests with the race detector |
| `make lint` | Run `golangci-lint` (requires `brew install golangci-lint`) |
| `make lint-fix` | Run linter with auto-fix |
| `make vet` | Run `go vet` |
| `make fmt` | Format with `gofmt` + `goimports` |
| `make check` | Run `fmt`, `vet`, `lint`, and `test` together |
| `make run` | Build and run in the current terminal |
| `make popup` | Build and open in a tmux popup (90%×90%) |
| `make clean` | Remove build artifacts |

## 🛠️ Tmux Setup

### 1️⃣ Add the keybinding

Add this to your `~/.tmux.conf`:

```tmux
# Open tmux-overseer popup with Ctrl-b, Ctrl-o
bind C-o display-popup -E -w 80% -h 80% -T " tmux-overseer " tmux-overseer
```

### 2️⃣ Reload tmux config

```bash
tmux source-file ~/.tmux.conf
```

### 3️⃣ Use it! 🎉

Press **`Ctrl-b`** then **`Ctrl-o`** from anywhere inside tmux. A popup will appear showing all your sessions.

> 💡 **Tip:** You can also run `tmux-overseer` directly in a terminal — it just works better as a tmux popup since it can switch you into sessions.

### 🎨 Alternative keybinding ideas

```tmux
# Larger popup
bind C-o display-popup -E -w 90% -h 90% -T " tmux-overseer " tmux-overseer

# Fixed size popup
bind C-o display-popup -E -w 100 -h 30 -T " tmux-overseer " tmux-overseer
```

## 🎯 Hook Setup (Recommended)

Hooks provide **real-time, accurate** status detection instead of relying on terminal output parsing. The hook system uses an internal HTTP server — when the TUI is running, hooks forward events to it automatically with no extra configuration.

> [!CAUTION]
> Hook event names and JSON payloads change frequently between Claude Code and Cursor IDE releases. If status detection stops working after an update, re-run the relevant setup script to re-register hooks with the current event schema.

### 🤖 Claude Code Hooks

> [!NOTE]
> **Requires `jq`** — install with `brew install jq` if not already present.

```bash
./scripts/setup-hooks.sh
```

This adds hooks to `~/.claude/settings.json` for all Claude Code lifecycle events.

The setup script will **prefer the compiled Go hook binary** (`claude-hook`) if available — it's ~6× faster than the bash fallback. Install it with:

```bash
make install-hook
```

| Event | Status | Description |
|-------|--------|-------------|
| `SessionStart` | 🟢 idle | New Claude Code session opened |
| `SessionEnd` | — | Session closed; cleans up state |
| `UserPromptSubmit` | 🟠 working | Processing your input |
| `Stop` | 🟢 idle | Claude finished responding |
| `PreToolUse` | 🟡 waiting | Claude needs permission for a tool |
| `PostToolUse` | 🟠 working | Tool execution completed |
| `PostToolUseFailure` | 🟠 working | Tool execution failed |
| `SubagentStart` | 🟠 working | Subagent launched |
| `SubagentStop` | 🟠 working | Subagent completed |
| `Notification` | 🟡 waiting | Claude sent a notification |
| `PreCompact` | 🟠 working | Context compaction in progress |
| `TaskCompleted` | 🟢 idle | Task finished |
| `TeammateIdle` | 🟢 idle | Teammate session went idle |
| `InstructionsLoaded` | — | Instructions loaded into context |

The hook also parses cost from the session transcript and detects the active model on each event.

> [!NOTE]
> Restart Claude Code after running the setup script for hooks to take effect. To ensure `SessionEnd` cleanup always completes, add this to your shell environment:
> ```bash
> export CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS=8000
> ```

### 🖱️ Cursor IDE Hooks

> [!NOTE]
> **Requires `jq`** — install with `brew install jq` if not already present.

```bash
./scripts/setup-cursor-hooks.sh
```

This copies `cursor-status-hook.sh` to `~/.cursor/hooks/` and registers it in `~/.cursor/hooks.json` for all Cursor agent lifecycle events.

| Event | Status | Description |
|-------|--------|-------------|
| `sessionStart` | 🟢 idle | Session opened |
| `sessionEnd` | — | Session closed; removes state file |
| `beforeSubmitPrompt` | 🟠 working | User submitted a prompt |
| `stop` | 🟢 idle | Claude finished responding |
| `preToolUse` | 🟠 working | Tool about to execute |
| `postToolUse` | 🟠 working | Tool execution completed |
| `subagentStart` | 🟠 working | Subagent launched |
| `subagentStop` | 🟠 working | Subagent completed |
| `preCompact` | 🟠 working | Context compaction in progress |
| `afterAgentResponse` | 🟠 working | Agent produced a response |
| `afterAgentThought` | 🟠 working | Agent reasoning step completed |
| `afterFileEdit` | 🟠 working | File was edited by agent |
| `afterShellExecution` | 🟠 working | Shell command executed |

Each event appends to an activity log (`~/.claude-tmux/cursor-{id}.log`) which powers the preview pane for Cursor sessions. Status files are written to `~/.claude-tmux/cursor-{id}.json`.

> [!NOTE]
> You may need to restart Cursor after running the setup script.

## ⌨️ Keybindings

### 📋 Session List

| Key | Action |
|-----|--------|
| `↑/k` or scroll | Move up |
| `↓/j` or scroll | Move down |
| `l/→` | Open action menu |
| `enter` or double-click | Switch to session/pane (or expand a collapsed group) |
| click | Select session |
| `tab` | Expand/collapse multi-pane windows, plan todos, or subagents |
| `f` | Cycle source filter (All → Claude → Cursor → Cloud) |
| `n` | New session (creates tmux session + starts Claude) |
| `i` | Send input (shortcut) |
| `d` | Kill session (shortcut) |
| `s` | Cycle sort mode (name → status → recency) |
| `/` | Filter (live search as you type) |
| `R` | Force refresh |
| `p` | Open plans browser |
| `a` | Open activity view |
| `?` | Help overlay |
| `q/esc` | Quit |

### 📋 Plans Browser (`p`)

| Key | Action |
|-----|--------|
| `↑/k` | Move up |
| `↓/j` | Move down |
| `Shift+J` / `Shift+K` | Extend selection down / up (range select) |
| `space` | Toggle item selection |
| `tab` | Expand/collapse workspace group |
| `enter` | Resume plan (opens Cursor plan mode or `claude --resume`) |
| `d` | Delete selected plan(s) |
| `f` | Cycle source filter (All → Claude → Cursor) |
| `c` | Toggle show/hide completed plans |
| `g` | Cycle group mode (workspace → day) |
| `t` | Generate title for selected plan |
| `/` | Filter plans by text |
| `S` | Force full DuckDB resync |
| `R` | Reload plans |
| `a` | Open activity view |
| `p/q/esc` | Back to session list |

### 📅 Activity View (`a`)

| Key | Action |
|-----|--------|
| `←/→` | Navigate between days |
| `↑/↓` | Scroll project list |
| `a/p/q/esc` | Back to session list |

### 📝 Action Menu (`l/→`)

| Key | Action |
|-----|--------|
| `↑/k` | Previous action |
| `↓/j` | Next action |
| `enter/l/→` | Execute action |
| `h/←/esc` | Back to session list |

### 🎬 Actions Available

**🖥️ CLI (tmux) sessions:**

| Action | Description |
|--------|-------------|
| 🔗 Switch to session | Jump into tmux pane |
| 💬 Send input | Type a message and send it to Claude |
| ✏️ Rename session | Rename the tmux session |
| 📥 Stage all changes | `git add -A` |
| 💾 Commit | Commit with a message |
| 🚀 Push | Push to remote (auto-sets upstream) |
| 📡 Fetch | Fetch from remote |
| 🌳 New worktree | Create a git worktree with a new session |
| 💀 Kill session | Terminate the tmux session |

**🖱️ Cursor IDE sessions:**

| Action | Description |
|--------|-------------|
| 🔗 Switch to session | Open Cursor via deeplink |
| 📥 Stage all changes | `git add -A` |
| 💾 Commit | Commit with a message |
| 🚀 Push | Push to remote (auto-sets upstream) |
| 📡 Fetch | Fetch from remote |
| 🖥️ Open in terminal | Create a new tmux session with Claude in the workspace |
| 📋 Copy path | Copy workspace path to clipboard |
| 🛑 End session | Stop tracking the Cursor session |

## 🚦 Status Indicators

| Symbol | Status | Color |
|--------|--------|-------|
| `○` | Idle — ready for input | 🟢 Green |
| `· ✻ ✽` | Working — animated flower spinner | 🟠 Orange |
| `◐` | Waiting — needs your attention | 🟡 Yellow |
| `?` | Unknown | ⚪ Gray |

## 🔧 How It Works

### 🖥️ Claude Code (CLI) Sessions

The tool scans all tmux sessions for panes running a `claude` command. For each pane:

1. 📸 Captures visible output via `tmux capture-pane`
2. 🎯 Detects status using **Claude Code hooks** (most accurate) or terminal pattern analysis (fallback)
3. 💰 Parses cost and model info from Claude's status line
4. 🌿 Gathers git info via `git` on the pane's working directory

### 🖱️ Cursor IDE Sessions

Cursor sessions are detected via a hook script that Cursor invokes on agent lifecycle events:

1. 🖥️ Hook script writes status files to `~/.claude-tmux/cursor-{id}.json` on each event
2. 📜 Activity log appended to `~/.claude-tmux/cursor-{id}.log` (tool usage, prompts, stops)
3. 🧹 Stale sessions auto-cleaned after 20 minutes of inactivity
4. 🔗 Switching opens Cursor via `cursor://file/{path}` deeplink

### ☁️ Cloud Agents

Cloud agent sessions are detected from two sources:

1. 📄 Local `cloud-handoffs.jsonl` — written by the Cursor hook when a prompt starts with `&`
2. 🌐 `api.cursor.com/v0/agents` — polled every 30 seconds when `CURSOR_API_KEY` is configured

### 📅 Activity Tracking

All plan activity is recorded in a local DuckDB database (`~/.claude-tmux/plans.duckdb`). On each plan sync the database records creates, modifications, and todo completions as timestamped events. The activity view aggregates these into a weekly heatmap and per-project summaries.

### 💰 Cost Persistence

Costs are tracked in a daily JSONL ledger (`~/.claude-tmux/costs-YYYY-MM-DD.jsonl`). Each cost update is appended with a high-water mark per session, so costs survive session restarts and terminal scrolling. The status bar shows the cumulative day total including finished sessions.

Everything runs locally — no API keys or network calls needed (unless Cloud agent API polling is enabled). 🔒

> 📖 For a deep dive into the architecture, see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## 📁 Project Structure

```
tmux-overseer/
├── cmd/
│   └── tmux-overseer/
│       └── main.go                  # Entry point
├── internal/
│   ├── core/
│   │   ├── types.go                 # Domain types (ClaudeWindow, PlanEntry, Subagent, etc.)
│   │   ├── types_ui.go              # View modes, sort modes, session actions
│   │   ├── types_messages.go        # Bubble Tea message types
│   │   └── styles.go                # Colours, mascot, spinner, lipgloss styles
│   ├── db/
│   │   ├── sessions.go              # DuckDB schema + plan/activity queries
│   │   └── sync.go                  # Plan diffing and sync to DuckDB
│   ├── detect/
│   │   ├── detect.go                # Claude CLI detection + hook-aware status
│   │   ├── detect_test.go           # Detection tests
│   │   ├── cursor.go                # Cursor IDE session discovery + activity logs
│   │   ├── cloud.go                 # Cloud agent detection (local + API)
│   │   └── costs.go                 # Daily cost ledger (JSONL persistence + TTL cache)
│   ├── exec/
│   │   └── exec.go                  # Subprocess execution with timeouts
│   ├── git/
│   │   ├── info.go                  # Git detection with cross-refresh TTL cache
│   │   └── ops.go                   # Git commands (stage, commit, push, fetch, worktree)
│   ├── plans/
│   │   ├── plans.go                 # Plan scanner (merges + parallelises both sources)
│   │   ├── cursor_plans.go          # Cursor .plan.md YAML parser + workspace resolution
│   │   ├── claude_convos.go         # Claude Code JSONL conversation scanner
│   │   └── titles.go                # Title generation via `claude -p` + override persistence
│   ├── state/
│   │   ├── scroll.go                # Centre-locked scroll state
│   │   ├── scroll_test.go           # Scroll tests
│   │   ├── cache.go                 # Disk caches for sessions and plans
│   │   └── util.go                  # Path helpers, selection persistence
│   ├── tmux/
│   │   ├── list.go                  # Session discovery (parallel tmux + Cursor + Cloud loading)
│   │   ├── capture.go               # Pane content capture
│   │   ├── session.go               # Session creation (new, resume, with command)
│   │   └── switch.go                # Session switching (tmux + Cursor deeplinks)
│   └── ui/
│       ├── model.go                 # Bubble Tea model, Init, Update, View
│       ├── model_sessions.go        # Session list key handling, sort, filter, items
│       ├── model_actions.go         # Action menu, dialog handlers (rename, commit, etc.)
│       ├── model_plans.go           # Plans view logic, grouping, multi-select, delete
│       ├── model_activity.go        # Activity view logic, heatmap, project summaries
│       ├── model_preview.go         # Preview pane with debouncing and cancellation
│       ├── model_commands.go        # Async commands (refresh, costs, clipboard)
│       ├── model_mouse.go           # Mouse click handling
│       ├── views.go                 # Top-level view dispatch
│       ├── views_layout.go          # Main layout (header, list, preview, status bar)
│       ├── views_sessions.go        # Session row rendering
│       ├── views_plans.go           # Plans view rendering (groups, progress bars)
│       ├── views_activity.go        # Activity view rendering (heatmap, project bars)
│       ├── views_dialogs.go         # Dialog overlays (action menu, confirm, input)
│       └── views_helpers.go         # Shared rendering utilities
├── scripts/
│   ├── status-hook.sh               # Claude Code hook for status/cost/model/subagents
│   ├── setup-hooks.sh               # Auto-configure Claude Code hooks
│   ├── cursor-status-hook.sh        # Cursor IDE hook for status + activity log
│   └── setup-cursor-hooks.sh        # Auto-configure Cursor IDE hooks
├── docs/
│   └── ARCHITECTURE.md              # Internal architecture reference
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. 🍴 Fork the repository
2. 🌿 Create your feature branch (`git checkout -b feature/amazing-feature`)
3. 💾 Commit your changes (`git commit -m 'Add some amazing feature'`)
4. 🚀 Push to the branch (`git push origin feature/amazing-feature`)
5. 📝 Open a Pull Request

## 📜 License

MIT
