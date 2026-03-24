package plans

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/state"
)

// claudeMessage represents the minimal fields we need from a JSONL line.
type claudeMessage struct {
	Type    string `json:"type"`
	Message struct {
		Role    string      `json:"role"`
		Content interface{} `json:"content"` // string or []block
	} `json:"message"`
}

// claudeProjectsDir returns the path to ~/.claude/projects/.
func claudeProjectsDir() string {
	home := state.CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// dirNameToWorkspace converts a Claude project directory name to a workspace path.
// e.g. "-Users-<username>-go-src-my-project" -> "/Users/<username>/go/src/my-project"
//
// Uses filesystem checks to correctly preserve dashes in directory names
// (e.g. "my-project" stays as one component rather than becoming "my/project").
func dirNameToWorkspace(name string) string {
	if name == "" {
		return ""
	}
	segments := strings.Split(name[1:], "-") // skip leading dash
	if len(segments) == 0 {
		return "/"
	}

	current := "/"
	i := 0
	for i < len(segments) {
		// Try longest dash-joined segment first, then shorter ones
		matched := false
		maxEnd := len(segments)
		if maxEnd > i+8 {
			maxEnd = i + 8 // limit lookahead for performance
		}
		for end := maxEnd; end > i+1; end-- {
			candidate := strings.Join(segments[i:end], "-")
			tryPath := filepath.Join(current, candidate)
			if info, err := os.Stat(tryPath); err == nil && info.IsDir() {
				current = tryPath
				i = end
				matched = true
				break
			}
		}
		if !matched {
			current = filepath.Join(current, segments[i])
			i++
		}
	}

	return current
}

// ScanClaudeConversations reads recent JSONL conversation files from ~/.claude/projects/
// and returns PlanEntry items. Only the most recent `limit` conversations are returned.
func ScanClaudeConversations(limit int) []core.PlanEntry {
	dir := claudeProjectsDir()
	if dir == "" {
		return nil
	}

	projDirs, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	type convFile struct {
		path      string
		workspace string
		modTime   int64
	}

	// Collect all JSONL files with their mod times
	var files []convFile
	for _, pd := range projDirs {
		if !pd.IsDir() {
			continue
		}
		workspace := dirNameToWorkspace(pd.Name())
		projPath := filepath.Join(dir, pd.Name())

		entries, err := os.ReadDir(projPath)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, convFile{
				path:      filepath.Join(projPath, e.Name()),
				workspace: workspace,
				modTime:   info.ModTime().Unix(),
			})
		}
	}

	// Sort by mod time descending, take top N
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})
	if len(files) > limit {
		files = files[:limit]
	}

	var entries []core.PlanEntry
	for _, cf := range files {
		entry, err := parseClaudeConversation(cf.path, cf.workspace)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	return entries
}

// parseClaudeConversation extracts a title from the first user message in a JSONL file.
func parseClaudeConversation(path, workspace string) (core.PlanEntry, error) {
	info, err := os.Stat(path)
	if err != nil {
		return core.PlanEntry{}, err
	}

	// Extract conversation ID from filename
	convID := strings.TrimSuffix(filepath.Base(path), ".jsonl")

	title := extractTitle(path)
	if title == "" {
		title = "(no title)"
	}

	// Shorten workspace for display
	home := state.CachedHomeDir()
	displayWorkspace := workspace
	if home != "" && strings.HasPrefix(workspace, home) {
		displayWorkspace = "~" + workspace[len(home):]
	}

	return core.PlanEntry{
		Source:        core.SourceCLI,
		Title:         title,
		WorkspacePath: displayWorkspace,
		FilePath:      path,
		LastActive:    info.ModTime(),
		ConvID:        convID,
	}, nil
}

// extractTitle reads a JSONL file and returns the first meaningful user message.
func extractTitle(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// Increase buffer for large lines
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Type != "user" {
			continue
		}

		text := extractTextContent(msg.Message.Content)
		if text == "" {
			continue
		}

		// Skip system/command messages
		if strings.HasPrefix(text, "<local-command") || strings.HasPrefix(text, "<command") {
			continue
		}

		// Trim and truncate
		text = strings.TrimSpace(text)
		if len(text) > 80 {
			text = text[:80] + "..."
		}

		if len(text) > 3 {
			return text
		}
	}

	return ""
}

// extractTextContent gets the text from a message content field,
// which can be a string or an array of content blocks.
func extractTextContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return v
	case []interface{}:
		for _, block := range v {
			if m, ok := block.(map[string]interface{}); ok {
				if m["type"] == "text" {
					if text, ok := m["text"].(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}
