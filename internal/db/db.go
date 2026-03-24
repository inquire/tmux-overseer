package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"

	_ "github.com/marcboeker/go-duckdb"
)

var (
	globalDB   *sql.DB
	globalOnce sync.Once
	globalErr  error
)

func dbPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	dir := filepath.Join(home, ".claude-tmux")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "plans.duckdb")
}

// Open returns the singleton DuckDB connection, creating it on first call.
func Open() (*sql.DB, error) {
	globalOnce.Do(func() {
		path := dbPath()
		if path == "" {
			globalErr = os.ErrNotExist
			return
		}
		globalDB, globalErr = sql.Open("duckdb", path)
		if globalErr != nil {
			return
		}
		globalErr = migrate(globalDB)
	})
	return globalDB, globalErr
}

// Close shuts down the DuckDB connection.
func Close() {
	if globalDB != nil {
		globalDB.Close()
		globalDB = nil
	}
}

func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS projects (
			workspace_path TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			first_seen_at TIMESTAMP NOT NULL DEFAULT current_timestamp,
			last_active_at TIMESTAMP NOT NULL DEFAULT current_timestamp
		)`,
		`CREATE TABLE IF NOT EXISTS plans (
			conv_id TEXT PRIMARY KEY,
			workspace_path TEXT,
			source TEXT NOT NULL,
			title TEXT NOT NULL,
			overview TEXT,
			file_path TEXT NOT NULL,
			created_by TEXT,
			created_at TIMESTAMP NOT NULL DEFAULT current_timestamp,
			last_modified_at TIMESTAMP NOT NULL DEFAULT current_timestamp,
			status TEXT NOT NULL DEFAULT 'active',
			total_todos INTEGER DEFAULT 0,
			completed_todos INTEGER DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS plan_agents (
			plan_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			role TEXT NOT NULL,
			first_seen_at TIMESTAMP NOT NULL DEFAULT current_timestamp,
			PRIMARY KEY (plan_id, agent_id, role)
		)`,
		`CREATE TABLE IF NOT EXISTS plan_todos (
			plan_id TEXT NOT NULL,
			todo_id TEXT NOT NULL,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			PRIMARY KEY (plan_id, todo_id)
		)`,
		`CREATE TABLE IF NOT EXISTS activity_events (
			workspace_path TEXT NOT NULL,
			plan_id TEXT,
			agent_id TEXT,
			event_type TEXT NOT NULL,
			event_data TEXT,
			occurred_at TIMESTAMP NOT NULL DEFAULT current_timestamp
		)`,
		`CREATE TABLE IF NOT EXISTS daily_activity (
			workspace_path TEXT NOT NULL,
			activity_date DATE NOT NULL,
			plans_created INTEGER DEFAULT 0,
			plans_modified INTEGER DEFAULT 0,
			todos_completed INTEGER DEFAULT 0,
			conversations_started INTEGER DEFAULT 0,
			composite_score INTEGER DEFAULT 0,
			PRIMARY KEY (workspace_path, activity_date)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Migration: drop old activity_events with id column and sequence
	db.Exec(`DROP SEQUENCE IF EXISTS activity_events_seq`)
	// Check if old table has 'id' column and recreate without it
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns
		WHERE table_name = 'activity_events' AND column_name = 'id'`)
	if err == nil {
		hasID := rows.Next()
		rows.Close()
		if hasID {
			db.Exec(`DROP TABLE activity_events`)
			db.Exec(`CREATE TABLE activity_events (
				workspace_path TEXT NOT NULL,
				plan_id TEXT,
				agent_id TEXT,
				event_type TEXT NOT NULL,
				event_data TEXT,
				occurred_at TIMESTAMP NOT NULL DEFAULT current_timestamp
			)`)
		}
	}

	return nil
}
