package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	"github.com/inquire/tmux-overseer/internal/detect"
	"github.com/inquire/tmux-overseer/internal/tmux"
)

// schedulePreview schedules a preview update with debouncing to avoid
// spawning a subprocess on every keystroke during rapid navigation.
func (m *Model) schedulePreview() tea.Cmd {
	_, p := m.selectedWindowAndPane()
	if p == nil {
		m.previewPending = ""
		m.previewDebounced = false
		return func() tea.Msg {
			return core.PreviewMsg{Content: "", PaneID: ""}
		}
	}

	paneID := p.PaneID
	m.previewPending = paneID

	if m.previewDebounced {
		return nil
	}

	m.previewDebounced = true
	return tea.Tick(150*time.Millisecond, func(t time.Time) tea.Msg {
		return core.PreviewDebounceMsg{PaneID: paneID}
	})
}

// fetchPreview executes the actual preview capture (called after debounce).
// Cancels any previously in-flight preview subprocess to avoid wasted work.
func (m *Model) fetchPreview(paneID string) tea.Cmd {
	if m.previewCancel != nil {
		m.previewCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.previewCancel = cancel

	w, _ := m.selectedWindowAndPane()
	source := core.SourceCLI
	convID := ""
	var win *core.ClaudeWindow
	if w != nil {
		source = w.Source
		convID = w.ConversationID
		win = w
	}

	if source == core.SourceCloud && w != nil {
		content := renderCloudPreview(w, m.styles)
		return func() tea.Msg {
			return core.PreviewMsg{Content: content, PaneID: paneID}
		}
	}

	styles := m.styles
	return func() tea.Msg {
		defer cancel()
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var content string
		switch {
		case source == core.SourceCursor && convID != "":
			events := detect.ReadCursorEventsRaw(convID, 60)
			header := renderPreviewHeader(win, styles)
			body := renderEventPreview(events, win, styles)
			if body == "" {
				body = detect.ReadCursorActivityLog(convID, 20)
			}
			content = joinPreviewSections(header, body)

		case source == core.SourceCLI:
			events := detect.ReadCLIEventsRaw(paneID, 60)
			header := renderPreviewHeader(win, styles)
			if len(events) > 0 {
				body := renderEventPreview(events, win, styles)
				content = joinPreviewSections(header, body)
			} else {
				// Fall back to raw pane capture; strip ANSI before display.
				raw := tmux.CapturePaneContentCtx(ctx, paneID, 25)
				raw = detect.StripANSI(raw)
				content = joinPreviewSections(header, raw)
			}

		default:
			raw := tmux.CapturePaneContentCtx(ctx, paneID, 25)
			raw = detect.StripANSI(raw)
			content = raw
		}

		select {
		case <-ctx.Done():
			return nil
		default:
		}
		return core.PreviewMsg{Content: content, PaneID: paneID}
	}
}

// previewCmd is the direct preview command (used for initial load and after refresh).
// Also cancels any in-flight preview to avoid stale results.
func (m *Model) previewCmd() tea.Cmd {
	w, p := m.selectedWindowAndPane()
	if p == nil {
		return func() tea.Msg {
			return core.PreviewMsg{Content: "", PaneID: ""}
		}
	}

	if m.previewCancel != nil {
		m.previewCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.previewCancel = cancel

	paneID := p.PaneID
	source := core.SourceCLI
	convID := ""
	var win *core.ClaudeWindow
	if w != nil {
		source = w.Source
		convID = w.ConversationID
		win = w
	}

	if source == core.SourceCloud && w != nil {
		content := renderCloudPreview(w, m.styles)
		return func() tea.Msg {
			return core.PreviewMsg{Content: content, PaneID: paneID}
		}
	}

	styles := m.styles
	return func() tea.Msg {
		defer cancel()

		var content string
		switch {
		case source == core.SourceCursor && convID != "":
			events := detect.ReadCursorEventsRaw(convID, 60)
			header := renderPreviewHeader(win, styles)
			body := renderEventPreview(events, win, styles)
			if body == "" {
				body = detect.ReadCursorActivityLog(convID, 20)
			}
			content = joinPreviewSections(header, body)

		case source == core.SourceCLI:
			events := detect.ReadCLIEventsRaw(paneID, 60)
			header := renderPreviewHeader(win, styles)
			if len(events) > 0 {
				body := renderEventPreview(events, win, styles)
				content = joinPreviewSections(header, body)
			} else {
				raw := tmux.CapturePaneContentCtx(ctx, paneID, 25)
				raw = detect.StripANSI(raw)
				content = joinPreviewSections(header, raw)
			}

		default:
			raw := tmux.CapturePaneContentCtx(ctx, paneID, 25)
			raw = detect.StripANSI(raw)
			content = raw
		}

		select {
		case <-ctx.Done():
			return nil
		default:
		}
		return core.PreviewMsg{Content: content, PaneID: paneID}
	}
}

// joinPreviewSections concatenates a header and body with a separator line.
// If either is empty, the separator is omitted.
func joinPreviewSections(header, body string) string {
	header = strings.TrimSpace(header)
	body = strings.TrimSpace(body)
	switch {
	case header == "" && body == "":
		return ""
	case header == "":
		return body
	case body == "":
		return header
	default:
		return header + "\n" + body
	}
}

// renderEventPreview formats a JSONL event log into a styled, hierarchical preview.
// Prompts anchor each "turn" with a dim timestamp; tools are paired on one line;
// responses flow below their turn without extra prefixes.
// Events tagged with agent_id are rendered nested under their subagent_start/stop block.
func renderEventPreview(events []core.CursorEvent, w *core.ClaudeWindow, s core.Styles) string {
	if len(events) == 0 {
		return ""
	}

	var (
		tsStyle        = lipgloss.NewStyle().Foreground(s.ColorDim)
		promptStyle    = lipgloss.NewStyle().Foreground(s.ColorWhite).Bold(true)
		toolStyle      = lipgloss.NewStyle().Foreground(s.ColorCyan)
		resultStyle    = lipgloss.NewStyle().Foreground(s.ColorDim)
		resultOkStyle  = lipgloss.NewStyle().Foreground(s.ColorGreen)
		resultErrStyle = lipgloss.NewStyle().Foreground(s.ColorRed)
		responseStyle  = lipgloss.NewStyle().Foreground(s.NormalRowStyle.GetForeground())
		fileStyle      = lipgloss.NewStyle().Foreground(s.ColorYellow)
		subStyle       = lipgloss.NewStyle().Foreground(s.ColorPurple)
		dimStyle       = lipgloss.NewStyle().Foreground(s.ColorDim)
	)

	baseIndent := "         "

	// Track active subagent nesting depth for │ connectors.
	var subagentStack []string // stack of active agent_ids
	inSubagent := func() bool { return len(subagentStack) > 0 }
	currentIndent := func() string {
		if !inSubagent() {
			return baseIndent
		}
		return baseIndent + "  " + dimStyle.Render("│") + " "
	}

	var lines []string
	i := 0
	for i < len(events) {
		e := events[i]
		indent := currentIndent()

		switch e.Type {
		case "session_start":
			lines = append(lines, dimStyle.Render("  · session started"))
			i++

		case "prompt":
			ts := tsStyle.Render("  " + e.Timestamp + "  ")
			text := truncateEvent(e.Text, 120)
			lines = append(lines, ts+promptStyle.Render("❯ "+text))
			i++

		case "tool_start":
			toolName := e.Tool
			inputShort := formatToolInput(e.Tool, e.Input)
			if i+1 < len(events) && events[i+1].Type == "tool_result" {
				result := events[i+1]
				outputShort, ok := formatToolOutput(result.Output)
				if ok {
					line := indent + toolStyle.Render("▸ "+toolName)
					if inputShort != "" {
						line += dimStyle.Render("(" + inputShort + ")")
					}
					line += " " + resultOkStyle.Render("→") + " " + resultStyle.Render(outputShort)
					lines = append(lines, line)
				} else {
					line := indent + toolStyle.Render("▸ "+toolName)
					if inputShort != "" {
						line += dimStyle.Render("(" + inputShort + ")")
					}
					lines = append(lines, line)
				}
				i += 2
			} else {
				line := indent + toolStyle.Render("▸ "+toolName)
				if inputShort != "" {
					line += dimStyle.Render("(" + inputShort + ")")
				}
				lines = append(lines, line)
				i++
			}

		case "tool_result":
			outputShort, _ := formatToolOutput(e.Output)
			if outputShort != "" {
				lines = append(lines, indent+dimStyle.Render("  → "+outputShort))
			}
			i++

		case "response":
			if e.Text != "" {
				text := truncateEvent(e.Text, 200)
				lines = append(lines, indent+responseStyle.Render(text))
			}
			i++

		case "thought":
			lines = append(lines, indent+dimStyle.Render("· thinking..."))
			i++

		case "file_edit":
			name := filepath.Base(e.Path)
			if name == "" || name == "." {
				name = e.Path
			}
			label := fileStyle.Render("✏ " + name)
			if e.Summary != "" {
				label += "  " + dimStyle.Render(truncateEvent(e.Summary, 60))
			}
			lines = append(lines, indent+label)
			i++

		case "shell_result":
			cmd := truncateEvent(e.Command, 50)
			exitStr := e.ExitCode
			var exitRendered string
			if exitStr == "0" || exitStr == "" {
				exitRendered = resultOkStyle.Render("exit " + exitStr)
			} else {
				exitRendered = resultErrStyle.Render("exit " + exitStr)
			}
			lines = append(lines, indent+toolStyle.Render("$ "+cmd)+"  "+exitRendered)
			i++

		case "subagent_start":
			agentType := e.Tool
			if agentType == "" {
				agentType = e.AgentType
			}
			if agentType == "" {
				agentType = "agent"
			}
			desc := truncateEvent(e.Description, 60)
			line := baseIndent + subStyle.Render("◆ "+agentType)
			if e.Model != "" {
				line += " " + dimStyle.Render("("+shortenModel(e.Model)+")")
			}
			if desc != "" {
				line += "  " + dimStyle.Render("\""+desc+"\"")
			}
			lines = append(lines, line)
			if e.AgentID != "" {
				subagentStack = append(subagentStack, e.AgentID)
			}
			i++

		case "subagent_stop":
			if len(subagentStack) > 0 {
				subagentStack = subagentStack[:len(subagentStack)-1]
			}
			agentType := e.Tool
			if agentType == "" {
				agentType = e.AgentType
			}
			label := "done"
			if agentType != "" {
				label = agentType + " done"
			}
			if e.Summary != "" {
				label += ": " + truncateEvent(e.Summary, 80)
			}
			lines = append(lines, baseIndent+subStyle.Render("◆ "+label))
			i++

		case "compact":
			lines = append(lines, indent+dimStyle.Render("⟳ context compacted"))
			i++

		case "stop":
			reason := e.Reason
			if reason == "" {
				reason = "stopped"
			}
			lines = append(lines, indent+dimStyle.Render("■ "+reason))
			i++

		default:
			if e.Type != "" {
				lines = append(lines, indent+dimStyle.Render("· "+e.Type))
			}
			i++
		}
	}

	return strings.Join(lines, "\n")
}

// renderPreviewHeader builds a one-line session summary shown above the event log.
func renderPreviewHeader(w *core.ClaudeWindow, s core.Styles) string {
	if w == nil {
		return ""
	}

	p := w.PrimaryPane()
	status := core.StatusUnknown
	modelStr := ""
	if p != nil {
		status = p.Status
		modelStr = p.Model
	}

	var parts []string

	// Status dot + label
	statusSymbol := s.StatusStyle(status).Render(status.Symbol())
	statusLabel := s.StatusStyle(status).Render(status.Label())
	parts = append(parts, statusSymbol+" "+statusLabel)

	// Model (shortened)
	if modelStr != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(s.ColorRed).Render(shortenModel(modelStr)))
	}

	// Effort level
	if w.EffortLevel != "" {
		parts = append(parts, s.EffortLevelStyle.Render(effortSymbol(w.EffortLevel)))
	}

	// Cost
	if cost := w.TotalCost(); cost > 0 {
		parts = append(parts, s.CostStyle.Render(fmt.Sprintf("$%.2f", cost)))
	}

	// Tool + prompt counts
	if w.ToolCount > 0 {
		parts = append(parts, s.DimRowStyle.Render(fmt.Sprintf("%d tools", w.ToolCount)))
	}
	if w.PromptCount > 0 {
		parts = append(parts, s.DimRowStyle.Render(fmt.Sprintf("%d prompts", w.PromptCount)))
	}

	// Duration
	if dur := w.SessionDuration(); dur != "" {
		parts = append(parts, s.DimRowStyle.Render(dur))
	}

	// Agent/permission mode badges
	switch w.AgentMode {
	case "plan":
		parts = append(parts, s.PlanModeBadgeStyle.Render("[PLAN]"))
	case "agent":
		parts = append(parts, s.AgentModeBadgeStyle.Render("[AGENT]"))
	}

	return "  " + strings.Join(parts, "  ")
}

