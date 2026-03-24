package plans

import (
	"sort"
	"sync"

	"github.com/inquire/tmux-overseer/internal/core"
)

// ScanAll loads plans from all sources (Cursor plans, Claude Code /plan files,
// and Claude Code conversation history), merges them, and returns them sorted
// by LastActive descending. The limit parameter caps how many Claude
// conversations to scan (Cursor plans and Claude plans are always fully loaded).
func ScanAll(claudeLimit int) []core.PlanEntry {
	var cursorPlans, claudePlans, claudeConvos []core.PlanEntry
	var wg sync.WaitGroup

	wg.Add(3)
	go func() {
		defer wg.Done()
		cursorPlans = ScanCursorPlans()
	}()
	go func() {
		defer wg.Done()
		claudePlans = ScanClaudePlans()
	}()
	go func() {
		defer wg.Done()
		claudeConvos = ScanClaudeConversations(claudeLimit)
	}()
	wg.Wait()

	all := make([]core.PlanEntry, 0, len(cursorPlans)+len(claudePlans)+len(claudeConvos))
	all = append(all, cursorPlans...)
	all = append(all, claudePlans...)
	all = append(all, claudeConvos...)

	sort.Slice(all, func(i, j int) bool {
		return all[i].LastActive.After(all[j].LastActive)
	})

	return all
}

// FilterIncomplete returns only plans that are not fully completed.
func FilterIncomplete(plans []core.PlanEntry) []core.PlanEntry {
	var result []core.PlanEntry
	for _, p := range plans {
		if !p.IsCompleted() {
			result = append(result, p)
		}
	}
	return result
}
