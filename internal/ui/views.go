package ui

import (
	"fmt"

	"github.com/inquire/tmux-overseer/internal/core"
)

// renderApp dispatches to the appropriate view renderer.
func renderApp(m Model) string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err)
	}

	if m.loading && len(m.windows) == 0 {
		return renderLoadingScreen(m)
	}

	switch m.mode {
	case core.ModePlans, core.ModePlanFilter:
		return renderPlansView(m)
	case core.ModeActivity:
		return renderActivityView(m)
	default:
		return renderMainLayout(m)
	}
}
