package state

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Cached home directory (computed once).
var (
	homeDir     string
	homeDirOnce sync.Once
)

// CachedHomeDir returns the cached home directory.
func CachedHomeDir() string {
	homeDirOnce.Do(func() {
		homeDir, _ = os.UserHomeDir()
	})
	return homeDir
}

// ShortenPath replaces the home directory with ~ and truncates long paths.
func ShortenPath(path string) string {
	home := CachedHomeDir()
	if home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + strings.TrimPrefix(path, home)
	}
	parts := strings.Split(path, string(filepath.Separator))
	if len(parts) > 3 {
		return ".../" + strings.Join(parts[len(parts)-2:], "/")
	}
	return path
}

// ExpandTilde expands ~ to the user's home directory.
func ExpandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home := CachedHomeDir()
	if home == "" {
		return path
	}
	return filepath.Join(home, path[1:])
}

// TruncateString truncates a string to maxLen, adding "…" if truncated.
func TruncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

// PadRight pads a string with spaces to the given width.
func PadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// MaxInt returns the larger of two ints.
func MaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MinInt returns the smaller of two ints.
func MinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StatusDir returns the path to the status directory (~/.claude-tmux).
func StatusDir() string {
	home := CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude-tmux")
}

// State file path for persisting selection across restarts.
const stateFileName = ".claude-tmux-state"

// stateFilePath returns the full path to the state file.
func stateFilePath() string {
	home := CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, stateFileName)
}

// SaveSelection persists the selected pane ID to disk.
func SaveSelection(paneID string) {
	path := stateFilePath()
	if path == "" {
		return
	}
	_ = os.WriteFile(path, []byte(paneID), 0600) // Best effort, ignore errors
}

// LoadSelection loads the previously selected pane ID from disk.
func LoadSelection() string {
	path := stateFilePath()
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
