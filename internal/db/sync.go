package db

import (
	"database/sql"
	"path/filepath"
	"strings"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/plans"
	"github.com/inquire/tmux-overseer/internal/state"
)

// SyncProgress reports progress during a sync operation.
type SyncProgress struct {
	Phase   string // "scanning", "syncing", "indexing"
	Current int
	Total   int
	Detail  string
}

// SyncResult summarizes what changed during a sync.
type SyncResult struct {
	TotalPlans    int
	NewPlans      int
	UpdatedPlans  int
	ArchivedPlans int
	ProjectCount  int
	EventCount    int
	ActivityDays  int
}

// FullResync drops all existing data and resyncs from scratch.
// Used when workspace resolution logic has changed.
func FullResync(entries []core.PlanEntry, progressCh chan<- SyncProgress) (*SyncResult, error) {
	d, err := Open()
	if err != nil {
		return nil, err
	}

	if progressCh != nil {
		progressCh <- SyncProgress{Phase: "cleaning", Total: len(entries)}
	}

	for _, table := range []string{
		"daily_activity", "activity_events", "plan_agents", "plan_todos", "plans", "projects",
	} {
		_, _ = d.Exec("DELETE FROM " + table)
	}

	if progressCh != nil {
		progressCh <- SyncProgress{Phase: "syncing", Total: len(entries)}
	}

	return doSync(d, entries, progressCh)
}

// SyncPlans diffs the given plan entries against the database and updates it.
// The optional progress channel receives per-item progress updates.
func SyncPlans(entries []core.PlanEntry, progressCh chan<- SyncProgress) (*SyncResult, error) {
	d, err := Open()
	if err != nil {
		return nil, err
	}

	if progressCh != nil {
		progressCh <- SyncProgress{Phase: "syncing", Total: len(entries)}
	}

	return doSync(d, entries, progressCh)
}

func doSync(d *sql.DB, entries []core.PlanEntry, progressCh chan<- SyncProgress) (*SyncResult, error) {
	existing, err := loadExistingPlans(d)
	if err != nil {
		return nil, err
	}

	result := &SyncResult{TotalPlans: len(entries)}
	seen := make(map[string]bool, len(entries))
	projects := make(map[string]bool)

	for i, entry := range entries {
		seen[entry.ConvID] = true
		ws := resolveWorkspace(entry)
		if ws != "" {
			projects[ws] = true
		}

		if progressCh != nil {
			detail := ""
			if ws != "" {
				detail = projectName(ws) + "/" + truncate(entry.Title, 40)
			} else {
				detail = truncate(entry.Title, 50)
			}
			progressCh <- SyncProgress{
				Phase:   "syncing",
				Current: i + 1,
				Total:   len(entries),
				Detail:  detail,
			}
		}

		prev, exists := existing[entry.ConvID]
		if !exists {
			if err := insertPlan(d, entry); err != nil {
				continue
			}
			emitEvent(d, ws, entry.ConvID, "", "plan_created", entry.LastActive)
			result.NewPlans++
		} else if entry.LastActive.After(prev.lastModified) {
			if err := updatePlan(d, entry); err != nil {
				continue
			}
			diffTodos(d, entry, prev)
			emitEvent(d, ws, entry.ConvID, "", "plan_modified", entry.LastActive)
			result.UpdatedPlans++
		}
	}

	for convID, prev := range existing {
		if !seen[convID] && prev.status == "active" {
			archivePlan(d, convID)
			emitEvent(d, prev.workspace, convID, "", "plan_archived", time.Now())
			result.ArchivedPlans++
		}
	}

	result.ProjectCount = len(projects)

	if progressCh != nil {
		progressCh <- SyncProgress{Phase: "indexing", Current: len(entries), Total: len(entries)}
	}

	// Populate plan_agents from Cursor registry
	syncCursorAgents(d)

	rebuildDailyActivity(d)

	// Collect diagnostic counts
	row := d.QueryRow(`SELECT COUNT(*) FROM activity_events`)
	_ = row.Scan(&result.EventCount)
	row = d.QueryRow(`SELECT COUNT(*) FROM daily_activity`)
	_ = row.Scan(&result.ActivityDays)

	return result, nil
}

func syncCursorAgents(d *sql.DB) {
	agentMap := plans.GetPlanAgents()
	if len(agentMap) == 0 {
		return
	}
	now := time.Now()
	for planID, info := range agentMap {
		if info.CreatedBy != "" {
			_, _ = d.Exec(
				`INSERT INTO plan_agents (plan_id, agent_id, role, first_seen_at)
				 VALUES (?, ?, 'creator', ?) ON CONFLICT DO NOTHING`,
				planID, info.CreatedBy, now,
			)
		}
		for _, editorID := range info.EditedBy {
			_, _ = d.Exec(
				`INSERT INTO plan_agents (plan_id, agent_id, role, first_seen_at)
				 VALUES (?, ?, 'editor', ?) ON CONFLICT DO NOTHING`,
				planID, editorID, now,
			)
		}
		for _, builderID := range info.BuiltBy {
			_, _ = d.Exec(
				`INSERT INTO plan_agents (plan_id, agent_id, role, first_seen_at)
				 VALUES (?, ?, 'builder', ?) ON CONFLICT DO NOTHING`,
				planID, builderID, now,
			)
		}
	}
}

