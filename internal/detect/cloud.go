package detect

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/inquire/tmux-overseer/internal/core"
)

const (
	cloudAPIBase   = "https://api.cursor.com/v0"
	cloudPollTTL   = 30 * time.Second
	cloudUserAgent = "claude-tmux/1.0"

	cloudHandoffMaxAge = 24 * time.Hour
)

type cloudAgent struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Status        string      `json:"status"`
	Source        cloudSource `json:"source"`
	Target        cloudTarget `json:"target"`
	Summary       string      `json:"summary"`
	CreatedAt     string      `json:"createdAt"`
	TriggerType   string      `json:"triggerType"`   // "manual", "automation", "schedule", etc.
	AutomationID  string      `json:"automationId"`  // set when triggered by a Cursor Automation
}

type cloudSource struct {
	Repository string `json:"repository"`
	Ref        string `json:"ref"`
}

type cloudTarget struct {
	BranchName string `json:"branchName"`
	URL        string `json:"url"`
	PRURL      string `json:"prUrl"`
}

type cloudListResponse struct {
	Agents     []cloudAgent `json:"agents"`
	NextCursor string       `json:"nextCursor"`
}

type cloudHandoff struct {
	ConversationID string `json:"conversation_id"`
	Workspace      string `json:"workspace"`
	WorkspaceName  string `json:"workspace_name"`
	Prompt         string `json:"prompt"`
	Model          string `json:"model"`
	Status         string `json:"status"`
	Timestamp      int64  `json:"timestamp"`
}

var cloudCache struct {
	mu        sync.Mutex
	agents    []core.ClaudeWindow
	fetchedAt time.Time
}

func cloudAPIKey() string {
	if key := os.Getenv("CURSOR_API_KEY"); key != "" {
		return key
	}
	dir := statusDir()
	if dir == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		CursorAPIKey string `json:"cursor_api_key"`
	}
	if json.Unmarshal(data, &cfg) == nil {
		return cfg.CursorAPIKey
	}
	return ""
}

// ReadCloudAgents returns ClaudeWindow entries for Cursor Cloud Agents
// using dual detection: local hook handoffs + API when key is set.
// Results are cached with a 30-second TTL.
func ReadCloudAgents() []core.ClaudeWindow {
	cloudCache.mu.Lock()
	defer cloudCache.mu.Unlock()

	if !cloudCache.fetchedAt.IsZero() && time.Since(cloudCache.fetchedAt) < cloudPollTTL {
		return cloudCache.agents
	}

	local := readLocalHandoffs()

	var api []core.ClaudeWindow
	if apiKey := cloudAPIKey(); apiKey != "" {
		api = fetchCloudAgents(apiKey)
	}

	merged := mergeCloudSources(local, api)

	cloudCache.agents = merged
	cloudCache.fetchedAt = time.Now()
	return merged
}

// InvalidateCloudCache forces the next ReadCloudAgents call to re-fetch.
func InvalidateCloudCache() {
	cloudCache.mu.Lock()
	cloudCache.fetchedAt = time.Time{}
	cloudCache.mu.Unlock()
}

func readLocalHandoffs() []core.ClaudeWindow {
	dir := statusDir()
	if dir == "" {
		return nil
	}

	path := filepath.Join(dir, "cloud-handoffs.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	now := time.Now()
	var windows []core.ClaudeWindow
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var h cloudHandoff
		if err := json.Unmarshal(scanner.Bytes(), &h); err != nil {
			continue
		}
		entryAge := now.Sub(time.Unix(h.Timestamp, 0))
		if entryAge > cloudHandoffMaxAge {
			continue
		}
		win := handoffToWindow(h)
		windows = append(windows, win)
	}
	return windows
}

