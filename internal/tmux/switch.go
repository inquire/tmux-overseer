package tmux

import (
	"fmt"
	"strings"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/exec"
)

// SwitchToTarget switches the tmux client to a specific session:window,
// then selects the given pane. Using session:window format ensures
// cross-session switching works correctly inside tmux popups.
func SwitchToTarget(sessionName string, windowIndex int, paneID string) {
	target := fmt.Sprintf("%s:%d", sessionName, windowIndex)
	_ = exec.Run(exec.DefaultTimeout, "tmux", "switch-client", "-t", target) // Best effort
	_ = exec.Run(exec.DefaultTimeout, "tmux", "select-pane", "-t", paneID)   // Best effort
}

// CurrentSessionName returns the name of the currently attached tmux session.
func CurrentSessionName() string {
	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "tmux", "display-message", "-p", "#{session_name}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// SwitchToSession switches to the appropriate window based on its source.
// For CLI sessions, uses tmux commands. For Cursor sessions, opens via deeplink.
func SwitchToSession(win core.ClaudeWindow) error {
	if win.Source == core.SourceCursor {
		return OpenCursorWorkspace(win.WorkspacePath)
	}
	p := win.PrimaryPane()
	if p == nil {
		return nil
	}
	SwitchToTarget(win.SessionName, win.WindowIndex, p.PaneID)
	return nil
}

// OpenCursorWorkspace opens a Cursor workspace using the cursor:// URL scheme.
func OpenCursorWorkspace(workspacePath string) error {
	if workspacePath == "" {
		return fmt.Errorf("no workspace path")
	}
	url := "cursor://file/" + workspacePath
	return exec.Run(exec.DefaultTimeout, "open", url)
}

// OpenInTerminal creates a new tmux session for a given path and starts Claude.
// Sanitizes the session name to be tmux-safe.
func OpenInTerminal(name, path string) error {
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	return StartClaudeInSession(safeName, path)
}
