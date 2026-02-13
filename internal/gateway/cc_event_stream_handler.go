package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"

	"ccgateway/internal/ccevent"
)

// handleCCEventsStream provides a Server-Sent Events (SSE) endpoint for real-time event streaming.
// GET /v1/cc/events/stream?session_id=xxx&run_id=xxx&event_type=xxx
func (s *server) handleCCEventsStream(w http.ResponseWriter, r *http.Request) {
	if s.eventStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "event store is not configured")
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}

	filter := ccevent.ListFilter{
		EventType: r.URL.Query().Get("event_type"),
		SessionID: r.URL.Query().Get("session_id"),
		RunID:     r.URL.Query().Get("run_id"),
		PlanID:    r.URL.Query().Get("plan_id"),
		TodoID:    r.URL.Query().Get("todo_id"),
	}

	ch, cancel := s.eventStore.Subscribe(filter)
	defer cancel()

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.EventType, data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
