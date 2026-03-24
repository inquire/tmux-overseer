package db

import (
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
)

// LinkSessionToPlans records that sessions with active plans are building those plans.
// Called during each session refresh to keep plan_agents up to date.
func LinkSessionToPlans(windows []core.ClaudeWindow) {
	d, err := Open()
	if err != nil {
		return
	}

	now := time.Now()
	for _, w := range windows {
		if w.ActivePlanTitle == "" || w.ConversationID == "" {
			continue
		}

		// Find the plan by title match (best effort)
		var planID string
		row := d.QueryRow(`SELECT conv_id FROM plans WHERE title = ? LIMIT 1`, w.ActivePlanTitle)
		if row.Scan(&planID) != nil || planID == "" {
			continue
		}

		_, _ = d.Exec(
			`INSERT INTO plan_agents (plan_id, agent_id, role, first_seen_at)
			 VALUES (?, ?, 'builder', ?)
			 ON CONFLICT DO NOTHING`,
			planID, w.ConversationID, now,
		)
	}
}
