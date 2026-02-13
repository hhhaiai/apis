package gateway

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/session"
)

func (s *server) handleCCSessions(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "session store is not configured")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req session.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.sessionStore.Create(req)
		if err != nil {
			writeSessionStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "session.created",
			SessionID: out.ID,
			Data: map[string]any{
				"title": out.Title,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodGet:
		limit := 0
		rawLimit := strings.TrimSpace(r.URL.Query().Get("limit"))
		if rawLimit != "" {
			n, err := strconv.Atoi(rawLimit)
			if err != nil || n < 0 {
				s.writeError(w, http.StatusBadRequest, "invalid_request_error", "limit must be an integer >= 0")
				return
			}
			limit = n
		}
		all := s.sessionStore.List(0)
		items := all
		if limit > 0 && limit < len(items) {
			items = items[:limit]
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
			"total": len(all),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCSessionByPath(w http.ResponseWriter, r *http.Request) {
	if s.sessionStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "session store is not configured")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/sessions/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "session endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCSessionGet(w, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "fork" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCSessionFork(w, r, parts[0])
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "session endpoint not found")
}

func (s *server) handleCCSessionGet(w http.ResponseWriter, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "session id is required")
		return
	}
	out, ok := s.sessionStore.Get(sessionID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "session not found")
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) handleCCSessionFork(w http.ResponseWriter, r *http.Request, sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "session id is required")
		return
	}
	var req session.CreateInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	out, err := s.sessionStore.Fork(sessionID, req)
	if err != nil {
		writeSessionStoreError(w, err)
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: "session.forked",
		SessionID: out.ID,
		Data: map[string]any{
			"parent_id": out.ParentID,
			"title":     out.Title,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}

func writeSessionStoreError(w http.ResponseWriter, err error) {
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

func writeErrorEnvelope(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Type: "error",
		Error: ErrorResponse{
			Type:    kind,
			Message: message,
		},
	})
}
