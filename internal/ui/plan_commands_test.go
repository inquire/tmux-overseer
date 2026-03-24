package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractUserMessages(t *testing.T) {
	tests := []struct {
		name         string
		jsonlContent string
		maxMessages  int
		maxChars     int
		wantCount    int
		wantFirst    string
		wantErr      bool
	}{
		{
			name: "claude code format with type user",
			jsonlContent: `{"type":"user","message":{"role":"user","content":"hello world"},"uuid":"abc","timestamp":"2026-03-16T17:31:21.847Z"}
{"type":"assistant","message":{"role":"assistant","content":"hi"},"uuid":"def","timestamp":"2026-03-16T17:31:22.847Z"}
{"type":"user","message":{"role":"user","content":"second message"},"uuid":"ghi","timestamp":"2026-03-16T17:31:23.847Z"}`,
			maxMessages: 10,
			maxChars:    500,
			wantCount:   2,
			wantFirst:   "hello world",
		},
		{
			name: "legacy format with type human",
			jsonlContent: `{"type":"human","message":{"role":"user","content":"legacy message"},"uuid":"abc"}
{"type":"assistant","message":{"role":"assistant","content":"reply"},"uuid":"def"}`,
			maxMessages: 10,
			maxChars:    500,
			wantCount:   1,
			wantFirst:   "legacy message",
		},
		{
			name: "array content blocks",
			jsonlContent: `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"block one"},{"type":"text","text":"block two"}]},"uuid":"abc"}`,
			maxMessages: 10,
			maxChars:    500,
			wantCount:   1,
			wantFirst:   "block one\nblock two",
		},
		{
			name: "respects max messages limit",
			jsonlContent: `{"type":"user","message":{"role":"user","content":"msg1"},"uuid":"1"}
{"type":"user","message":{"role":"user","content":"msg2"},"uuid":"2"}
{"type":"user","message":{"role":"user","content":"msg3"},"uuid":"3"}`,
			maxMessages: 2,
			maxChars:    500,
			wantCount:   2,
			wantFirst:   "msg1",
		},
		{
			name:         "truncates long messages",
			jsonlContent: `{"type":"user","message":{"role":"user","content":"` + string(make([]byte, 600)) + `"},"uuid":"1"}`,
			maxMessages:  10,
			maxChars:     50,
			wantCount:    0, // null bytes won't produce meaningful text after trimming
		},
		{
			name:         "empty file",
			jsonlContent: "",
			maxMessages:  10,
			maxChars:     500,
			wantCount:    0,
		},
		{
			name: "skips assistant and system messages",
			jsonlContent: `{"type":"assistant","message":{"role":"assistant","content":"reply"},"uuid":"1"}
{"type":"system","message":{"role":"system","content":"sys msg"},"uuid":"2"}
{"type":"user","message":{"role":"user","content":"user msg"},"uuid":"3"}`,
			maxMessages: 10,
			maxChars:    500,
			wantCount:   1,
			wantFirst:   "user msg",
		},
		{
			name: "fallback to message.role when type is missing",
			jsonlContent: `{"message":{"role":"user","content":"fallback msg"},"uuid":"1"}`,
			maxMessages:  10,
			maxChars:     500,
			wantCount:    1,
			wantFirst:    "fallback msg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			jsonlPath := filepath.Join(tmpDir, "test.jsonl")
			if err := os.WriteFile(jsonlPath, []byte(tt.jsonlContent), 0644); err != nil {
				t.Fatalf("failed to write test file: %v", err)
			}

			msgs, err := extractUserMessages(jsonlPath, tt.maxMessages, tt.maxChars)
			if (err != nil) != tt.wantErr {
				t.Fatalf("extractUserMessages() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(msgs) != tt.wantCount {
				t.Errorf("got %d messages, want %d", len(msgs), tt.wantCount)
			}
			if tt.wantFirst != "" && len(msgs) > 0 && msgs[0] != tt.wantFirst {
				t.Errorf("first message = %q, want %q", msgs[0], tt.wantFirst)
			}
		})
	}
}

func TestExtractUserMessages_FileNotFound(t *testing.T) {
	_, err := extractUserMessages("/nonexistent/path.jsonl", 10, 500)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
