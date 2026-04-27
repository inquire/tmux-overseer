package tmux

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/detect"
	"github.com/inquire/tmux-overseer/internal/exec"
	"github.com/inquire/tmux-overseer/internal/git"
	"github.com/inquire/tmux-overseer/internal/plans"
)

// rawPane holds parsed tmux pane info before enrichment.
type rawPane struct {
	sessionName string
	createdAt   int64
	attached    bool
	paneID      string
	cmd         string
	path        string
	winIdx      int
	winName     string
}

// ListClaudeWindows discovers all Claude instances across tmux sessions.
// Uses a single tmux call + parallel enrichment for performance.
func ListClaudeWindows() ([]core.ClaudeWindow, error) {
	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "tmux", "list-panes", "-a", "-F",
		"#{session_name}\t#{session_created}\t#{session_attached}\t#{pane_id}\t#{pane_current_command}\t#{pane_current_path}\t#{window_index}\t#{window_name}")
	if err != nil {
		return nil, fmt.Errorf("tmux not running or no sessions")
	}

	var claudePanes []rawPane
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 8 {
			continue
		}
		if !detect.IsClaudeCommand(parts[4]) {
			continue
		}
		createdAt, _ := strconv.ParseInt(parts[1], 10, 64)
		winIdx, _ := strconv.Atoi(parts[6])
		claudePanes = append(claudePanes, rawPane{
			sessionName: parts[0],
			createdAt:   createdAt,
			attached:    parts[2] == "1",
			paneID:      parts[3],
			cmd:         parts[4],
			path:        parts[5],
			winIdx:      winIdx,
			winName:     parts[7],
		})
	}

	if len(claudePanes) == 0 {
		return nil, nil
	}

	// Parallel enrichment with bounded worker pool.
	type enrichedPane struct {
		pane     core.ClaudePane
		hookData *detect.HookData
		gitInfo  git.Info
	}
	enrichedPanes := make([]enrichedPane, len(claudePanes))

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(8)

	for i, rp := range claudePanes {
		idx, rp := i, rp
		g.Go(func() error {
			content := CapturePaneContent(rp.paneID, 15)
			status, cost, modelName, lastTool := detect.EnrichWithHook(rp.paneID, content)
			gitInfo := git.DetectInfoCached(rp.path)

			hd := detect.ReadHookData(rp.paneID)

			sandboxType := detect.SandboxTypeFromCommand(rp.cmd)
			if hd != nil && hd.SandboxType != "" {
				sandboxType = hd.SandboxType
			}

			enrichedPanes[idx] = enrichedPane{
				pane: core.ClaudePane{
					PaneID:      rp.paneID,
					Status:      status,
					WorkingDir:  rp.path,
					GitBranch:   gitInfo.Branch,
					GitDirty:    gitInfo.Dirty,
					GitStaged:   gitInfo.Staged,
					IsWorktree:  gitInfo.IsWorktree,
					HasGit:      gitInfo.HasGit,
					Cost:        cost,
					Model:       modelName,
					LastTool:    lastTool,
					SandboxType: sandboxType,
				},
				hookData: hd,
				gitInfo:  gitInfo,
			}
			return nil
		})
	}
	_ = g.Wait()

	// Group enriched panes into windows.
	windowMap := make(map[string]*core.ClaudeWindow)
	for i, rp := range claudePanes {
		key := fmt.Sprintf("%s:%d", rp.sessionName, rp.winIdx)
		if w, ok := windowMap[key]; ok {
			w.Panes = append(w.Panes, enrichedPanes[i].pane)
		} else {
			windowMap[key] = &core.ClaudeWindow{
				SessionName: rp.sessionName,
				WindowIndex: rp.winIdx,
				WindowName:  rp.winName,
				Panes:       []core.ClaudePane{enrichedPanes[i].pane},
				Attached:    rp.attached,
				CreatedAt:   rp.createdAt,
			}
		}
		// Enrich window from hook data and git/content detection.
		w := windowMap[key]
		ep := enrichedPanes[i]

		if ep.hookData != nil {
			hd := ep.hookData
			if w.AgentMode == "" {
				w.AgentMode = hd.AgentMode
				w.PromptCount = hd.PromptCount
				w.ToolCount = hd.ToolCount
				w.SubagentCount = hd.SubagentCount
				w.SessionStartTS = hd.SessionStartTS
				if hd.SubagentCount > 0 {
					w.Subagents = detect.ReadCLISubagents(rp.paneID)
				}
			}
		}

		// SandboxType: hook data only (requires ~/.claude-tmux mounted in Docker).
		if w.SandboxType == "" && ep.hookData != nil && ep.hookData.SandboxType != "" {
			w.SandboxType = ep.hookData.SandboxType
		}

		// Worktree: git detection is the authoritative source.
		if w.OriginalRepo == "" && ep.gitInfo.IsWorktree && ep.gitInfo.OriginalRepo != "" {
			w.OriginalRepo   = ep.gitInfo.OriginalRepo
			w.WorktreeBranch = ep.gitInfo.Branch
			w.WorktreePath   = ep.pane.WorkingDir
		}
	}

	var windows []core.ClaudeWindow
	for _, w := range windowMap {
		windows = append(windows, *w)
	}

	sort.Slice(windows, func(i, j int) bool {
		if windows[i].Attached != windows[j].Attached {
			return windows[i].Attached
		}
		if windows[i].SessionName != windows[j].SessionName {
			return windows[i].SessionName < windows[j].SessionName
		}
		return windows[i].WindowIndex < windows[j].WindowIndex
	})

	return windows, nil
}

