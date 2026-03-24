// Package hookserver provides an HTTP server that receives hook events from
// Claude Code and Cursor, enabling real-time status updates without waiting
// for the 5-second polling cycle.
//
// Claude Code (v2.1.63+) supports "url" hooks that POST JSON to a URL:
//
//	{"type": "url", "url": "http://localhost:28721/hook"}
//
// The server does two things on each POST:
//  1. Writes the same status JSON file the shell script would write (persistent
//     cache layer for restarts and the staleness-checked file-poll fallback).
//  2. Sends a HookEventMsg into the Bubble Tea event loop so the UI updates
//     immediately without waiting for the next poll tick.
//
// The shell script hooks remain registered alongside the URL hooks so that
// status files are also written when the TUI is not running.
package hookserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
)

const DefaultPort = 28721

// hookPayload is the raw JSON received from a Claude Code or Cursor hook POST.
// Both CLIs send the same fields; Cursor adds conversation_id and workspace_roots.
type hookPayload struct {
	// Common fields
	HookEventName string `json:"hook_event_name"`
	SessionID     string `json:"session_id"`
	Model         string `json:"model"`
	CWD           string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"` // raw JSON; used to extract summary
	AgentID       string          `json:"agent_id"`
	AgentType     string          `json:"agent_type"`
	ParentAgentID string          `json:"parent_agent_id"`
	Description   string          `json:"description"`
	EffortLevel   string `json:"effort"`

	// Worktree info (v2.1.69+)
	Worktree *worktreePayload `json:"worktree"`

	// Cursor-specific
	ConversationID string   `json:"conversation_id"`
	WorkspaceRoots []string `json:"workspace_roots"`
	CursorVersion  string   `json:"cursor_version"`
}

type worktreePayload struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	Branch       string `json:"branch"`
	OriginalRepo string `json:"originalRepo"`
}

// Server receives hook POSTs from Claude Code/Cursor and bridges them into
// the Bubble Tea event loop.
type Server struct {
	port      int
	events    chan<- core.HookEventMsg
	statusDir string
	srv       *http.Server
}

// New creates a new Server. events is written to when a hook arrives;
// the Bubble Tea model should drain this channel via waitForHookCmd.
// statusDir is ~/.claude-tmux (where status JSON files are written).
func New(port int, events chan<- core.HookEventMsg, statusDir string) *Server {
	return &Server{
		port:      port,
		events:    events,
		statusDir: statusDir,
	}
}

// Start starts the HTTP server. It returns the actual listening port (useful
// when port 0 is passed to pick a random available port) and any startup error.
// The server shuts down when ctx is cancelled.
func (s *Server) Start(ctx context.Context) (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/hook", s.handleHook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	listener, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.port))
	if err != nil {
		return 0, err
	}
	s.port = listener.Addr().(*net.TCPAddr).Port

	s.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	go func() { _ = s.srv.Serve(listener) }()
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutCtx)
	}()

	// Write the port to a file so setup scripts can discover it.
	s.writePortFile()

	return s.port, nil
}

// Port returns the port the server is listening on (valid after Start).
func (s *Server) Port() int { return s.port }

// writePortFile persists the listening port so shell setup scripts can
// register URL hooks pointing at the running TUI instance.
func (s *Server) writePortFile() {
	if s.statusDir == "" {
		return
	}
	_ = os.MkdirAll(s.statusDir, 0700)
	path := filepath.Join(s.statusDir, ".hookserver-port")
	_ = os.WriteFile(path, []byte(strconv.Itoa(s.port)), 0600)
}

// RemovePortFile removes the port file on clean shutdown so stale port
// files don't cause setup scripts to register hooks to a dead server.
func (s *Server) RemovePortFile() {
	if s.statusDir != "" {
		_ = os.Remove(filepath.Join(s.statusDir, ".hookserver-port"))
	}
}

func (s *Server) handleHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var p hookPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Write the status file (persistent cache, same role as the shell script).
	s.writeStatusFile(p)

	// Update per-subagent tool activity when a tool event arrives for a subagent.
	if p.AgentID != "" {
		switch p.HookEventName {
		case "PreToolUse", "preToolUse":
			s.updateSubagentTool(p, true)
		case "PostToolUse", "PostToolUseFailure", "postToolUse":
			s.updateSubagentTool(p, false)
		}
	}

	// Build and deliver the Bubble Tea message (non-blocking drop if full).
	msg := payloadToMsg(p)
	select {
	case s.events <- msg:
	default:
	}

	w.WriteHeader(http.StatusOK)
}

// payloadToMsg converts a raw hook payload into a HookEventMsg for the UI.
func payloadToMsg(p hookPayload) core.HookEventMsg {
	msg := core.HookEventMsg{
		SessionID:      p.SessionID,
		Event:          p.HookEventName,
		Status:         eventToStatus(p.HookEventName),
		Model:          p.Model,
		CWD:            p.CWD,
		AgentMode:      permissionToAgentMode(p.PermissionMode),
		PermissionMode: p.PermissionMode,
		EffortLevel:    p.EffortLevel,
	}

	if p.ConversationID != "" {
		msg.ConversationID = p.ConversationID
		msg.Source = "cursor"
		if len(p.WorkspaceRoots) > 0 {
			msg.CWD = p.WorkspaceRoots[0]
		}
	} else {
		msg.Source = "cli"
	}

	if p.Worktree != nil {
		msg.WorktreePath = p.Worktree.Path
		msg.WorktreeBranch = p.Worktree.Branch
		msg.OriginalRepo = p.Worktree.OriginalRepo
	}

	return msg
}

