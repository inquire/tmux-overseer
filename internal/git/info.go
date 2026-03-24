// Package git provides git operations for claude-tmux.
package git

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/inquire/tmux-overseer/internal/exec"
)

// Info holds git metadata for a working directory.
type Info struct {
	HasGit      bool
	Branch      string
	Dirty       bool   // unstaged changes
	Staged      bool   // staged changes
	IsWorktree  bool
	OriginalRepo string // non-empty for linked worktrees: path to the main checkout
}

// gitCache caches DetectInfo results by directory to avoid repeated subprocess calls.
// Entries expire after gitCacheTTL (10 seconds), covering multiple auto-refresh cycles.
const gitCacheTTL = 10 * time.Second

type cachedGitInfo struct {
	info      Info
	fetchedAt time.Time
}

var gitCache struct {
	mu    sync.Mutex
	items map[string]cachedGitInfo
}

// DetectInfoCached is like DetectInfo but returns a cached result when fresh
// (within gitCacheTTL). Avoids running 2 git subprocesses per working directory
// on every 5-second refresh cycle.
func DetectInfoCached(path string) Info {
	if path == "" {
		return Info{}
	}

	gitCache.mu.Lock()
	if gitCache.items == nil {
		gitCache.items = make(map[string]cachedGitInfo)
	}
	if entry, ok := gitCache.items[path]; ok && time.Since(entry.fetchedAt) < gitCacheTTL {
		gitCache.mu.Unlock()
		return entry.info
	}
	gitCache.mu.Unlock()

	info := DetectInfo(path)

	gitCache.mu.Lock()
	gitCache.items[path] = cachedGitInfo{info: info, fetchedAt: time.Now()}
	gitCache.mu.Unlock()

	return info
}

// InvalidateGitCache clears cached git info for a path, forcing fresh detection.
// Call this after git operations that change repository state.
func InvalidateGitCache(path string) {
	gitCache.mu.Lock()
	defer gitCache.mu.Unlock()
	if gitCache.items != nil {
		delete(gitCache.items, path)
	}
}

// DetectInfo gathers git information for a directory using only 2 subprocess calls.
func DetectInfo(path string) Info {
	info := Info{}

	// Call 1: is-inside-work-tree, git-dir, git-common-dir in one call.
	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "git", "-C", path, "rev-parse",
		"--is-inside-work-tree", "--git-dir", "--git-common-dir")
	if err != nil {
		return info
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 3 || lines[0] != "true" {
		return info
	}
	info.HasGit = true

	gitDir := strings.TrimSpace(lines[1])
	commonDir := strings.TrimSpace(lines[2])
	info.IsWorktree = gitDir != commonDir
	// For linked worktrees commonDir is an absolute path to the main .git dir.
	// Its parent is the original repo root.
	if info.IsWorktree && filepath.IsAbs(commonDir) {
		info.OriginalRepo = filepath.Dir(commonDir)
	}

	// Call 2: branch name + file status in one porcelain v2 call.
	out, err = exec.RunWithTimeout(exec.DefaultTimeout, "git", "-C", path, "status", "--porcelain=v2", "--branch")
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)

			if strings.HasPrefix(line, "# branch.head ") {
				branch := strings.TrimPrefix(line, "# branch.head ")
				if branch != "(detached)" {
					info.Branch = branch
				}
				continue
			}

			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}

			if len(line) >= 2 {
				parts := strings.Fields(line)
				if len(parts) >= 1 {
					xy := parts[0]
					if len(xy) >= 2 {
						index := xy[0]
						workTree := xy[1]
						if index != '.' && index != '?' {
							info.Staged = true
						}
						if workTree != '.' && workTree != '?' {
							info.Dirty = true
						}
					}
				}
			}
		}
	}

	return info
}