// formatToolInput returns a compact representation of a tool's input for display.
// For file-path tools it shows just the filename; for commands it shows the command.
func formatToolInput(tool, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	switch tool {
	case "Read", "Write", "Edit", "MultiEdit", "NotebookRead", "NotebookEdit":
		// Extract file path from JSON {"file_path":"..."} or plain path
		if path := extractJSONString(input, "file_path"); path != "" {
			return filepath.Base(path)
		}
		return filepath.Base(input)
	case "Bash", "computer":
		cmd := extractJSONString(input, "command")
		if cmd == "" {
			cmd = input
		}
		return truncateEvent(cmd, 45)
	case "Grep", "GlobTool":
		pattern := extractJSONString(input, "pattern")
		if pattern == "" {
			pattern = input
		}
		return truncateEvent(pattern, 40)
	case "WebSearch", "WebFetch":
		q := extractJSONString(input, "query")
		if q == "" {
			q = extractJSONString(input, "url")
		}
		if q == "" {
			q = input
		}
		return truncateEvent(q, 40)
	default:
		return truncateEvent(input, 40)
	}
}

// formatToolOutput returns a compact one-line summary of a tool's output.
// ok is false when the output is empty (tool returned nothing useful).
func formatToolOutput(output string) (string, bool) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", false
	}

	// Count lines for context
	lines := strings.Split(output, "\n")
	lineCount := len(lines)

	// Single-line outputs: show directly
	if lineCount == 1 {
		return truncateEvent(output, 80), true
	}

	// Multi-line: show first non-empty line + line count
	first := ""
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			first = l
			break
		}
	}
	summary := truncateEvent(first, 60)
	if lineCount > 1 {
		summary += fmt.Sprintf("  (%d lines)", lineCount)
	}
	return summary, true
}

