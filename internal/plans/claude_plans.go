package plans

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
	"gopkg.in/yaml.v3"
)

// claudeSettings mirrors the relevant subset of ~/.claude/settings.json.
type claudeSettings struct {
	PlansDirectory string `json:"plansDirectory"`
}

// claudePlansDir returns the directory where Claude Code stores /plan files.
// It reads ~/.claude/settings.json to check for a user-configured plansDirectory,
// and falls back to ~/.claude/plans/ (the default introduced in Claude Code v2.1.0).
func claudePlansDir() string {
	home := state.CachedHomeDir()
	if home == "" {
		return ""
	}

	defaultDir := filepath.Join(home, ".claude", "plans")

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return defaultDir
	}

	var cfg claudeSettings
	if err := json.Unmarshal(data, &cfg); err != nil {
		return defaultDir
	}
	if cfg.PlansDirectory == "" {
		return defaultDir
	}
	return state.ExpandTilde(cfg.PlansDirectory)
}

// ScanClaudePlans reads all .plan.md files from the Claude Code plans directory
// and returns PlanEntry items. These are created by the /plan command
// (Claude Code v2.1.0+). They share the same YAML frontmatter format as Cursor plans.
func ScanClaudePlans() []core.PlanEntry {
	dir := claudePlansDir()
	if dir == "" {
		return nil
	}

	pattern := filepath.Join(dir, "*.plan.md")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) == 0 {
		return nil
	}

	var entries []core.PlanEntry
	for _, file := range files {
		entry, err := parseClaudePlan(file)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

// parseClaudePlan parses a single Claude Code plan file.
// The format is identical to Cursor's .plan.md: YAML frontmatter between ---
// delimiters, followed by optional Markdown body.
func parseClaudePlan(path string) (core.PlanEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return core.PlanEntry{}, err
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	var fm cursorPlanFrontmatter
	hasFrontmatter := false
	bodyStart := 0

	// Try to parse YAML frontmatter
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}
		if endIdx > 0 {
			yamlData := strings.Join(lines[1:endIdx], "\n")
			if err := yaml.Unmarshal([]byte(yamlData), &fm); err == nil && fm.Name != "" {
				hasFrontmatter = true
				bodyStart = endIdx + 1
			}
		}
	}

	// If no frontmatter or no name, extract title from first # heading
	title := fm.Name
	if title == "" {
		for _, line := range lines[bodyStart:] {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "# ") {
				title = strings.TrimPrefix(trimmed, "# ")
				// Strip "Plan: " prefix if present
				title = strings.TrimPrefix(title, "Plan: ")
				break
			}
		}
	}
	if title == "" {
		// Last resort: use filename
		base := filepath.Base(path)
		title = strings.TrimSuffix(strings.TrimSuffix(base, ".plan.md"), ".md")
		title = strings.ReplaceAll(title, "-", " ")
	}

	// Extract overview from frontmatter or first non-heading paragraph
	overview := fm.Overview
	if overview == "" && !hasFrontmatter {
		pastHeading := false
		for _, line := range lines[bodyStart:] {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				pastHeading = true
				continue
			}
			if pastHeading && trimmed != "" {
				overview = trimmed
				break
			}
		}
	}

	todos := make([]core.PlanTodo, 0, len(fm.Todos))
	for _, t := range fm.Todos {
		todos = append(todos, core.PlanTodo{
			Content: t.Content,
			Status:  t.Status,
		})
	}

	// If no frontmatter todos, scan for markdown checkboxes
	if len(todos) == 0 {
		for _, line := range lines[bodyStart:] {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
				todos = append(todos, core.PlanTodo{
					Content: strings.TrimSpace(trimmed[6:]),
					Status:  "completed",
				})
			} else if strings.HasPrefix(trimmed, "- [ ] ") {
				todos = append(todos, core.PlanTodo{
					Content: strings.TrimSpace(trimmed[6:]),
					Status:  "pending",
				})
			}
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return core.PlanEntry{}, err
	}

	base := filepath.Base(path)
	convID := strings.TrimSuffix(strings.TrimSuffix(base, ".plan.md"), ".md")

	// Prefer explicit workspace from frontmatter; fall back to plans dir.
	workspace := fm.Workspace
	if workspace == "" {
		workspace = claudePlansDir()
	} else {
		workspace = state.ExpandTilde(workspace)
	}

	return core.PlanEntry{
		Source:        core.SourceCLI,
		Title:         title,
		Overview:      overview,
		Todos:         todos,
		Tags:          fm.Tags,
		WorkspacePath: workspace,
		FilePath:      path,
		LastActive:    info.ModTime(),
		ConvID:        convID,
	}, nil
}

