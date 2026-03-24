package tmux

import (
	"context"
	"strings"

	"github.com/inquire/tmux-overseer/internal/exec"
)

// CapturePaneContentCtx captures the last N non-trailing-blank lines of a tmux pane,
// using the provided context for cancellation support.
func CapturePaneContentCtx(ctx context.Context, paneID string, maxLines int) string {
	out, err := exec.RunWithContext(ctx, "tmux", "capture-pane", "-t", paneID, "-p", "-e")
	if err != nil {
		return ""
	}
	return trimPaneOutput(string(out), maxLines)
}

// CapturePaneContent captures the last N non-trailing-blank lines of a tmux pane.
func CapturePaneContent(paneID string, maxLines int) string {
	out, err := exec.RunWithTimeout(exec.DefaultTimeout, "tmux", "capture-pane", "-t", paneID, "-p", "-e")
	if err != nil {
		return ""
	}
	return trimPaneOutput(string(out), maxLines)
}

// trimPaneOutput returns the last maxLines non-trailing-blank lines from raw pane output.
func trimPaneOutput(raw string, maxLines int) string {
	allLines := strings.Split(raw, "\n")

	lastNonEmpty := len(allLines) - 1
	for lastNonEmpty >= 0 && strings.TrimSpace(allLines[lastNonEmpty]) == "" {
		lastNonEmpty--
	}
	if lastNonEmpty < 0 {
		return ""
	}

	start := lastNonEmpty - maxLines + 1
	if start < 0 {
		start = 0
	}

	return strings.Join(allLines[start:lastNonEmpty+1], "\n")
}