// extractJSONString naively extracts a string value for a given key from JSON.
// Used to avoid a full json.Unmarshal for display-only purposes.
func extractJSONString(json, key string) string {
	needle := `"` + key + `":`
	idx := strings.Index(json, needle)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimSpace(json[idx+len(needle):])
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// truncateEvent trims and collapses whitespace in an event string for single-line display.
func truncateEvent(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	// Collapse multiple spaces
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	if len(s) > maxLen {
		return s[:maxLen] + "…"
	}
	return s
}

func renderCloudPreview(w *core.ClaudeWindow, s core.Styles) string {
	var b strings.Builder

	status := "unknown"
	if p := w.PrimaryPane(); p != nil {
		status = p.Status.Label()
	}
	fmt.Fprintf(&b, "☁  Cloud Agent  [%s]\n", strings.ToUpper(status))
	b.WriteString(strings.Repeat("─", 40) + "\n")

	if w.WorkspacePath != "" {
		fmt.Fprintf(&b, "  Workspace:  %s\n", w.WorkspacePath)
	}
	if p := w.PrimaryPane(); p != nil && p.GitBranch != "" {
		fmt.Fprintf(&b, "  Branch:     %s\n", p.GitBranch)
	}
	if dur := w.SessionDuration(); dur != "" {
		fmt.Fprintf(&b, "  Duration:   %s\n", dur)
	}

	if w.CloudSummary != "" {
		b.WriteString("\n  Summary:\n")
		for _, line := range wrapText(w.CloudSummary, 60) {
			fmt.Fprintf(&b, "    %s\n", line)
		}
	}

	if w.CloudPRURL != "" {
		fmt.Fprintf(&b, "\n  PR:    %s\n", w.CloudPRURL)
	}
	if w.CloudAgentURL != "" {
		fmt.Fprintf(&b, "  URL:   %s\n", w.CloudAgentURL)
	}

	return b.String()
}

func wrapText(text string, width int) []string {
	if len(text) <= width {
		return []string{text}
	}
	var lines []string
	for len(text) > width {
		cut := width
		if idx := strings.LastIndex(text[:cut], " "); idx > 0 {
			cut = idx
		}
		lines = append(lines, text[:cut])
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		lines = append(lines, text)
	}
	return lines
}
