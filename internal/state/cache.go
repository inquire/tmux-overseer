package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// SessionsCache is the on-disk structure for persisting the last-known session list.
// It is intentionally generic (raw JSON) so the state package does not import core.
type SessionsCache struct {
	SavedAt         time.Time       `json:"saved_at"`
	AttachedSession string          `json:"attached_session"`
	Windows         json.RawMessage `json:"windows"`
}

const sessionsCacheFile = ".claude-tmux-sessions.json"

// claudeTmuxDir returns the path to ~/.claude-tmux/, the shared state directory.
func claudeTmuxDir() string {
	home := CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude-tmux")
}

// sessionsCachePath returns the full path to the sessions cache file.
func sessionsCachePath() string {
	dir := claudeTmuxDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, sessionsCacheFile)
}

// SaveSessionsCache writes the cache to disk. windows must be a valid JSON-encoded
// []core.ClaudeWindow value. Errors are silently ignored (best-effort).
func SaveSessionsCache(attachedSession string, windows json.RawMessage) {
	path := sessionsCachePath()
	if path == "" {
		return
	}
	c := SessionsCache{
		SavedAt:         time.Now(),
		AttachedSession: attachedSession,
		Windows:         windows,
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

// LoadSessionsCache reads the sessions cache from disk.
// Returns nil if the file does not exist or cannot be parsed.
func LoadSessionsCache() *SessionsCache {
	path := sessionsCachePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c SessionsCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}

// PlansCache is the on-disk structure for persisting plan scan results.
type PlansCache struct {
	SavedAt time.Time       `json:"saved_at"`
	Plans   json.RawMessage `json:"plans"`
}

const plansCacheFile = ".claude-tmux-plans.json"

// plansCachePath returns the full path to the plans cache file.
func plansCachePath() string {
	dir := claudeTmuxDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, plansCacheFile)
}

// SavePlansCache writes plan entries to disk. plans must be valid JSON.
func SavePlansCache(plans json.RawMessage) {
	path := plansCachePath()
	if path == "" {
		return
	}
	c := PlansCache{
		SavedAt: time.Now(),
		Plans:   plans,
	}
	data, err := json.Marshal(c)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

// LoadPlansCache reads the plans cache from disk.
// Returns nil if the file does not exist or cannot be parsed.
func LoadPlansCache() *PlansCache {
	path := plansCachePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var c PlansCache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil
	}
	return &c
}
