package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
)

// startRestructureMsg signals that a restructure was confirmed and should begin.
type startRestructureMsg struct {
	FilePath string
}

// startConvertMsg signals that a JSONL conversation conversion was confirmed.
type startConvertMsg struct {
	FilePath  string
	Workspace string
}

// findClaude returns the full path to the claude binary.
func findClaude() string {
	// Check common locations
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".local", "bin", "claude"),
		"/usr/local/bin/claude",
		"/opt/homebrew/bin/claude",
		"claude", // fallback to PATH
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "claude"
}

// restructurePlanCmd runs `claude -p --model haiku` to restructure a plan file.
// Uses stdin piping to avoid "argument list too long" errors.
func restructurePlanCmd(filePath string) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(filePath)
		if err != nil {
			return core.RestructurePlanMsg{FilePath: filePath, Err: err}
		}

		// Load plan conventions so haiku has the full context
		home, _ := os.UserHomeDir()
		conventions := ""
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "plan.md")); err == nil {
			conventions = string(data)
		}
		claudeMd := ""
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md")); err == nil {
			claudeMd = string(data)
		}

		prompt := "You are restructuring a plan file. Follow these conventions EXACTLY.\n\n" +
			"## CLAUDE.md conventions:\n" + claudeMd + "\n\n" +
			"## plan.md conventions:\n" + conventions + "\n\n" +
			"## Instructions:\n" +
			"Restructure the plan below to match the conventions above. " +
			"Keep all existing content and progress (don't reset completed todos). " +
			"Ensure frontmatter has name, overview (with repo name), and tags. " +
			"Ensure todos are actionable. Add missing sections (What We're Building, Verification) if absent. " +
			"Output ONLY the restructured plan markdown, nothing else.\n\n" +
			"## Plan to restructure:\n" + string(content)

		claudeBin := findClaude()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cmd := osexec.CommandContext(ctx, claudeBin, "-p", "--model", "haiku")
		cmd.Stdin = strings.NewReader(prompt)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			errMsg := err.Error()
			if stderr.Len() > 0 {
				errMsg += ": " + stderr.String()
			}
			return core.RestructurePlanMsg{FilePath: filePath, Err: fmt.Errorf("%s", errMsg)}
		}
		out := []byte(stdout.String())
		if err := os.WriteFile(filePath, out, 0644); err != nil {
			return core.RestructurePlanMsg{FilePath: filePath, Err: err}
		}
		return core.RestructurePlanMsg{FilePath: filePath}
	}
}

// extractUserMessages reads a JSONL file and returns the first maxMessages user messages,
// each truncated to maxChars characters.
func extractUserMessages(jsonlPath string, maxMessages, maxChars int) ([]string, error) {
	f, err := os.Open(jsonlPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var messages []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer
	for scanner.Scan() && len(messages) < maxMessages {
		var entry struct {
			Role    string `json:"role"`
			Type    string `json:"type"`
			Message struct {
				Role    string `json:"role"`
				Content any    `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		// Claude Code JSONL uses "type":"user" at top level with message.role="user"
		role := entry.Role
		if role == "" {
			if entry.Type == "user" || entry.Type == "human" || entry.Message.Role == "user" {
				role = "user"
			}
		}
		if role != "user" {
			continue
		}

		// Extract text content
		var text string
		switch c := entry.Message.Content.(type) {
		case string:
			text = c
		case []any:
			for _, block := range c {
				if m, ok := block.(map[string]any); ok {
					if t, ok := m["text"].(string); ok {
						text += t + "\n"
					}
				}
			}
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		if len(text) > maxChars {
			text = text[:maxChars] + "..."
		}
		messages = append(messages, text)
	}
	return messages, scanner.Err()
}

// convertConversationCmd converts a JSONL conversation into a .plan.md file using haiku.
func convertConversationCmd(jsonlPath, workspace string) tea.Cmd {
	return func() tea.Msg {
		messages, err := extractUserMessages(jsonlPath, 10, 500)
		if err != nil {
			return core.ConvertConversationMsg{OriginalPath: jsonlPath, Err: err}
		}
		if len(messages) == 0 {
			return core.ConvertConversationMsg{OriginalPath: jsonlPath, Err: fmt.Errorf("no user messages found in conversation")}
		}

		// Load conventions
		home, _ := os.UserHomeDir()
		conventions := ""
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "plan.md")); err == nil {
			conventions = string(data)
		}
		claudeMd := ""
		if data, err := os.ReadFile(filepath.Join(home, ".claude", "CLAUDE.md")); err == nil {
			claudeMd = string(data)
		}

		// Build conversation summary
		var convBuf strings.Builder
		for i, msg := range messages {
			fmt.Fprintf(&convBuf, "Message %d:\n%s\n\n", i+1, msg)
		}

		today := time.Now().Format("2006-01-02")
		prompt := "Convert this conversation into a structured .plan.md file.\n\n" +
			"## CLAUDE.md conventions (for plan naming, frontmatter, etc.):\n" + claudeMd + "\n\n" +
			"## plan.md conventions:\n" + conventions + "\n\n" +
			"## Instructions:\n" +
			"Generate a .plan.md file from the conversation messages below.\n" +
			"- The workspace/repo is: " + workspace + "\n" +
			"- Today's date is: " + today + "\n" +
			"- Include frontmatter with: name, overview (mentioning the repo), tags\n" +
			"- Include actionable todos based on what the conversation discusses\n" +
			"- The name should be a short descriptive slug (3-6 words, lowercase, hyphenated)\n" +
			"- Output ONLY the plan markdown, nothing else.\n\n" +
			"## Conversation messages:\n" + convBuf.String()

		claudeBin := findClaude()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		cmd := osexec.CommandContext(ctx, claudeBin, "-p", "--model", "haiku")
		cmd.Stdin = strings.NewReader(prompt)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err = cmd.Run()
		if err != nil {
			errMsg := err.Error()
			if stderr.Len() > 0 {
				errMsg += ": " + stderr.String()
			}
			return core.ConvertConversationMsg{OriginalPath: jsonlPath, Err: fmt.Errorf("%s", errMsg)}
		}

		output := stdout.String()

		// Extract name from frontmatter to build filename slug
		slug := "converted-plan"
		nameRe := regexp.MustCompile(`(?m)^name:\s*(.+)$`)
		if matches := nameRe.FindStringSubmatch(output); len(matches) > 1 {
			raw := strings.TrimSpace(matches[1])
			// Clean up: lowercase, replace spaces with hyphens, remove non-alphanumeric
			raw = strings.ToLower(raw)
			raw = strings.ReplaceAll(raw, " ", "-")
			cleaned := regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(raw, "")
			cleaned = regexp.MustCompile(`-+`).ReplaceAllString(cleaned, "-")
			cleaned = strings.Trim(cleaned, "-")
			if cleaned != "" {
				slug = cleaned
			}
		}

		// Extract title for the flash message
		title := slug
		if matches := nameRe.FindStringSubmatch(output); len(matches) > 1 {
			title = strings.TrimSpace(matches[1])
		}

		filename := today + "-" + slug + ".plan.md"
		plansDir := filepath.Join(home, ".claude", "plans")
		_ = os.MkdirAll(plansDir, 0755)
		newPath := filepath.Join(plansDir, filename)

		if err := os.WriteFile(newPath, []byte(output), 0644); err != nil {
			return core.ConvertConversationMsg{OriginalPath: jsonlPath, Err: err}
		}

		return core.ConvertConversationMsg{OriginalPath: jsonlPath, NewPath: newPath, Title: title}
	}
}