type existingPlan struct {
	workspace    string
	lastModified time.Time
	status       string
	todos        map[string]string // todoID -> status
}

func loadExistingPlans(d *sql.DB) (map[string]existingPlan, error) {
	result := make(map[string]existingPlan)

	rows, err := d.Query(`SELECT conv_id, workspace_path, last_modified_at, status FROM plans`)
	if err != nil {
		return result, nil
	}
	defer rows.Close()

	for rows.Next() {
		var convID string
		var workspace sql.NullString
		var lastMod time.Time
		var status string
		if err := rows.Scan(&convID, &workspace, &lastMod, &status); err != nil {
			continue
		}
		result[convID] = existingPlan{
			workspace:    workspace.String,
			lastModified: lastMod,
			status:       status,
			todos:        make(map[string]string),
		}
	}

	todoRows, err := d.Query(`SELECT plan_id, todo_id, status FROM plan_todos`)
	if err != nil {
		return result, nil
	}
	defer todoRows.Close()

	for todoRows.Next() {
		var planID, todoID, status string
		if err := todoRows.Scan(&planID, &todoID, &status); err != nil {
			continue
		}
		if ep, ok := result[planID]; ok {
			ep.todos[todoID] = status
			result[planID] = ep
		}
	}

	return result, nil
}

func insertPlan(d *sql.DB, entry core.PlanEntry) error {
	sourceStr := "claude"
	if entry.Source == core.SourceCursor {
		sourceStr = "cursor"
	}

	ws := resolveWorkspace(entry)
	workspace := sql.NullString{String: ws, Valid: ws != ""}

	if ws != "" {
		_, _ = d.Exec(
			`INSERT INTO projects (workspace_path, name, first_seen_at, last_active_at)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT (workspace_path) DO UPDATE SET last_active_at = excluded.last_active_at`,
			ws, projectName(ws), entry.LastActive, entry.LastActive,
		)
	}

	_, err := d.Exec(
		`INSERT INTO plans (conv_id, workspace_path, source, title, overview, file_path,
		 created_by, created_at, last_modified_at, status, total_todos, completed_todos)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)
		 ON CONFLICT (conv_id) DO NOTHING`,
		entry.ConvID, workspace, sourceStr, entry.Title, entry.Overview, entry.FilePath,
		sql.NullString{String: entry.ConvID, Valid: sourceStr == "claude"},
		entry.LastActive, entry.LastActive,
		len(entry.Todos), entry.CompletedCount(),
	)
	if err != nil {
		return err
	}

	if sourceStr == "claude" {
		_, _ = d.Exec(
			`INSERT INTO plan_agents (plan_id, agent_id, role, first_seen_at)
			 VALUES (?, ?, 'creator', ?)
			 ON CONFLICT DO NOTHING`,
			entry.ConvID, entry.ConvID, entry.LastActive,
		)
	}

	for i, todo := range entry.Todos {
		todoID := core.Itoa(i)
		_, _ = d.Exec(
			`INSERT INTO plan_todos (plan_id, todo_id, content, status)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT (plan_id, todo_id) DO UPDATE SET content = excluded.content, status = excluded.status`,
			entry.ConvID, todoID, todo.Content, todo.Status,
		)
	}

	return nil
}

func updatePlan(d *sql.DB, entry core.PlanEntry) error {
	ws := resolveWorkspace(entry)

	if ws != "" {
		_, _ = d.Exec(
			`INSERT INTO projects (workspace_path, name, first_seen_at, last_active_at)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT (workspace_path) DO UPDATE SET last_active_at = excluded.last_active_at`,
			ws, projectName(ws), entry.LastActive, entry.LastActive,
		)
	}

	workspace := sql.NullString{String: ws, Valid: ws != ""}
	_, err := d.Exec(
		`UPDATE plans SET title = ?, overview = ?, workspace_path = ?,
		 last_modified_at = ?, total_todos = ?, completed_todos = ?, status = 'active'
		 WHERE conv_id = ?`,
		entry.Title, entry.Overview, workspace,
		entry.LastActive, len(entry.Todos), entry.CompletedCount(),
		entry.ConvID,
	)
	if err != nil {
		return err
	}

	_, _ = d.Exec(`DELETE FROM plan_todos WHERE plan_id = ?`, entry.ConvID)
	for i, todo := range entry.Todos {
		todoID := core.Itoa(i)
		_, _ = d.Exec(
			`INSERT INTO plan_todos (plan_id, todo_id, content, status) VALUES (?, ?, ?, ?)`,
			entry.ConvID, todoID, todo.Content, todo.Status,
		)
	}

	return nil
}

