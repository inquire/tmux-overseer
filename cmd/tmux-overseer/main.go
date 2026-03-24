package main

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/inquire/tmux-overseer/internal/db"
	"github.com/inquire/tmux-overseer/internal/ui"
)

func main() {
	defer db.Close()

	p := tea.NewProgram(ui.InitialModel())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
