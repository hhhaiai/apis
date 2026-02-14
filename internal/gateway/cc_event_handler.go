package gateway

import (
	"encoding/json"
	"net/http"

	"ccgateway/internal/ccevent"
)

func (s *server) handleCCEvents(w http.ResponseWriter, r *http.Request) {
	if s.eventStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "event store is not configured")
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	limit, ok := parseNonNegativeInt(r.URL.Query().Get("limit"))
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "limit must be an integer >= 0")
		return
	}
	filter := ccevent.ListFilter{
		Limit:      limit,
		EventType:  r.URL.Query().Get("event_type"),
		SessionID:  r.URL.Query().Get("session_id"),
		RunID:      r.URL.Query().Get("run_id"),
		PlanID:     r.URL.Query().Get("plan_id"),
		TodoID:     r.URL.Query().Get("todo_id"),
		TeamID:     r.URL.Query().Get("team_id"),
		SubagentID: r.URL.Query().Get("subagent_id"),
	}
	items := s.eventStore.List(filter)
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  items,
		"count": len(items),
	})
}
