package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/subagent"
)

func (s *server) handleCCSubagents(w http.ResponseWriter, r *http.Request) {
	if s.subagentStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "subagent store is not configured")
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
	parentID := strings.TrimSpace(r.URL.Query().Get("parent_id"))
	teamID := strings.TrimSpace(r.URL.Query().Get("team_id"))
	if parentID == "" && teamID != "" {
		parentID = teamID
	}
	status := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("status")))
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	includeDeleted := parseBoolQuery(r.URL.Query().Get("include_deleted"))

	items := s.subagentStore.List(parentID)
	filtered := make([]subagent.Agent, 0, len(items))
	for _, item := range items {
		if status == "" && !includeDeleted && strings.EqualFold(strings.TrimSpace(item.Status), "deleted") {
			continue
		}
		if status != "" && strings.ToLower(strings.TrimSpace(item.Status)) != status {
			continue
		}
		if model != "" && strings.TrimSpace(item.Model) != model {
			continue
		}
		filtered = append(filtered, item)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].CreatedAt.After(filtered[j].CreatedAt)
	})
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  filtered,
		"count": len(filtered),
	})
}

func (s *server) handleCCSubagentByPath(w http.ResponseWriter, r *http.Request) {
	if s.subagentStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "subagent store is not configured")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/v1/cc/subagents/")
	id = strings.Trim(strings.TrimSpace(id), "/")
	if id == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "subagent endpoint not found")
		return
	}
	parts := strings.Split(id, "/")
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			s.handleCCSubagentGet(w, parts[0])
			return
		case http.MethodDelete:
			s.handleCCSubagentDelete(w, r, parts[0])
			return
		default:
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
	}
	if len(parts) == 2 && parts[1] == "terminate" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCSubagentTerminate(w, r, parts[0])
		return
	}
	if len(parts) == 2 && (parts[1] == "timeline" || parts[1] == "events") {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCSubagentTimeline(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "stream" {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCSubagentStream(w, r, parts[0])
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "subagent endpoint not found")
}

func (s *server) handleCCSubagentGet(w http.ResponseWriter, id string) {
	out, ok := s.subagentStore.Get(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "subagent not found")
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) handleCCSubagentTerminate(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	if r != nil && r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
	}
	out, err := s.subagentStore.TerminateWithMeta(id, req.By, req.Reason)
	if err != nil {
		writeSubagentStoreError(w, err)
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType:  "subagent.terminated",
		SubagentID: out.ID,
		Data: map[string]any{
			"subagent_id": out.ID,
			"parent_id":   out.ParentID,
			"status":      out.Status,
			"by":          out.TerminatedBy,
			"reason":      out.TerminationReason,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) handleCCSubagentTimeline(w http.ResponseWriter, r *http.Request, id string) {
	if s.eventStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "event store is not configured")
		return
	}

	id = strings.TrimSpace(id)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "subagent id is required")
		return
	}
	if _, ok := s.subagentStore.Get(id); !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "subagent not found")
		return
	}

	limit, ok := parseNonNegativeInt(r.URL.Query().Get("limit"))
	if !ok {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "limit must be an integer >= 0")
		return
	}
	filter := ccevent.ListFilter{
		Limit:      limit,
		EventType:  strings.TrimSpace(r.URL.Query().Get("event_type")),
		SubagentID: id,
	}
	items := s.eventStore.List(filter)

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"subagent_id": id,
		"data":        items,
		"count":       len(items),
	})
}

func (s *server) handleCCSubagentStream(w http.ResponseWriter, r *http.Request, id string) {
	if s.eventStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "event store is not configured")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}

	id = strings.TrimSpace(id)
	if id == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "subagent id is required")
		return
	}
	if _, ok := s.subagentStore.Get(id); !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "subagent not found")
		return
	}

	filter := ccevent.ListFilter{
		EventType:  strings.TrimSpace(r.URL.Query().Get("event_type")),
		SubagentID: id,
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

func (s *server) handleCCSubagentDelete(w http.ResponseWriter, r *http.Request, id string) {
	var req struct {
		By     string `json:"by"`
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
	}
	out, err := s.subagentStore.Delete(id, req.By, req.Reason)
	if err != nil {
		writeSubagentStoreError(w, err)
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType:  "subagent.deleted",
		SubagentID: out.ID,
		Data: map[string]any{
			"subagent_id": out.ID,
			"parent_id":   out.ParentID,
			"status":      out.Status,
			"by":          out.DeletedBy,
			"reason":      out.DeletionReason,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func writeSubagentStoreError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(strings.ToLower(msg), "not found"):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", msg)
	default:
		writeErrorEnvelope(w, http.StatusBadRequest, "invalid_request_error", msg)
	}
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
