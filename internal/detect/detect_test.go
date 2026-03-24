package detect

import (
	"testing"

	"github.com/inquire/tmux-overseer/internal/core"
)

func TestStatus(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected core.Status
	}{
		{
			name:     "idle with prompt at end",
			content:  "some output\n❯",
			expected: core.StatusIdle,
		},
		{
			name:     "idle with prompt on last line",
			content:  "previous command\nsome output\n\n❯",
			expected: core.StatusIdle,
		},
		{
			name:     "idle with prompt and placeholder text",
			content:  "some output\nmore text\n❯ Try \"refactor apply.go\"",
			expected: core.StatusIdle,
		},
		{
			name:     "idle with status bar and prompt",
			content:  "Model: Opus 4.6 | Cost: $1.23\n❯ Type something",
			expected: core.StatusIdle,
		},
		{
			name:     "working with interrupt hint",
			content:  "Processing...\nctrl+c to interrupt",
			expected: core.StatusWorking,
		},
		{
			name:     "working with spinner",
			content:  "Working on task ⠋",
			expected: core.StatusWorking,
		},
		{
			name:     "working with another spinner",
			content:  "Thinking ⠹",
			expected: core.StatusWorking,
		},
		{
			name:     "waiting for selection input",
			content:  "What would you like?\n1. Option A\n2. Option B\nEnter to select · ↑/↓ to navigate · Esc to cancel",
			expected: core.StatusWaitingInput,
		},
		{
			name:     "waiting for y/n input",
			content:  "Do you want to continue? [y/n]",
			expected: core.StatusWaitingInput,
		},
		{
			name:     "waiting for input (Y/n)",
			content:  "Proceed? [Y/n]",
			expected: core.StatusWaitingInput,
		},
		{
			name:     "waiting with navigation hint",
			content:  "Select an option\n↑/↓ to navigate",
			expected: core.StatusWaitingInput,
		},
		{
			name:     "unknown state",
			content:  "random text without any indicators",
			expected: core.StatusUnknown,
		},
		{
			name:     "idle with ANSI codes",
			content:  "\x1b[38;2;177;185;249m❯\x1b[39m Try something",
			expected: core.StatusIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Status(tt.content)
			if got != tt.expected {
				t.Errorf("Status() = %v (%s), want %v (%s)", got, got.Label(), tt.expected, tt.expected.Label())
			}
		})
	}
}

