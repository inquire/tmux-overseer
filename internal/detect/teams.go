package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/inquire/tmux-overseer/internal/core"
)

// teamMemberConfig is one entry in ~/.claude/teams/{name}/config.json "members".
type teamMemberConfig struct {
	Name    string `json:"name"`
	AgentID string `json:"agent_id"`
	Role    string `json:"role"` // "lead" or "teammate"
}

// teamConfig is the top-level structure of ~/.claude/teams/{name}/config.json.
type teamConfig struct {
	Name    string             `json:"name"`
	Members []teamMemberConfig `json:"members"`
}

// ReadTeams scans ~/.claude/teams/*/config.json and returns all configured teams.
// Returns nil if the teams directory doesn't exist or no config files are found.
func ReadTeams() []teamConfig {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	teamsDir := filepath.Join(home, ".claude", "teams")
	entries, err := os.ReadDir(teamsDir)
	if err != nil {
		return nil
	}
	var teams []teamConfig
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		cfgPath := filepath.Join(teamsDir, e.Name(), "config.json")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			continue
		}
		var t teamConfig
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		if t.Name == "" {
			t.Name = e.Name()
		}
		teams = append(teams, t)
	}
	return teams
}

// AnnotateTeams cross-references loaded ClaudeWindows with team configs and
// sets TeamName/TeamRole on matching windows. Matching is done by:
//  1. agent_id field in the team member config vs the window's session name.
//  2. Session name containing the team name (fallback heuristic).
func AnnotateTeams(windows []core.ClaudeWindow) {
	teams := ReadTeams()
	if len(teams) == 0 {
		return
	}

	// Build an index: agentID → (teamName, role)  and  memberName → (teamName, role).
	type membership struct{ team, role string }
	byAgentID := make(map[string]membership)
	byName := make(map[string]membership)
	for _, t := range teams {
		for _, m := range t.Members {
			role := m.Role
			if role == "" {
				role = "teammate"
			}
			mem := membership{team: t.Name, role: role}
			if m.AgentID != "" {
				byAgentID[m.AgentID] = mem
			}
			if m.Name != "" {
				byName[strings.ToLower(m.Name)] = mem
			}
		}
	}

	for i, w := range windows {
		if mem, ok := byAgentID[w.ConversationID]; ok {
			windows[i].TeamName = mem.team
			windows[i].TeamRole = mem.role
			continue
		}
		// Fallback: check if session name contains a team member name.
		lower := strings.ToLower(w.SessionName + ":" + w.WindowName)
		for name, mem := range byName {
			if strings.Contains(lower, name) {
				windows[i].TeamName = mem.team
				windows[i].TeamRole = mem.role
				break
			}
		}
	}
}
