package gateway

import (
	"fmt"
	"net/http"
	"strings"

	"ccgateway/internal/ccevent"
)

func (s *server) appendEvent(in ccevent.AppendInput) {
	if s.eventStore == nil {
		return
	}
	in = withRecordText(in)
	_, _ = s.eventStore.Append(in)
}

func withRecordText(in ccevent.AppendInput) ccevent.AppendInput {
	data := in.Data
	if data == nil {
		data = map[string]any{}
	}
	if strings.TrimSpace(valueAsString(data["record_text"])) == "" {
		if text := buildRecordText(in, data); text != "" {
			data["record_text"] = text
		}
	}
	in.Data = data
	return in
}

func buildRecordText(in ccevent.AppendInput, data map[string]any) string {
	eventType := strings.TrimSpace(in.EventType)
	if eventType == "" {
		return ""
	}
	parts := []string{eventType}

	appendPair := func(key, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		parts = append(parts, key+"="+val)
	}

	appendPair("session_id", strings.TrimSpace(in.SessionID))
	appendPair("run_id", strings.TrimSpace(in.RunID))
	appendPair("plan_id", strings.TrimSpace(in.PlanID))
	appendPair("todo_id", strings.TrimSpace(in.TodoID))
	appendPair("team_id", strings.TrimSpace(in.TeamID))
	appendPair("subagent_id", strings.TrimSpace(in.SubagentID))

	for _, key := range []string{
		"path",
		"mode",
		"status",
		"name",
		"version",
		"enabled",
		"task_id",
		"title",
		"assigned_to",
		"agent_id",
		"team_name",
		"from",
		"to",
		"content",
		"by",
		"reason",
		"parent_id",
		"count",
		"target",
		"skill_count",
		"hook_count",
		"mcp_count",
		"completed_todo_id",
		"started_todo_id",
		"remaining_pending",
		"remaining_running",
		"total_linked_todos",
	} {
		appendPair(key, valueAsString(data[key]))
	}

	if output := normalizeSpaces(valueAsString(data["output_text"])); output != "" {
		parts = append(parts, fmt.Sprintf(`output="%s"`, truncateText(output, 220)))
	}
	if errText := normalizeSpaces(valueAsString(data["error"])); errText != "" {
		parts = append(parts, fmt.Sprintf(`error="%s"`, truncateText(errText, 160)))
	}

	return strings.Join(parts, " | ")
}

func requestSessionID(r *http.Request, metadata map[string]any) string {
	if r != nil {
		if v := strings.TrimSpace(r.Header.Get("x-cc-session-id")); v != "" {
			return v
		}
	}
	for _, key := range []string{"session_id", "cc_session_id", "sessionId"} {
		if v, ok := metadata[key].(string); ok {
			if text := strings.TrimSpace(v); text != "" {
				return text
			}
		}
	}
	return ""
}
