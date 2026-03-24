package git

import (
	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/exec"
)

// StageAll stages all changes in the given directory.
func StageAll(path string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Run(exec.DefaultTimeout, "git", "-C", path, "add", "-A")
		if err != nil {
			return core.GitResultMsg{Success: false, Message: "Stage failed: " + err.Error()}
		}
		InvalidateGitCache(path)
		return core.GitResultMsg{Success: true, Message: "All changes staged"}
	}
}

// Commit creates a commit with the given message.
func Commit(path, message string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Run(exec.DefaultTimeout, "git", "-C", path, "commit", "-m", message)
		if err != nil {
			return core.GitResultMsg{Success: false, Message: "Commit failed: " + err.Error()}
		}
		InvalidateGitCache(path)
		return core.GitResultMsg{Success: true, Message: "Committed"}
	}
}

// Push pushes to the remote. Falls back to setting upstream if the first push fails.
func Push(path string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Run(exec.DefaultTimeout, "git", "-C", path, "push")
		if err != nil {
			err = exec.Run(exec.DefaultTimeout, "git", "-C", path, "push", "-u", "origin", "HEAD")
			if err != nil {
				return core.GitResultMsg{Success: false, Message: "Push failed: " + err.Error()}
			}
			InvalidateGitCache(path)
			return core.GitResultMsg{Success: true, Message: "Pushed (set upstream)"}
		}
		InvalidateGitCache(path)
		return core.GitResultMsg{Success: true, Message: "Pushed"}
	}
}

// Fetch fetches from the remote.
func Fetch(path string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Run(exec.DefaultTimeout, "git", "-C", path, "fetch")
		if err != nil {
			return core.GitResultMsg{Success: false, Message: "Fetch failed: " + err.Error()}
		}
		InvalidateGitCache(path)
		return core.GitResultMsg{Success: true, Message: "Fetched"}
	}
}

// WorktreeAdd creates a new worktree.
func WorktreeAdd(repoPath, worktreePath, branch string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Run(exec.DefaultTimeout, "git", "-C", repoPath, "worktree", "add", worktreePath, "-b", branch)
		if err != nil {
			return core.GitResultMsg{Success: false, Message: "Worktree creation failed: " + err.Error()}
		}
		return core.GitResultMsg{Success: true, Message: "Worktree created"}
	}
}

// WorktreeRemove removes a worktree.
func WorktreeRemove(repoPath, worktreePath string) tea.Cmd {
	return func() tea.Msg {
		err := exec.Run(exec.DefaultTimeout, "git", "-C", repoPath, "worktree", "remove", worktreePath)
		if err != nil {
			return core.GitResultMsg{Success: false, Message: "Worktree removal failed: " + err.Error()}
		}
		return core.GitResultMsg{Success: true, Message: "Worktree removed"}
	}
}