// eventToStatus maps a hook event name to a session status string.
func eventToStatus(event string) string {
	switch event {
	case "SessionStart", "Stop", "SessionEnd", "TaskCompleted", "TeammateIdle":
		return "idle"
	case "UserPromptSubmit",
		"PreToolUse", "PostToolUse", "PostToolUseFailure",
		"SubagentStart", "SubagentStop", "PreCompact",
		"beforeSubmitPrompt", "preToolUse", "postToolUse",
		"subagentStart", "subagentStop", "preCompact",
		"afterAgentResponse", "afterAgentThought",
		"afterFileEdit", "afterShellExecution":
		return "working"
	case "Notification":
		return "waiting"
	default:
		return "idle"
	}
}

func permissionToAgentMode(mode string) string {
	if mode == "plan" {
		return "plan"
	}
	return "agent"
}

// writeStatusFile writes a minimal status JSON so the file-poll path stays
// consistent and restarts can read last-known state. For CLI sessions, we
// use session_id as the key (the shell script uses TMUX_PANE, which is not
// available in the HTTP path; both files may coexist and both serve as caches).
func (s *Server) writeStatusFile(p hookPayload) {
	if s.statusDir == "" {
		return
	}

	ts := time.Now().Unix()
	status := eventToStatus(p.HookEventName)

	var fields map[string]any
	var filename string

	if p.ConversationID != "" {
		filename = "cursor-" + sanitize(p.ConversationID) + ".json"
		workspace := p.CWD
		if len(p.WorkspaceRoots) > 0 {
			workspace = p.WorkspaceRoots[0]
		}
		fields = map[string]any{
			"conversation_id": p.ConversationID,
			"source":          "cursor",
			"status":          status,
			"event":           p.HookEventName,
			"model":           p.Model,
			"workspace":       workspace,
			"cwd":             p.CWD,
			"permission_mode": p.PermissionMode,
			"agent_mode":      permissionToAgentMode(p.PermissionMode),
			"timestamp":       ts,
		}
	} else if p.SessionID != "" {
		filename = "status-session-" + sanitize(p.SessionID) + ".json"
		fields = map[string]any{
			"session_id":      p.SessionID,
			"status":          status,
			"event":           p.HookEventName,
			"model":           p.Model,
			"cwd":             p.CWD,
			"permission_mode": p.PermissionMode,
			"agent_mode":      permissionToAgentMode(p.PermissionMode),
			"timestamp":       ts,
		}
	} else {
		return
	}

	// Add worktree fields when present.
	if p.Worktree != nil {
		fields["worktree_path"] = p.Worktree.Path
		fields["worktree_branch"] = p.Worktree.Branch
		fields["original_repo"] = p.Worktree.OriginalRepo
	}
	if p.EffortLevel != "" {
		fields["effort_level"] = p.EffortLevel
	}

	data, err := json.Marshal(fields)
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(s.statusDir, filename), data, 0644)
}

func sanitize(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

// updateSubagentTool reads the appropriate .subagents.json file, updates the
// matching subagent's CurrentTool/CurrentToolInput, and writes it back.
// set=true on PreToolUse (sets activity), set=false on PostToolUse (clears it).
func (s *Server) updateSubagentTool(p hookPayload, set bool) {
	if s.statusDir == "" {
		return
	}

	var filePath string
	if p.ConversationID != "" {
		filePath = filepath.Join(s.statusDir, "cursor-"+sanitize(p.ConversationID)+".subagents.json")
	} else if p.SessionID != "" {
		filePath = filepath.Join(s.statusDir, "status-session-"+sanitize(p.SessionID)+".subagents.json")
	} else {
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	var agents []core.Subagent
	if err := json.Unmarshal(data, &agents); err != nil {
		return
	}

	now := time.Now().Format("15:04:05")
	changed := false
	for i, a := range agents {
		if a.ID != p.AgentID {
			continue
		}
		if set {
			agents[i].CurrentTool = p.ToolName
			agents[i].CurrentToolInput = toolInputSummary(p.ToolName, p.ToolInput)
			agents[i].LastActivityAt = now
		} else {
			agents[i].CurrentTool = ""
			agents[i].CurrentToolInput = ""
		}
		changed = true
		break
	}
	if !changed {
		return
	}

	out, err := json.Marshal(agents)
	if err != nil {
		return
	}
	_ = os.WriteFile(filePath, out, 0644)
}

// toolInputSummary extracts a short, human-readable summary from a tool's
// raw JSON input. Returns at most 40 characters.
func toolInputSummary(toolName string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	// Per-tool heuristics: pick the most useful field.
	key := ""
	switch strings.ToLower(toolName) {
	case "bash", "shell", "computer":
		key = "command"
	case "grep", "search":
		key = "pattern"
	case "read", "readfile", "write", "writefile", "edit":
		key = "file_path"
	case "websearch":
		key = "query"
	case "webfetch":
		key = "url"
	default:
		// Fall through to generic extraction below.
	}
	if key != "" {
		if v, ok := m[key]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err == nil {
				return truncate40(s)
			}
		}
	}
	// Generic: use first string field value.
	for _, v := range m {
		var s string
		if err := json.Unmarshal(v, &s); err == nil && s != "" {
			return truncate40(s)
		}
	}
	return ""
}

func truncate40(s string) string {
	runes := []rune(s)
	if len(runes) <= 40 {
		return s
	}
	return string(runes[:37]) + "..."
}

// ReadPort reads the port from the port file written by a running server.
// Returns 0 if no server is currently running.
func ReadPort(statusDir string) int {
	if statusDir == "" {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(statusDir, ".hookserver-port"))
	if err != nil {
		return 0
	}
	port, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	return port
}
