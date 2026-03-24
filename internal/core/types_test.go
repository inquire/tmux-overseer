package core

import "testing"

func TestNextTodo(t *testing.T) {
	tests := []struct {
		name  string
		todos []PlanTodo
		want  string
	}{
		{
			name:  "empty",
			todos: nil,
			want:  "",
		},
		{
			name: "in_progress first",
			todos: []PlanTodo{
				{Content: "done", Status: "completed"},
				{Content: "doing", Status: "in_progress"},
				{Content: "later", Status: "pending"},
			},
			want: "doing",
		},
		{
			name: "pending when no in_progress",
			todos: []PlanTodo{
				{Content: "done", Status: "completed"},
				{Content: "todo", Status: "pending"},
			},
			want: "todo",
		},
		{
			name: "all completed",
			todos: []PlanTodo{
				{Content: "done", Status: "completed"},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PlanEntry{Todos: tt.todos}
			if got := p.NextTodo(); got != tt.want {
				t.Errorf("NextTodo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSortedTodos(t *testing.T) {
	p := PlanEntry{
		Todos: []PlanTodo{
			{Content: "pending1", Status: "pending"},
			{Content: "done1", Status: "completed"},
			{Content: "doing1", Status: "in_progress"},
			{Content: "cancel1", Status: "cancelled"},
		},
	}
	sorted := p.SortedTodos()
	wantOrder := []string{"completed", "in_progress", "pending", "cancelled"}
	for i, want := range wantOrder {
		if sorted[i].Status != want {
			t.Errorf("SortedTodos()[%d].Status = %q, want %q", i, sorted[i].Status, want)
		}
	}
}

func TestCompletionPct(t *testing.T) {
	tests := []struct {
		name  string
		todos []PlanTodo
		want  int
	}{
		{"empty", nil, 0},
		{"half", []PlanTodo{
			{Status: "completed"}, {Status: "pending"},
		}, 50},
		{"all done", []PlanTodo{
			{Status: "completed"}, {Status: "completed"},
		}, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := PlanEntry{Todos: tt.todos}
			if got := p.CompletionPct(); got != tt.want {
				t.Errorf("CompletionPct() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTagPillStyle(t *testing.T) {
	// Same tag should always produce the same style
	s1 := TagPillStyle("refactor")
	s2 := TagPillStyle("refactor")
	if s1.GetForeground() != s2.GetForeground() {
		t.Error("TagPillStyle should be deterministic for the same tag")
	}

	// Different tags should (usually) produce different styles
	s3 := TagPillStyle("auth")
	if s1.GetForeground() == s3.GetForeground() {
		t.Log("TagPillStyle: 'refactor' and 'auth' happened to collide (acceptable)")
	}
}