// ResolveCLIPlansForSessions matches Claude Code plan files to live CLI sessions.
// Matching strategy (in order):
//  1. Plan has explicit `workspace:` frontmatter field that matches the session CWD.
//  2. Plan filename slug contains the session working directory basename.
//  3. Most recently modified plan with incomplete todos assigned to the most recently
//     active session (fallback for ambiguous cases).
//
// Returns a map of paneID → ActivePlanInfo.
func ResolveCLIPlansForSessions(sessions []CLISession) map[string]ActivePlanInfo {
	if len(sessions) == 0 {
		return nil
	}

	entries := ScanClaudePlans()
	if len(entries) == 0 {
		return nil
	}

	// Only consider plans modified in the last 7 days with at least one todo.
	cutoff := time.Now().AddDate(0, 0, -7)
	var candidates []core.PlanEntry
	for _, e := range entries {
		if e.LastActive.Before(cutoff) {
			continue
		}
		candidates = append(candidates, e)
	}
	if len(candidates) == 0 {
		return nil
	}
	// Sort newest-first.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].LastActive.After(candidates[j].LastActive)
	})

	result := make(map[string]ActivePlanInfo)
	usedPlan := make(map[string]bool) // plan filepath → already matched

	// Pass 1: explicit workspace match.
	for _, s := range sessions {
		for _, e := range candidates {
			if usedPlan[e.FilePath] {
				continue
			}
			if e.WorkspacePath == claudePlansDir() {
				continue // no explicit workspace
			}
			if e.WorkspacePath == s.WorkDir ||
				strings.HasSuffix(s.WorkDir, "/"+filepath.Base(e.WorkspacePath)) {
				result[s.PaneID] = planToActiveInfo(e)
				usedPlan[e.FilePath] = true
				break
			}
		}
	}

	// Pass 2: filename slug contains the session's working directory basename.
	for _, s := range sessions {
		if _, matched := result[s.PaneID]; matched {
			continue
		}
		base := filepath.Base(s.WorkDir)
		if base == "" || base == "." {
			continue
		}
		slug := strings.ToLower(base)
		for _, e := range candidates {
			if usedPlan[e.FilePath] {
				continue
			}
			planSlug := strings.ToLower(filepath.Base(e.FilePath))
			if strings.Contains(planSlug, slug) {
				result[s.PaneID] = planToActiveInfo(e)
				usedPlan[e.FilePath] = true
				break
			}
		}
	}

	// Pass 3: assign the newest unmatched plan to each unmatched session.
	for _, s := range sessions {
		if _, matched := result[s.PaneID]; matched {
			continue
		}
		for _, e := range candidates {
			if usedPlan[e.FilePath] {
				continue
			}
			result[s.PaneID] = planToActiveInfo(e)
			usedPlan[e.FilePath] = true
			break
		}
	}

	return result
}

// CLISession is a minimal representation of a live CLI session for plan matching.
type CLISession struct {
	PaneID  string
	WorkDir string
}

func planToActiveInfo(e core.PlanEntry) ActivePlanInfo {
	completed := 0
	for _, t := range e.Todos {
		if t.Status == "completed" {
			completed++
		}
	}
	return ActivePlanInfo{
		Title:     e.Title,
		Completed: completed,
		Total:     len(e.Todos),
		Todos:     e.Todos,
	}
}
