package hook

import (
	"encoding/json"
	"testing"
)

func TestJSONEscape(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello`, `hello`},
		{`say "hi"`, `say \"hi\"`},
		{"line\nnew", `line\nnew`},
		{`back\slash`, `back\\slash`},
		{"tab\there", `tab\there`},
	}
	for _, tt := range tests {
		got := jsonEscape(tt.input)
		if got != tt.want {
			t.Errorf("jsonEscape(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildEventJSON(t *testing.T) {
	raw := buildEventJSON("12:30:00", "tool_start", map[string]string{
		"tool":  "Read",
		"input": "/tmp/test",
	})
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, raw)
	}
	if m["ts"] != "12:30:00" {
		t.Errorf("ts = %v", m["ts"])
	}
	if m["tool"] != "Read" {
		t.Errorf("tool = %v", m["tool"])
	}
}

func TestAppendAgentTag(t *testing.T) {
	base := `{"ts":"12:00","type":"tool_start"}`
	tagged := appendAgentTag(base, "agent-1", "explore")
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tagged), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["agent_id"] != "agent-1" {
		t.Errorf("agent_id = %v", m["agent_id"])
	}

	// Empty agent ID returns unchanged
	if appendAgentTag(base, "", "") != base {
		t.Error("empty agent ID should return unchanged")
	}
}
