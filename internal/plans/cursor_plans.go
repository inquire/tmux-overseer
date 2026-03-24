package plans

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/exec"
	"github.com/inquire/tmux-overseer/internal/state"
	"gopkg.in/yaml.v3"
)

// cursorPlanFrontmatter represents the YAML frontmatter in a Cursor .plan.md file.
type cursorPlanFrontmatter struct {
	Name      string           `yaml:"name"`
	Overview  string           `yaml:"overview"`
	Todos     []cursorPlanTodo `yaml:"todos"`
	Tags      []string         `yaml:"tags"`
	IsProject bool             `yaml:"isProject"`
	Workspace string           `yaml:"workspace"` // optional: workspace/repo path for CLI plans
}

type cursorPlanTodo struct {
	ID      string `yaml:"id"`
	Content string `yaml:"content"`
	Status  string `yaml:"status"`
}

// cursorPlansDir returns the path to ~/.cursor/plans/.
func cursorPlansDir() string {
	home := state.CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".cursor", "plans")
}

// ScanCursorPlans reads all .plan.md files from ~/.cursor/plans/ and returns PlanEntry items.
// Resolves workspace paths by reading Cursor's plan registry database.
func ScanCursorPlans() []core.PlanEntry {
	dir := cursorPlansDir()
	if dir == "" {
		return nil
	}

	pattern := filepath.Join(dir, "*.plan.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	var entries []core.PlanEntry
	for _, file := range files {
		entry, err := parseCursorPlan(file)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Resolve workspace paths from Cursor's plan registry
	enrichPlanWorkspaces(entries)

	return entries
}

// parseCursorPlan parses a single Cursor plan file's YAML frontmatter.
func parseCursorPlan(path string) (core.PlanEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return core.PlanEntry{}, err
	}
	defer f.Close()

	// Extract YAML frontmatter between --- delimiters
	scanner := bufio.NewScanner(f)
	var yamlLines []string
	inFrontmatter := false
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			if inFrontmatter {
				break // end of frontmatter
			}
			inFrontmatter = true
			continue
		}
		if inFrontmatter {
			yamlLines = append(yamlLines, line)
		}
	}

	if len(yamlLines) == 0 {
		return core.PlanEntry{}, os.ErrNotExist
	}

	var fm cursorPlanFrontmatter
	yamlData := strings.Join(yamlLines, "\n")
	if err := yaml.Unmarshal([]byte(yamlData), &fm); err != nil {
		return core.PlanEntry{}, err
	}

	// Convert todos
	todos := make([]core.PlanTodo, 0, len(fm.Todos))
	for _, t := range fm.Todos {
		todos = append(todos, core.PlanTodo{
			Content: t.Content,
			Status:  t.Status,
		})
	}

	// Get file modification time
	info, err := os.Stat(path)
	if err != nil {
		return core.PlanEntry{}, err
	}

	// Extract plan ID from filename (e.g., "performance_and_caching_improvements_3d1b135f" from the .plan.md path)
	planID := strings.TrimSuffix(filepath.Base(path), ".plan.md")

	return core.PlanEntry{
		Source:     core.SourceCursor,
		Title:      fm.Name,
		Overview:   fm.Overview,
		Todos:      todos,
		Tags:       fm.Tags,
		FilePath:   path,
		LastActive: info.ModTime(),
		ConvID:     planID, // used as key for workspace resolution
	}, nil
}

// --- Workspace resolution for Cursor plans ---

// planRegistryEntry is the minimal structure we need from the Cursor plan registry.
type planRegistryEntry struct {
	CreatedBy string `json:"createdBy"`
}

// cursorStateDBPath returns the path to Cursor's global state database.
func cursorStateDBPath() string {
	home := state.CachedHomeDir()
	if home == "" {
		return ""
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage", "state.vscdb")
	}
	// Linux fallback
	return filepath.Join(home, ".config", "Cursor", "User", "globalStorage", "state.vscdb")
}

// cursorProjectsDir returns ~/.cursor/projects/.
func cursorProjectsDir() string {
	home := state.CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".cursor", "projects")
}

// readPlanRegistry reads Cursor's composer.planRegistry from state.vscdb
// using the sqlite3 CLI. Returns a map of planID -> composerID (createdBy).
func readPlanRegistry() map[string]string {
	dbPath := cursorStateDBPath()
	if dbPath == "" {
		return nil
	}

	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "sqlite3", dbPath,
		"SELECT value FROM ItemTable WHERE key = 'composer.planRegistry'")
	if err != nil || len(out) == 0 {
		return nil
	}

	var registry map[string]planRegistryEntry
	if err := json.Unmarshal(out, &registry); err != nil {
		return nil
	}

	result := make(map[string]string, len(registry))
	for planID, entry := range registry {
		if entry.CreatedBy != "" {
			result[planID] = entry.CreatedBy
		}
	}
	return result
}

