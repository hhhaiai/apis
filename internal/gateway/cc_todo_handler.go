package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/todo"
)

func (s *server) handleCCTodos(w http.ResponseWriter, r *http.Request) {
	if s.todoStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "todo store is not configured")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req todo.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.todoStore.Create(req)
		if err != nil {
			writeTodoStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "todo.created",
			SessionID: out.SessionID,
			RunID:     out.RunID,
			PlanID:    out.PlanID,
			TodoID:    out.ID,
			Data: map[string]any{
				"status":  out.Status,
				"title":   out.Title,
				"plan_id": out.PlanID,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodGet:
		limit, ok := parseNonNegativeInt(r.URL.Query().Get("limit"))
		if !ok {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "limit must be an integer >= 0")
			return
		}
		filter := todo.ListFilter{
			Limit:     limit,
			Status:    r.URL.Query().Get("status"),
			SessionID: r.URL.Query().Get("session_id"),
			RunID:     r.URL.Query().Get("run_id"),
			PlanID:    r.URL.Query().Get("plan_id"),
		}
		items := s.todoStore.List(filter)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCTodoByPath(w http.ResponseWriter, r *http.Request) {
	if s.todoStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "todo store is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/todos/")
	path = strings.Trim(path, "/")
	if path == "" || strings.Contains(path, "/") {
		s.writeError(w, http.StatusNotFound, "not_found_error", "todo endpoint not found")
		return
	}
	switch r.Method {
	case http.MethodGet:
		out, ok := s.todoStore.Get(path)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "todo not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodPut:
		var req todo.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.todoStore.Update(path, req)
		if err != nil {
			writeTodoStoreError(w, err)
			return
		}
		eventType := "todo.updated"
		if out.Status == todo.StatusCompleted {
			eventType = "todo.completed"
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: eventType,
			SessionID: out.SessionID,
			RunID:     out.RunID,
			PlanID:    out.PlanID,
			TodoID:    out.ID,
			Data: map[string]any{
				"status":  out.Status,
				"plan_id": out.PlanID,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func parseNonNegativeInt(raw string) (int, bool) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, true
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func writeTodoStoreError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", msg)
	case strings.Contains(msg, "already exists"):
		writeErrorEnvelope(w, http.StatusConflict, "invalid_request_error", msg)
	default:
		writeErrorEnvelope(w, http.StatusBadRequest, "invalid_request_error", msg)
	}
}
