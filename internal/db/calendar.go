package db

import (
	"database/sql"
	"time"
)

// DayActivity holds the composite score for a single day.
type DayActivity struct {
	Date  time.Time
	Score int
}

// ProjectSummary holds aggregate stats for a single project.
type ProjectSummary struct {
	WorkspacePath  string
	Name           string
	TotalPlans     int
	CompletedPlans int
	TotalTodos     int
	CompletedTodos int
	TotalScore     int
}

// DayDetail holds plan-level detail for a specific day.
type DayDetail struct {
	Date           time.Time
	PlansTouched   int
	TodosCompleted int
	Projects       []DayProjectDetail
}

// DayProjectDetail groups plans by project for a specific day.
type DayProjectDetail struct {
	ProjectName string
	Plans       []DayPlanEntry
}

// DayPlanEntry is a plan that was active on a specific day.
type DayPlanEntry struct {
	ConvID         string
	Title          string
	ProjectName    string
	EventType      string
	Source         string // "cli" or "cursor"
	CompletedTodos int
	TotalTodos     int
}

// GetActivityGrid computes daily scores directly from activity_events.
func GetActivityGrid(weeks int) []DayActivity {
	d, err := Open()
	if err != nil {
		return nil
	}

	startDate := time.Now().AddDate(0, 0, -weeks*7)

	scoreMap := queryDayScores(d, startDate)

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	start := today.AddDate(0, 0, -weeks*7)
	for start.Weekday() != time.Sunday {
		start = start.AddDate(0, 0, -1)
	}

	var result []DayActivity
	for dt := start; !dt.After(today); dt = dt.AddDate(0, 0, 1) {
		key := dt.Format("2006-01-02")
		result = append(result, DayActivity{
			Date:  dt,
			Score: scoreMap[key],
		})
	}

	return result
}

// queryDayScores computes composite scores per day from activity_events.
func queryDayScores(d *sql.DB, since time.Time) map[string]int {
	scoreMap := make(map[string]int)

	rows, err := d.Query(`
		SELECT
			CAST(occurred_at AS DATE) AS day,
			CAST(COALESCE(SUM(CASE
				WHEN event_type = 'plan_created' THEN 3
				WHEN event_type = 'todo_completed' THEN 2
				WHEN event_type = 'conversation_started' THEN 1
				WHEN event_type = 'plan_modified' THEN 1
				ELSE 0
			END), 0) AS INTEGER) AS score
		FROM activity_events
		GROUP BY CAST(occurred_at AS DATE)
		ORDER BY day`)
	if err != nil {
		return scoreMap
	}
	defer rows.Close()

	sinceDate := since.Format("2006-01-02")
	for rows.Next() {
		var dt time.Time
		var score int
		if err := rows.Scan(&dt, &score); err != nil {
			continue
		}
		key := dt.Format("2006-01-02")
		if key >= sinceDate {
			scoreMap[key] = score
		}
	}

	return scoreMap
}

// GetProjectSummaries returns stats for all projects, ordered by activity.
func GetProjectSummaries() []ProjectSummary {
	d, err := Open()
	if err != nil {
		return nil
	}

	rows, err := d.Query(`
		SELECT
			p.workspace_path,
			p.name,
			COUNT(DISTINCT pl.conv_id) AS total_plans,
			COUNT(DISTINCT CASE WHEN pl.status = 'completed' THEN pl.conv_id END) AS completed_plans,
			COALESCE(SUM(pl.total_todos), 0),
			COALESCE(SUM(pl.completed_todos), 0),
			COALESCE((
				SELECT SUM(CASE
					WHEN event_type = 'plan_created' THEN 3
					WHEN event_type = 'todo_completed' THEN 2
					WHEN event_type = 'conversation_started' THEN 1
					WHEN event_type = 'plan_modified' THEN 1
					ELSE 0
				END)
				FROM activity_events ae
				WHERE ae.workspace_path = p.workspace_path
			), 0) AS total_score
		FROM projects p
		LEFT JOIN plans pl ON pl.workspace_path = p.workspace_path AND pl.status = 'active'
		GROUP BY p.workspace_path, p.name
		ORDER BY total_score DESC, total_plans DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var result []ProjectSummary
	for rows.Next() {
		var ps ProjectSummary
		if err := rows.Scan(
			&ps.WorkspacePath, &ps.Name, &ps.TotalPlans, &ps.CompletedPlans,
			&ps.TotalTodos, &ps.CompletedTodos, &ps.TotalScore,
		); err != nil {
			continue
		}
		result = append(result, ps)
	}

	return result
}

// GetDayDetail returns project-grouped plan detail for a specific date.
func GetDayDetail(date time.Time) DayDetail {
	d, err := Open()
	if err != nil {
		return DayDetail{Date: date}
	}

	dateStr := date.Format("2006-01-02")
	detail := DayDetail{Date: date}

	// Count distinct plans touched and todos completed
	row := d.QueryRow(`
		SELECT
			COUNT(DISTINCT plan_id),
			COUNT(CASE WHEN event_type = 'todo_completed' THEN 1 END)
		FROM activity_events
		WHERE CAST(occurred_at AS DATE) = CAST(? AS DATE)
		  AND plan_id IS NOT NULL AND plan_id != ''`,
		dateStr)
	_ = row.Scan(&detail.PlansTouched, &detail.TodosCompleted)

	// Get plans grouped by project
	rows, err := d.Query(`
		SELECT
			COALESCE(pr.name, ae.workspace_path, 'unknown') AS project_name,
			ae.plan_id,
			COALESCE(pl.title, '(untitled)'),
			ae.event_type,
			COALESCE(pl.source, 'cli'),
			COALESCE(pl.completed_todos, 0),
			COALESCE(pl.total_todos, 0)
		FROM activity_events ae
		LEFT JOIN plans pl ON pl.conv_id = ae.plan_id
		LEFT JOIN projects pr ON pr.workspace_path = ae.workspace_path
		WHERE CAST(ae.occurred_at AS DATE) = CAST(? AS DATE)
		  AND ae.plan_id IS NOT NULL AND ae.plan_id != ''
		ORDER BY project_name, pl.title`,
		dateStr,
	)
	if err != nil {
		return detail
	}
	defer rows.Close()

	projectMap := make(map[string]*DayProjectDetail)
	var projectOrder []string

	for rows.Next() {
		var projName, convID, title, eventType, source string
		var completed, total int
		if err := rows.Scan(&projName, &convID, &title, &eventType, &source, &completed, &total); err != nil {
			continue
		}

		if _, ok := projectMap[projName]; !ok {
			projectMap[projName] = &DayProjectDetail{ProjectName: projName}
			projectOrder = append(projectOrder, projName)
		}

		// Deduplicate plans within a project (multiple events per plan per day)
		proj := projectMap[projName]
		found := false
		for _, existing := range proj.Plans {
			if existing.ConvID == convID {
				found = true
				break
			}
		}
		if !found {
			proj.Plans = append(proj.Plans, DayPlanEntry{
				ConvID:         convID,
				Title:          title,
				ProjectName:    projName,
				EventType:      eventType,
				Source:         source,
				CompletedTodos: completed,
				TotalTodos:     total,
			})
		}
	}

	for _, name := range projectOrder {
		detail.Projects = append(detail.Projects, *projectMap[name])
	}

	return detail
}