func diffTodos(d *sql.DB, entry core.PlanEntry, prev existingPlan) {
	ws := resolveWorkspace(entry)
	for i, todo := range entry.Todos {
		todoID := core.Itoa(i)
		prevStatus, existed := prev.todos[todoID]
		if !existed {
			continue
		}
		if prevStatus != "completed" && todo.Status == "completed" {
			emitEvent(d, ws, entry.ConvID, "", "todo_completed", entry.LastActive)
		} else if prevStatus != "in_progress" && todo.Status == "in_progress" {
			emitEvent(d, ws, entry.ConvID, "", "todo_started", entry.LastActive)
		}
	}
}

func archivePlan(d *sql.DB, convID string) {
	_, _ = d.Exec(`UPDATE plans SET status = 'archived' WHERE conv_id = ?`, convID)
}

func emitEvent(d *sql.DB, workspace, planID, agentID, eventType string, ts time.Time) {
	var nullAgent *string
	if agentID != "" {
		nullAgent = &agentID
	}
	_, _ = d.Exec(
		`INSERT INTO activity_events (workspace_path, plan_id, agent_id, event_type, occurred_at)
		 VALUES (?, ?, ?, ?, ?)`,
		workspace, planID, nullAgent, eventType, ts,
	)
}

func rebuildDailyActivity(d *sql.DB) {
	_, _ = d.Exec(`DELETE FROM daily_activity`)
	_, _ = d.Exec(`
		INSERT INTO daily_activity (workspace_path, activity_date,
			plans_created, plans_modified, todos_completed, conversations_started, composite_score)
		SELECT
			workspace_path,
			CAST(occurred_at AS DATE),
			COUNT(CASE WHEN event_type = 'plan_created' THEN 1 END),
			COUNT(CASE WHEN event_type = 'plan_modified' THEN 1 END),
			COUNT(CASE WHEN event_type = 'todo_completed' THEN 1 END),
			COUNT(CASE WHEN event_type = 'conversation_started' THEN 1 END),
			COUNT(CASE WHEN event_type = 'plan_created' THEN 1 END) * 3 +
			COUNT(CASE WHEN event_type = 'todo_completed' THEN 1 END) * 2 +
			COUNT(CASE WHEN event_type = 'conversation_started' THEN 1 END) * 1 +
			COUNT(CASE WHEN event_type = 'plan_modified' THEN 1 END) * 1
		FROM activity_events
		WHERE workspace_path IS NOT NULL AND workspace_path != ''
		GROUP BY workspace_path, CAST(occurred_at AS DATE)
	`)
}

// resolveWorkspace returns a workspace path for a plan entry.
// If the entry already has a workspace, it's returned as-is.
// Otherwise, for Cursor plans, derive it from the plan's file path.
func resolveWorkspace(entry core.PlanEntry) string {
	if entry.WorkspacePath != "" {
		return entry.WorkspacePath
	}
	// Cursor plans live at ~/.cursor/plans/*.plan.md
	// We can't derive a workspace from that, but the plan is associated
	// with the project that's open in Cursor. Use a fallback.
	if entry.Source == core.SourceCursor {
		return "(cursor)"
	}
	return ""
}

// projectName derives a human-readable project name from a workspace path.
// Handles tilde-prefixed paths and multi-component names.
func projectName(workspacePath string) string {
	if workspacePath == "" || workspacePath == "(cursor)" {
		return workspacePath
	}

	// Expand tilde for proper parsing
	expanded := workspacePath
	if strings.HasPrefix(expanded, "~/") {
		home := state.CachedHomeDir()
		if home != "" {
			expanded = filepath.Join(home, expanded[2:])
		}
	}

	base := filepath.Base(expanded)

	// If the base name is very generic (single word like "go", "src", "claude"),
	// include the parent directory for disambiguation
	if len(base) <= 4 || isGenericDirName(base) {
		parent := filepath.Base(filepath.Dir(expanded))
		if parent != "" && parent != "." && parent != "/" {
			return parent + "-" + base
		}
	}

	return base
}

func isGenericDirName(name string) bool {
	generic := map[string]bool{
		"go": true, "src": true, "app": true, "api": true,
		"web": true, "cli": true, "cmd": true, "lib": true,
		"ai": true, "ml": true, "ui": true, "docs": true,
	}
	return generic[strings.ToLower(name)]
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
