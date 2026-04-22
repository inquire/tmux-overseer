package plans

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/inquire/tmux-overseer/internal/core"
	iexec "github.com/inquire/tmux-overseer/internal/exec"
	"github.com/inquire/tmux-overseer/internal/state"
)

const titleOverridesFile = "plan-title-overrides.json"

var claudeBinary struct {
	sync.Once
	path string
}

// resolveClaudeBinary finds the claude CLI binary, checking PATH first
// then common install locations.
func resolveClaudeBinary() string {
	claudeBinary.Do(func() {
		if p, err := osexec.LookPath("claude"); err == nil {
			claudeBinary.path = p
			return
		}
		home := state.CachedHomeDir()
		if home == "" {
			return
		}
		candidates := []string{
			filepath.Join(home, ".local", "bin", "claude"),
			filepath.Join(home, ".npm-global", "bin", "claude"),
			"/usr/local/bin/claude",
			"/opt/homebrew/bin/claude",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				claudeBinary.path = c
				return
			}
		}
	})
	return claudeBinary.path
}

func titleOverridesPath() string {
	home := state.CachedHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude-tmux", titleOverridesFile)
}

// LoadTitleOverrides reads the persistent title overrides from disk.
// Returns an empty map (never nil) if the file doesn't exist or can't be parsed.
func LoadTitleOverrides() map[string]string {
	path := titleOverridesPath()
	if path == "" {
		return make(map[string]string)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return make(map[string]string)
	}
	var overrides map[string]string
	if err := json.Unmarshal(data, &overrides); err != nil {
		return make(map[string]string)
	}
	return overrides
}

// SaveTitleOverride atomically merges a single title override into the persisted file.
func SaveTitleOverride(convID, title string) error {
	path := titleOverridesPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	overrides := LoadTitleOverrides()
	overrides[convID] = title

	data, err := json.MarshalIndent(overrides, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ApplyTitleOverrides replaces plan titles with any persisted overrides.
func ApplyTitleOverrides(plans []core.PlanEntry, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	for i := range plans {
		if title, ok := overrides[plans[i].ConvID]; ok {
			plans[i].Title = title
		}
	}
}

// ExtractConversationContext extracts a short summary of conversation content
// suitable for sending to an LLM for title generation.
func ExtractConversationContext(plan core.PlanEntry) string {
	if plan.Source == core.SourceCursor {
		return extractCursorContext(plan)
	}
	return extractClaudeContext(plan.FilePath)
}

func extractCursorContext(plan core.PlanEntry) string {
	var parts []string
	if plan.Overview != "" {
		parts = append(parts, "Overview: "+plan.Overview)
	}
	for _, t := range plan.Todos {
		parts = append(parts, "- "+t.Content)
	}
	text := strings.Join(parts, "\n")
	if len(text) > 500 {
		text = text[:500]
	}
	return text
}

func extractClaudeContext(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var messages []string
	const maxMessages = 5

	for scanner.Scan() {
		if len(messages) >= maxMessages {
			break
		}
		var msg claudeMessage
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			continue
		}
		if msg.Type != "user" {
			continue
		}
		text := extractTextContent(msg.Message.Content)
		if text == "" {
			continue
		}
		if strings.HasPrefix(text, "<local-command") || strings.HasPrefix(text, "<command") {
			continue
		}
		text = strings.TrimSpace(text)
		if len(text) > 200 {
			text = text[:200]
		}
		if len(text) > 3 {
			messages = append(messages, text)
		}
	}

	return strings.Join(messages, "\n---\n")
}

// GenerateTitle calls `claude -p` to generate a concise title from conversation context.
// This is a blocking call intended to be run inside a tea.Cmd.
func GenerateTitle(plan core.PlanEntry) (string, error) {
	claudePath := resolveClaudeBinary()
	if claudePath == "" {
		return "", fmt.Errorf("claude CLI not found in PATH or common locations")
	}

	ctx := ExtractConversationContext(plan)
	if ctx == "" {
		return "", fmt.Errorf("no conversation context found")
	}

	prompt := fmt.Sprintf(
		"Generate a concise 3-6 word title for this conversation. "+
			"Output ONLY the title, nothing else. No quotes, no punctuation at the end.\n\n"+
			"Conversation:\n%s", ctx)

	out, err := iexec.RunWithTimeout(30*time.Second, claudePath, "-p",
		"--no-session-persistence",
		"--max-budget-usd", "0.01",
		prompt)
	if err != nil {
		return "", fmt.Errorf("claude -p failed: %w", err)
	}

	title := strings.TrimSpace(string(out))
	title = strings.Trim(title, "\"'")
	if title == "" {
		return "", fmt.Errorf("empty title returned")
	}
	if len(title) > 60 {
		title = title[:60]
	}
	return title, nil
}

// GenerateTitleCmd returns a tea.Cmd that generates a title for a single plan.
func GenerateTitleCmd(plan core.PlanEntry) tea.Cmd {
	return func() tea.Msg {
		title, err := GenerateTitle(plan)
		if err != nil {
			return core.TitleGeneratedMsg{ConvID: plan.ConvID, Err: err}
		}
		if saveErr := SaveTitleOverride(plan.ConvID, title); saveErr != nil {
			return core.TitleGeneratedMsg{ConvID: plan.ConvID, NewTitle: title, Err: saveErr}
		}
		return core.TitleGeneratedMsg{ConvID: plan.ConvID, NewTitle: title}
	}
}
