package main

import (
	"fmt"
	"io"
	"os"

	"github.com/inquire/tmux-overseer/internal/hook"
)

func main() {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		os.Exit(1)
	}

	statusDir := os.Getenv("HOME") + "/.claude-tmux"
	os.MkdirAll(statusDir, 0o755)

	tmuxPane := os.Getenv("TMUX_PANE")

	if err := hook.Process(input, statusDir, tmuxPane); err != nil {
		fmt.Fprintf(os.Stderr, "hook error: %v\n", err)
		os.Exit(1)
	}
}