// ListAllSessions returns all Claude sessions from tmux, Cursor, and Cloud Agents.
// All three sources load in parallel for faster startup.
func ListAllSessions() ([]core.ClaudeWindow, error) {
	var (
		tmuxWindows   []core.ClaudeWindow
		cursorWindows []core.ClaudeWindow
		cloudWindows  []core.ClaudeWindow
		tmuxErr       error
	)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		tmuxWindows, tmuxErr = ListClaudeWindows()
	}()
	go func() {
		defer wg.Done()
		cursorWindows, _ = detect.ReadCursorSessions()
	}()
	go func() {
		defer wg.Done()
		cloudWindows = detect.ReadCloudAgents()
	}()
	wg.Wait()

	var allWindows []core.ClaudeWindow

	if tmuxErr == nil {
		for i := range tmuxWindows {
			tmuxWindows[i].Source = core.SourceCLI
			tmuxWindows[i].ComputeSearchText()
		}

		// Resolve active plan progress for CLI sessions (same as Cursor does via ResolvePlansForSessions).
		if len(tmuxWindows) > 0 {
			var cliSessions []plans.CLISession
			for _, w := range tmuxWindows {
				if p := w.PrimaryPane(); p != nil {
					cliSessions = append(cliSessions, plans.CLISession{
						PaneID:  p.PaneID,
						WorkDir: p.WorkingDir,
					})
				}
			}
			planMap := plans.ResolveCLIPlansForSessions(cliSessions)
			for i := range tmuxWindows {
				p := tmuxWindows[i].PrimaryPane()
				if p == nil {
					continue
				}
				// Plan file todos (collapsible in UI)
				if info, ok := planMap[p.PaneID]; ok && info.Total > 0 {
					tmuxWindows[i].ActivePlanTitle = info.Title
					tmuxWindows[i].ActivePlanDone = info.Completed
					tmuxWindows[i].ActivePlanTotal = info.Total
					tmuxWindows[i].ActivePlanTodos = info.Todos
				}
				// Native task list (always shown expanded, separate from plan todos)
				taskTodos := detect.ReadCLITaskList(p.PaneID)
				if len(taskTodos) == 0 {
					taskTodos = detect.ReadCLITodos(p.PaneID)
				}
				tmuxWindows[i].TaskTodos = taskTodos
			}
		}

		allWindows = append(allWindows, tmuxWindows...)
	}

	if len(cursorWindows) > 0 {
		// Enrich Cursor sessions with git info and subagent lists.
		g, _ := errgroup.WithContext(context.Background())
		g.SetLimit(8)

		for i := range cursorWindows {
			idx := i
			g.Go(func() error {
				win := &cursorWindows[idx]
				if len(win.Panes) == 0 {
					return nil
				}
				pane := &win.Panes[0]
				workDir := pane.WorkingDir
				if workDir == "" {
					workDir = win.WorkspacePath
				}

				gitInfo := git.DetectInfoCached(workDir)
				pane.GitBranch = gitInfo.Branch
				pane.GitDirty = gitInfo.Dirty
				pane.GitStaged = gitInfo.Staged
				pane.IsWorktree = gitInfo.IsWorktree
				pane.HasGit = gitInfo.HasGit

				if gitInfo.IsWorktree && gitInfo.OriginalRepo != "" {
					win.OriginalRepo   = gitInfo.OriginalRepo
					win.WorktreeBranch = gitInfo.Branch
					win.WorktreePath   = workDir
				}

				if win.SubagentCount > 0 && win.ConversationID != "" {
					win.Subagents = detect.ReadCursorSubagents(win.ConversationID)
				}

				win.ComputeSearchText()
				return nil
			})
		}
		_ = g.Wait()

		// Resolve active plan progress for Cursor sessions.
		var composerIDs []string
		for i := range cursorWindows {
			if cursorWindows[i].ConversationID != "" {
				composerIDs = append(composerIDs, cursorWindows[i].ConversationID)
			}
		}
		if len(composerIDs) > 0 {
			planMap := plans.ResolvePlansForSessions(composerIDs)
			for i := range cursorWindows {
			if info, ok := planMap[cursorWindows[i].ConversationID]; ok {
				cursorWindows[i].ActivePlanTitle = info.Title
				cursorWindows[i].ActivePlanDone = info.Completed
				cursorWindows[i].ActivePlanTotal = info.Total
				cursorWindows[i].ActivePlanTodos = info.Todos
			}
			}
		}

		allWindows = append(allWindows, cursorWindows...)
	}

	if len(cloudWindows) > 0 {
		allWindows = append(allWindows, cloudWindows...)
	}

	if len(allWindows) == 0 && tmuxErr != nil {
		return nil, tmuxErr
	}

	sort.Slice(allWindows, func(i, j int) bool {
		if allWindows[i].Source != allWindows[j].Source {
			return allWindows[i].Source < allWindows[j].Source
		}
		if allWindows[i].Attached != allWindows[j].Attached {
			return allWindows[i].Attached
		}
		if allWindows[i].SessionName != allWindows[j].SessionName {
			return allWindows[i].SessionName < allWindows[j].SessionName
		}
		return allWindows[i].WindowIndex < allWindows[j].WindowIndex
	})

	return allWindows, nil
}
