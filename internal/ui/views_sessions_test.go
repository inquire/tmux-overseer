package ui

import (
	"strings"
	"testing"

	"github.com/inquire/tmux-overseer/internal/core"
)

var testStyles = core.NewStyles(true)

func makeTestWindow(source core.SessionSource, status core.Status, cost float64) core.ClaudeWindow {
	return core.ClaudeWindow{
		SessionName: "test-session",
		WindowName:  "test-session",
		Source:      source,
		Panes: []core.ClaudePane{{
			PaneID:     "%1",
			Status:     status,
			WorkingDir: "/home/user/project",
			Model:      "claude-sonnet-4-6",
			HasGit:     true,
			GitBranch:  "main",
			Cost:       cost,
		}},
	}
}

func TestRenderSessionRowLine1_ShowsName(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 1.23)
	line := renderSessionRowLine1(" ", win, false, 80, testStyles)
	// Strip ANSI for plain-text checks
	plain := stripANSI(line)
	if !strings.Contains(plain, "test-session") {
		t.Errorf("line1 missing session name: %q", plain)
	}
}

func TestRenderSessionRowLine1_ShowsCost(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 1.23)
	line := renderSessionRowLine1(" ", win, false, 80, testStyles)
	plain := stripANSI(line)
	if !strings.Contains(plain, "$1.23") {
		t.Errorf("line1 missing cost: %q", plain)
	}
}

func TestRenderSessionRowLine1_ShowsMarker(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.TaskTodos = []core.PlanTodo{{Content: "a task", Status: "pending"}}
	collapsed := renderSessionRowLine1("▸ ", win, false, 80, testStyles)
	expanded := renderSessionRowLine1("▾ ", win, false, 80, testStyles)
	if !strings.Contains(collapsed, "▸") {
		t.Errorf("collapsed missing ▸ marker: %q", collapsed)
	}
	if !strings.Contains(expanded, "▾") {
		t.Errorf("expanded missing ▾ marker: %q", expanded)
	}
}

func TestRenderSessionRowLine2_ShowsProgress(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.ActivePlanDone = 3
	win.ActivePlanTotal = 10
	win.ActivePlanTitle = "tasks"
	line := renderSessionRowLine2(win, false, 80, testStyles)
	plain := stripANSI(line)
	if !strings.Contains(plain, "3/10") {
		t.Errorf("line2 missing progress count: %q", plain)
	}
}

func TestRenderSessionRowLine2_TaskTodosProgress(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.TaskTodos = []core.PlanTodo{
		{Content: "done", Status: "completed"},
		{Content: "done", Status: "completed"},
		{Content: "pending", Status: "pending"},
	}
	line := renderSessionRowLine2(win, false, 80, testStyles)
	plain := stripANSI(line)
	if !strings.Contains(plain, "2/3") {
		t.Errorf("line2 missing task progress 2/3: %q", plain)
	}
}

func TestRenderSessionRowLine3_ShowsLastTool(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.Panes[0].LastTool = "Bash"
	line := renderSessionRowLine3(win, testStyles)
	plain := stripANSI(line)
	if !strings.Contains(plain, "Bash") {
		t.Errorf("line3 missing last tool: %q", plain)
	}
}

func TestRenderSessionRowLine3_EmptyWhenIdle(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusIdle, 0)
	win.Panes[0].LastTool = "Bash"
	line := renderSessionRowLine3(win, testStyles)
	if line != "" {
		t.Errorf("line3 should be empty when idle: %q", line)
	}
}

func TestRenderSessionRowLine3_ShowsSubagents(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.Subagents = []core.Subagent{
		{AgentType: "Explore", Description: "find auth", CurrentTool: "Grep"},
	}
	line := renderSessionRowLine3(win, testStyles)
	plain := stripANSI(line)
	// Description is the primary label now; type is the fallback
	if !strings.Contains(plain, "find auth") {
		t.Errorf("line3 missing subagent description: %q", plain)
	}
}

func TestRenderSessionRowExpanded_TasksNumbered(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.TaskTodos = []core.PlanTodo{
		{Content: "first task", Status: "completed"},
		{Content: "second task", Status: "in_progress"},
		{Content: "third task", Status: "pending"},
	}
	expanded := renderSessionRowExpanded(win, 80, testStyles)
	plain := stripANSI(expanded)
	if !strings.Contains(plain, "1.") {
		t.Errorf("expanded missing task number 1: %q", plain)
	}
	if !strings.Contains(plain, "2.") {
		t.Errorf("expanded missing task number 2: %q", plain)
	}
	if !strings.Contains(plain, "tasks") {
		t.Errorf("expanded missing tasks header: %q", plain)
	}
}

func TestRenderSessionRowExpanded_ActivitySection(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusWorking, 0)
	win.Subagents = []core.Subagent{
		{AgentType: "Explore", Description: "find auth", CurrentTool: "Grep"},
	}
	expanded := renderSessionRowExpanded(win, 80, testStyles)
	plain := stripANSI(expanded)
	if !strings.Contains(plain, "activity") {
		t.Errorf("expanded missing activity header: %q", plain)
	}
	if !strings.Contains(plain, "find auth") {
		t.Errorf("expanded missing subagent description: %q", plain)
	}
}

func TestRenderSessionRowExpanded_EmptyWhenNoData(t *testing.T) {
	win := makeTestWindow(core.SourceCLI, core.StatusIdle, 0)
	expanded := renderSessionRowExpanded(win, 80, testStyles)
	if expanded != "" {
		t.Errorf("expanded should be empty when no tasks/subagents: %q", expanded)
	}
}

// stripANSI removes ANSI escape sequences for plain-text assertions.
func stripANSI(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
		} else {
			result.WriteByte(s[i])
			i++
		}
	}
	return result.String()
}
