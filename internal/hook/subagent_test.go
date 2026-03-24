package hook

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSubagentAdd(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "subagents.json")
	os.WriteFile(fp, []byte("[]"), 0o644)

	count, err := subagentAdd(fp, SubagentEntry{
		ID: "a1", AgentType: "explore", Description: "searching",
		Model: "opus", StartedAt: "12:00", Status: "working",
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Add a second
	count, _ = subagentAdd(fp, SubagentEntry{ID: "a2", AgentType: "shell"})
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestSubagentRemove(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "subagents.json")
	os.WriteFile(fp, []byte(`[{"id":"a1"},{"id":"a2"}]`), 0o644)

	count, err := subagentRemove(fp, "a1")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestSubagentRemoveAll(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "subagents.json")
	os.WriteFile(fp, []byte(`[{"id":"a1"}]`), 0o644)

	count, _ := subagentRemove(fp, "a1")
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestSubagentCount(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "subagents.json")
	os.WriteFile(fp, []byte(`[{"id":"a1"},{"id":"a2"},{"id":"a3"}]`), 0o644)

	if got := subagentCount(fp); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}

	// Missing file returns 0
	if got := subagentCount(filepath.Join(dir, "nope.json")); got != 0 {
		t.Errorf("missing file count = %d, want 0", got)
	}
}