// buildComposerWorkspaceMap scans ~/.cursor/projects/*/agent-transcripts/
// and builds a map of composerID -> workspace path.
// Handles both old-style .txt files and new-style directories containing .jsonl.
func buildComposerWorkspaceMap() map[string]string {
	projDir := cursorProjectsDir()
	if projDir == "" {
		return nil
	}

	dirs, err := os.ReadDir(projDir)
	if err != nil {
		return nil
	}

	result := make(map[string]string)
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}

		workspace := projectDirToWorkspace(d.Name())
		if workspace == "" {
			continue
		}

		transcriptsDir := filepath.Join(projDir, d.Name(), "agent-transcripts")
		transcripts, err := os.ReadDir(transcriptsDir)
		if err != nil {
			continue
		}

		for _, t := range transcripts {
			if t.IsDir() {
				// New format: directory named by composerID
				result[t.Name()] = workspace
			} else if strings.HasSuffix(t.Name(), ".txt") {
				// Legacy format: composerID.txt
				composerID := strings.TrimSuffix(t.Name(), ".txt")
				result[composerID] = workspace
			} else if strings.HasSuffix(t.Name(), ".jsonl") {
				composerID := strings.TrimSuffix(t.Name(), ".jsonl")
				result[composerID] = workspace
			}
		}
	}

	return result
}

// projectDirToWorkspace converts a Cursor project directory name to a workspace path.
// e.g., "Users-<usernam>-go-src-tmux-claude-go" -> "/Users/<username>/go/src/tmux-claude-go"
//
// The encoding is ambiguous (hyphens in paths become hyphens in the dir name),
// so we verify the decoded path exists on disk. If the simple decode doesn't
// match, we try resolving against actual filesystem entries.
func projectDirToWorkspace(dirName string) string {
	if dirName == "" || !strings.HasPrefix(dirName, "Users-") {
		return ""
	}

	// Simple decode: replace leading segment and convert hyphens to slashes
	candidate := "/" + strings.ReplaceAll(dirName, "-", "/")

	// Clean up double slashes
	for strings.Contains(candidate, "//") {
		candidate = strings.ReplaceAll(candidate, "//", "/")
	}

	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}

	// Ambiguous case: try to resolve by walking the filesystem.
	// Split into segments and greedily match directory entries.
	parts := strings.Split(dirName, "-")
	return resolvePathSegments(parts)
}

// resolvePathSegments tries to reconstruct a filesystem path from hyphen-separated
// segments by greedily matching directory entries. This handles cases where the
// original path contained hyphens (e.g., "tmux-claude-go" encoded as "tmux-claude-go").
func resolvePathSegments(parts []string) string {
	if len(parts) == 0 {
		return ""
	}

	current := "/"
	i := 0
	for i < len(parts) {
		// Try increasingly longer segment combinations (greedy)
		matched := false
		for end := len(parts); end > i; end-- {
			segment := strings.Join(parts[i:end], "-")
			candidate := filepath.Join(current, segment)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				current = candidate
				i = end
				matched = true
				break
			}
		}
		if !matched {
			// Try single segment as fallback
			current = filepath.Join(current, parts[i])
			i++
		}
	}

	// Verify the final path exists
	if info, err := os.Stat(current); err == nil && info.IsDir() {
		return current
	}
	return ""
}

// --- Session-to-plan resolution for live Cursor sessions ---

// ActivePlanInfo holds plan progress for a live session.
type ActivePlanInfo struct {
	Title     string
	Completed int
	Total     int
	Todos     []core.PlanTodo
}

// CompactProgress returns a short "(3/5)" string, or empty if no todos.
func (a ActivePlanInfo) CompactProgress() string {
	if a.Total == 0 {
		return ""
	}
	return fmt.Sprintf("(%d/%d)", a.Completed, a.Total)
}

// ProgressBar returns "🟩🟩🟩⬜⬜ 3/5" style string, matching the plans view.
func (a ActivePlanInfo) ProgressBar() string {
	if a.Total == 0 {
		return ""
	}
	maxBlocks := 10
	if a.Total < maxBlocks {
		maxBlocks = a.Total
	}
	filled := 0
	if a.Total > 0 {
		filled = (a.Completed * maxBlocks) / a.Total
	}
	bar := strings.Repeat("🟩", filled) + strings.Repeat("⬜", maxBlocks-filled)
	return fmt.Sprintf("%s %d/%d", bar, a.Completed, a.Total)
}

// fullRegistryEntry has all fields needed for session-to-plan matching.
type fullRegistryEntry struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	CreatedBy     string              `json:"createdBy"`
	EditedBy      []string            `json:"editedBy"`
	BuiltBy       map[string][]string `json:"builtBy"`
	LastUpdatedAt int64               `json:"lastUpdatedAt"`
	URI           struct {
		FSPath string `json:"fsPath"`
	} `json:"uri"`
}

var fullRegistryCache struct {
	sync.Mutex
	entries   []fullRegistryEntry
	fetchedAt time.Time
}

const fullRegistryTTL = 30 * time.Second

