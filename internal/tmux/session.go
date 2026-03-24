package tmux

import (
	"fmt"
	"strings"

	"github.com/inquire/tmux-overseer/internal/exec"
)

// KillSession kills the specified tmux session.
func KillSession(sessionName string) error {
	return exec.Run(exec.DefaultTimeout, "tmux", "kill-session", "-t", sessionName)
}

// SendKeys sends text input to a tmux pane.
func SendKeys(paneID string, text string) error {
	return exec.Run(exec.DefaultTimeout, "tmux", "send-keys", "-t", paneID, text, "Enter")
}

// NewSession creates a new tmux session with the given name and path.
func NewSession(name, path string) error {
	return exec.Run(exec.DefaultTimeout, "tmux", "new-session", "-d", "-s", name, "-c", path)
}

// StartClaudeInSession creates a new tmux session and starts Claude in it.
func StartClaudeInSession(name, path string) error {
	if err := NewSession(name, path); err != nil {
		return err
	}
	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "tmux", "list-panes", "-t", name, "-F", "#{pane_id}")
	if err != nil {
		return err
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return fmt.Errorf("could not find pane ID for session %s", name)
	}
	return SendKeys(paneID, "claude")
}

// CreateSessionWithCommand creates a new tmux session and runs a specific command in it.
func CreateSessionWithCommand(name, path, command string) error {
	if err := NewSession(name, path); err != nil {
		return err
	}
	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "tmux", "list-panes", "-t", name, "-F", "#{pane_id}")
	if err != nil {
		return err
	}
	paneID := strings.TrimSpace(string(out))
	if paneID == "" {
		return fmt.Errorf("could not find pane ID for session %s", name)
	}
	return SendKeys(paneID, command)
}

// RenameSession renames a tmux session.
func RenameSession(oldName, newName string) error {
	return exec.Run(exec.DefaultTimeout, "tmux", "rename-session", "-t", oldName, newName)
}
