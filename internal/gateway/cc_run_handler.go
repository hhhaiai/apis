package gateway

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *server) handleCCRuns(w http.ResponseWriter, r *http.Request) {
	if s.runStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "run store is not configured")
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
	filter := runListFilterFromRequest(r, limit)
	items := s.runStore.List(filter)
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  items,
		"count": len(items),
	})
}

func (s *server) handleCCRunByPath(w http.ResponseWriter, r *http.Request) {
	if s.runStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "run store is not configured")
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/runs/")
	path = strings.Trim(path, "/")
	if path == "" || strings.Contains(path, "/") {
		s.writeError(w, http.StatusNotFound, "not_found_error", "run endpoint not found")
		return
	}
	out, ok := s.runStore.Get(path)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "run not found")
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}