func TestParseCost(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected float64
	}{
		{
			name:     "simple cost",
			content:  "some text\nCost: $1.23\nmore text",
			expected: 1.23,
		},
		{
			name:     "no cost",
			content:  "no cost information",
			expected: 0,
		},
		{
			name:     "zero cost",
			content:  "Cost: $0.00",
			expected: 0,
		},
		{
			name:     "cost with ANSI",
			content:  "\x1b[38;5;70mCost: $5.67\x1b[39m",
			expected: 5.67,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCost(tt.content)
			if got != tt.expected {
				t.Errorf("ParseCost() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseModel(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name:     "opus model",
			content:  "some text\nModel: Opus 4.6 | Cost: $1.23",
			expected: "Opus 4.6",
		},
		{
			name:     "no model",
			content:  "no model information",
			expected: "",
		},
		{
			name:     "model with ANSI",
			content:  "\x1b[38;5;30mModel: Sonnet 3.5\x1b[39m | Ctx: 100",
			expected: "Sonnet 3.5",
		},
		{
			name:     "startup screen opus",
			content:  "Opus 4.6 · API Usage Billing · RealtimeBoard, Inc.",
			expected: "Opus 4.6",
		},
		{
			name:     "startup screen sonnet",
			content:  "Sonnet 3.5 · API Usage",
			expected: "Sonnet 3.5",
		},
		{
			name:     "startup screen haiku",
			content:  "Haiku 3.0 · Free tier",
			expected: "Haiku 3.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseModel(tt.content)
			if got != tt.expected {
				t.Errorf("ParseModel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsClaudeCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		expected bool
	}{
		{
			name:     "claude command",
			cmd:      "claude",
			expected: true,
		},
		{
			name:     "version number",
			cmd:      "0.5.0",
			expected: true,
		},
		{
			name:     "version 2.1.39",
			cmd:      "2.1.39",
			expected: true,
		},
		{
			name:     "not claude",
			cmd:      "bash",
			expected: false,
		},
		{
			name:     "zsh",
			cmd:      "zsh",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsClaudeCommand(tt.cmd)
			if got != tt.expected {
				t.Errorf("IsClaudeCommand() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "color codes",
			input:    "\x1b[38;2;177;185;249m❯\x1b[39m",
			expected: "❯",
		},
		{
			name:     "mixed content",
			input:    "\x1b[1mBold\x1b[0m normal \x1b[32mgreen\x1b[0m",
			expected: "Bold normal green",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripANSI(tt.input)
			if got != tt.expected {
				t.Errorf("stripANSI() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestStatusWithHook(t *testing.T) {
	// Test that StatusWithHook falls back to terminal parsing when no hook file exists
	tests := []struct {
		name     string
		paneID   string
		content  string
		expected core.Status
	}{
		{
			name:     "fallback to terminal parsing - idle",
			paneID:   "nonexistent-pane-12345",
			content:  "some output\n❯",
			expected: core.StatusIdle,
		},
		{
			name:     "fallback to terminal parsing - working",
			paneID:   "nonexistent-pane-67890",
			content:  "Processing...\nctrl+c to interrupt",
			expected: core.StatusWorking,
		},
		{
			name:     "fallback to terminal parsing - waiting",
			paneID:   "nonexistent-pane-11111",
			content:  "Continue? [y/n]",
			expected: core.StatusWaitingInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StatusWithHook(tt.paneID, tt.content)
			if got != tt.expected {
				t.Errorf("StatusWithHook() = %v (%s), want %v (%s)", got, got.Label(), tt.expected, tt.expected.Label())
			}
		})
	}
}

func TestReadHookStatus(t *testing.T) {
	// Test that ReadHookStatus returns unknown for non-existent pane
	got := ReadHookStatus("definitely-nonexistent-pane-999")
	if got != core.StatusUnknown {
		t.Errorf("ReadHookStatus() for nonexistent pane = %v, want %v", got, core.StatusUnknown)
	}
}

func TestReadHookData(t *testing.T) {
	// Test that ReadHookData returns nil for non-existent pane
	got := ReadHookData("definitely-nonexistent-pane-888")
	if got != nil {
		t.Errorf("ReadHookData() for nonexistent pane = %v, want nil", got)
	}
}

func TestCostWithHook(t *testing.T) {
	// Test that CostWithHook falls back to terminal parsing when no hook file exists
	tests := []struct {
		name     string
		paneID   string
		content  string
		expected float64
	}{
		{
			name:     "fallback to terminal parsing - with cost",
			paneID:   "nonexistent-pane-cost-1",
			content:  "Model: Opus | Cost: $5.67",
			expected: 5.67,
		},
		{
			name:     "fallback to terminal parsing - no cost",
			paneID:   "nonexistent-pane-cost-2",
			content:  "no cost info here",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CostWithHook(tt.paneID, tt.content)
			if got != tt.expected {
				t.Errorf("CostWithHook() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestModelWithHook(t *testing.T) {
	// Test that ModelWithHook falls back to terminal parsing when no hook file exists
	tests := []struct {
		name     string
		paneID   string
		content  string
		expected string
	}{
		{
			name:     "fallback to terminal parsing - with model",
			paneID:   "nonexistent-pane-model-1",
			content:  "Model: Opus 4.6 | Cost: $1.23",
			expected: "Opus 4.6",
		},
		{
			name:     "fallback to terminal parsing - no model",
			paneID:   "nonexistent-pane-model-2",
			content:  "no model info here",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModelWithHook(tt.paneID, tt.content)
			if got != tt.expected {
				t.Errorf("ModelWithHook() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestSanitizePaneID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple pane ID",
			input:    "%10",
			expected: "_10",
		},
		{
			name:     "alphanumeric",
			input:    "abc123",
			expected: "abc123",
		},
		{
			name:     "with special chars",
			input:    "session:window.pane",
			expected: "session_window_pane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePaneID(tt.input)
			if got != tt.expected {
				t.Errorf("sanitizePaneID() = %q, want %q", got, tt.expected)
			}
		})
	}
}