func readFullPlanRegistry() []fullRegistryEntry {
	fullRegistryCache.Lock()
	defer fullRegistryCache.Unlock()

	if time.Since(fullRegistryCache.fetchedAt) < fullRegistryTTL && fullRegistryCache.entries != nil {
		return fullRegistryCache.entries
	}

	dbPath := cursorStateDBPath()
	if dbPath == "" {
		return nil
	}

	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "sqlite3", dbPath,
		"SELECT value FROM ItemTable WHERE key = 'composer.planRegistry'")
	if err != nil || len(out) == 0 {
		return nil
	}

	var registry map[string]fullRegistryEntry
	if err := json.Unmarshal(out, &registry); err != nil {
		return nil
	}

	entries := make([]fullRegistryEntry, 0, len(registry))
	for _, e := range registry {
		entries = append(entries, e)
	}

	fullRegistryCache.entries = entries
	fullRegistryCache.fetchedAt = time.Now()
	return entries
}

// PlanAgentInfo contains agent data for a plan from the Cursor registry.
type PlanAgentInfo struct {
	CreatedBy string
	EditedBy  []string
	BuiltBy   []string
}

// GetPlanAgents reads the Cursor plan registry and returns agent info per plan ID.
func GetPlanAgents() map[string]PlanAgentInfo {
	entries := readFullPlanRegistry()
	if len(entries) == 0 {
		return nil
	}

	result := make(map[string]PlanAgentInfo, len(entries))
	for _, e := range entries {
		info := PlanAgentInfo{
			CreatedBy: e.CreatedBy,
			EditedBy:  e.EditedBy,
		}
		for agentID := range e.BuiltBy {
			info.BuiltBy = append(info.BuiltBy, agentID)
		}
		result[e.ID] = info
	}
	return result
}

// ResolvePlansForSessions maps composer IDs (from live Cursor sessions)
// to plan progress info. Uses a TTL cache to avoid repeated SQLite queries.
func ResolvePlansForSessions(composerIDs []string) map[string]ActivePlanInfo {
	if len(composerIDs) == 0 {
		return nil
	}

	wanted := make(map[string]struct{}, len(composerIDs))
	for _, id := range composerIDs {
		wanted[id] = struct{}{}
	}

	entries := readFullPlanRegistry()
	if len(entries) == 0 {
		return nil
	}

	// For each wanted composer ID, find the most recently updated plan
	type bestMatch struct {
		entry fullRegistryEntry
	}
	best := make(map[string]*bestMatch)

	for _, entry := range entries {
		associated := make(map[string]bool)
		if entry.CreatedBy != "" {
			associated[entry.CreatedBy] = true
		}
		for _, id := range entry.EditedBy {
			associated[id] = true
		}
		for id := range entry.BuiltBy {
			associated[id] = true
		}

		for cid := range associated {
			if _, ok := wanted[cid]; !ok {
				continue
			}
			if existing, ok := best[cid]; !ok || entry.LastUpdatedAt > existing.entry.LastUpdatedAt {
				best[cid] = &bestMatch{entry: entry}
			}
		}
	}

	if len(best) == 0 {
		return nil
	}

	planDir := cursorPlansDir()
	result := make(map[string]ActivePlanInfo, len(best))

	for composerID, m := range best {
		filePath := m.entry.URI.FSPath
		if filePath == "" && planDir != "" {
			filePath = filepath.Join(planDir, m.entry.ID+".plan.md")
		}
		if filePath == "" {
			result[composerID] = ActivePlanInfo{Title: m.entry.Name}
			continue
		}

		planEntry, err := parseCursorPlan(filePath)
		if err != nil {
			result[composerID] = ActivePlanInfo{Title: m.entry.Name}
			continue
		}

		result[composerID] = ActivePlanInfo{
			Title:     m.entry.Name,
			Completed: planEntry.CompletedCount(),
			Total:     len(planEntry.Todos),
			Todos:     planEntry.Todos,
		}
	}

	return result
}

// enrichPlanWorkspaces resolves and sets WorkspacePath for Cursor plans
// by reading the plan registry and matching composer IDs to project folders.
func enrichPlanWorkspaces(entries []core.PlanEntry) {
	if len(entries) == 0 {
		return
	}

	// Read plan registry: planID -> composerID
	registry := readPlanRegistry()
	if len(registry) == 0 {
		return
	}

	// Build composer -> workspace map
	composerMap := buildComposerWorkspaceMap()
	if len(composerMap) == 0 {
		return
	}

	// Enrich each plan entry
	home := state.CachedHomeDir()
	for i := range entries {
		planID := entries[i].ConvID
		composerID, ok := registry[planID]
		if !ok {
			continue
		}
		workspace, ok := composerMap[composerID]
		if !ok {
			continue
		}

		// Shorten for display (~/...)
		display := workspace
		if home != "" && strings.HasPrefix(workspace, home) {
			display = "~" + workspace[len(home):]
		}
		entries[i].WorkspacePath = display
	}
}
