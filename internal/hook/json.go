package hook

import "strings"

// jsonEscape escapes a string for embedding in a JSON value (no quotes added).
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// buildEventJSON builds a JSON object string from ts, type, and extra key-value pairs.
func buildEventJSON(ts, eventType string, extra map[string]string) string {
	var b strings.Builder
	b.WriteString(`{"ts":"`)
	b.WriteString(jsonEscape(ts))
	b.WriteString(`","type":"`)
	b.WriteString(jsonEscape(eventType))
	b.WriteByte('"')
	for k, v := range extra {
		b.WriteString(`,"`)
		b.WriteString(k)
		b.WriteString(`":"`)
		b.WriteString(jsonEscape(v))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

// appendAgentTag adds agent_id and agent_type fields to a JSON string.
func appendAgentTag(eventJSON, agentID, agentType string) string {
	if agentID == "" {
		return eventJSON
	}
	return eventJSON[:len(eventJSON)-1] +
		`,"agent_id":"` + jsonEscape(agentID) +
		`","agent_type":"` + jsonEscape(agentType) + `"}`
}
