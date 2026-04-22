package hook

import (
	"encoding/json"
	"os"
	"strings"
)

// SubagentEntry represents an active subagent in the list file.
type SubagentEntry struct {
	ID            string `json:"id"`
	AgentType     string `json:"agent_type"`
	Description   string `json:"description"`
	Model         string `json:"model"`
	StartedAt     string `json:"started_at"`
	Status        string `json:"status"`
	ParentAgentID string `json:"parent_agent_id,omitempty"`
	SandboxType   string `json:"sandbox_type,omitempty"` // "docker", "kubernetes", or ""
}

// DetectSandbox returns the sandbox type of the current process environment.
// Returns "docker", "kubernetes", or "" (bare/unknown).
func DetectSandbox() string {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		s := string(data)
		if strings.Contains(s, "docker") || strings.Contains(s, "containerd") {
			return "docker"
		}
		if strings.Contains(s, "kubepods") {
			return "kubernetes"
		}
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return "kubernetes"
	}
	return ""
}

func readSubagentList(path string) ([]SubagentEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil
	}
	var list []SubagentEntry
	_ = json.Unmarshal(data, &list)
	return list, nil
}

func subagentAdd(path string, entry SubagentEntry) (int, error) {
	list, _ := readSubagentList(path)
	list = append(list, entry)
	data, _ := json.Marshal(list)
	return len(list), os.WriteFile(path, data, 0o644)
}

func subagentRemove(path string, agentID string) (int, error) {
	list, _ := readSubagentList(path)
	var filtered []SubagentEntry
	for _, e := range list {
		if e.ID != agentID {
			filtered = append(filtered, e)
		}
	}
	if filtered == nil {
		filtered = []SubagentEntry{}
	}
	data, _ := json.Marshal(filtered)
	return len(filtered), os.WriteFile(path, data, 0o644)
}

func subagentCount(path string) int {
	list, _ := readSubagentList(path)
	return len(list)
}