func handoffToWindow(h cloudHandoff) core.ClaudeWindow {
	status := core.StatusWorking
	if h.Status == "FINISHED" {
		status = core.StatusIdle
	}

	name := h.WorkspaceName
	if name == "" {
		name = "cloud"
	}
	promptPreview := h.Prompt
	if len(promptPreview) > 50 {
		promptPreview = promptPreview[:50]
	}
	if promptPreview != "" {
		name += ": " + promptPreview
	}

	pane := core.ClaudePane{
		PaneID:     h.ConversationID,
		Status:     status,
		WorkingDir: h.Workspace,
		Model:      h.Model,
	}

	win := core.ClaudeWindow{
		SessionName:    name,
		WindowName:     name,
		Panes:          []core.ClaudePane{pane},
		Source:         core.SourceCloud,
		ConversationID: h.ConversationID,
		WorkspacePath:  h.Workspace,
		CreatedAt:      h.Timestamp,
		SessionStartTS: h.Timestamp,
		CloudSummary:   "Handed off from local session",
	}
	win.ComputeSearchText()
	return win
}

func fetchCloudAgents(apiKey string) []core.ClaudeWindow {
	req, err := http.NewRequest("GET", cloudAPIBase+"/agents?limit=20", nil)
	if err != nil {
		return nil
	}
	req.SetBasicAuth(apiKey, "")
	req.Header.Set("User-Agent", cloudUserAgent)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var listResp cloudListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil
	}

	var windows []core.ClaudeWindow
	for _, agent := range listResp.Agents {
		win := cloudAgentToWindow(agent)
		windows = append(windows, win)
	}
	return windows
}

func cloudAgentToWindow(agent cloudAgent) core.ClaudeWindow {
	var status core.Status
	switch agent.Status {
	case "CREATING", "RUNNING":
		status = core.StatusWorking
	case "FINISHED":
		status = core.StatusIdle
	case "STOPPED":
		status = core.StatusIdle
	default:
		status = core.StatusUnknown
	}

	name := agent.Name
	if name == "" {
		name = agent.ID
	}
	if len(name) > 50 {
		name = name[:50]
	}

	// Determine source: automation-triggered agents get SourceAutomation.
	source := core.SourceCloud
	if agent.AutomationID != "" || agent.TriggerType == "automation" || agent.TriggerType == "schedule" {
		source = core.SourceAutomation
	}

	pane := core.ClaudePane{
		PaneID:     agent.ID,
		Status:     status,
		WorkingDir: agent.Source.Repository,
		GitBranch:  agent.Target.BranchName,
		HasGit:     agent.Source.Repository != "",
	}

	var createdTS int64
	if t, err := time.Parse(time.RFC3339, agent.CreatedAt); err == nil {
		createdTS = t.Unix()
	}

	// Include automation trigger context in the summary prefix.
	summary := agent.Summary
	if source == core.SourceAutomation && agent.TriggerType != "" && agent.TriggerType != "manual" {
		if summary != "" {
			summary = "[" + agent.TriggerType + "] " + summary
		} else {
			summary = "triggered by " + agent.TriggerType
		}
	}

	win := core.ClaudeWindow{
		SessionName:       name,
		WindowName:        name,
		Panes:             []core.ClaudePane{pane},
		Source:            source,
		ConversationID:    agent.ID,
		WorkspacePath:     agent.Source.Repository,
		CreatedAt:         createdTS,
		SessionStartTS:    createdTS,
		CloudAgentURL:     agent.Target.URL,
		CloudPRURL:        agent.Target.PRURL,
		CloudSummary:      summary,
		AutomationTrigger: agent.TriggerType,
	}
	win.ComputeSearchText()
	return win
}

// mergeCloudSources combines local handoff entries with API results.
// API entries take precedence for matching conversation IDs.
func mergeCloudSources(local, api []core.ClaudeWindow) []core.ClaudeWindow {
	if len(api) == 0 {
		return local
	}
	if len(local) == 0 {
		return api
	}

	apiByID := make(map[string]struct{}, len(api))
	for _, w := range api {
		apiByID[w.ConversationID] = struct{}{}
	}

	merged := make([]core.ClaudeWindow, 0, len(api)+len(local))
	merged = append(merged, api...)

	for _, w := range local {
		if _, exists := apiByID[w.ConversationID]; !exists {
			merged = append(merged, w)
		}
	}

	return merged
}

// GetCloudSessionCount returns the number of Cloud sessions in the list.
func GetCloudSessionCount(windows []core.ClaudeWindow) int {
	count := 0
	for _, w := range windows {
		if w.Source == core.SourceCloud {
			count++
		}
	}
	return count
}

