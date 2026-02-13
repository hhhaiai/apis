package gateway

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

func (s *server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.Header().Set("cache-control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dashboardHTML)
}

func (s *server) handleAdminCost(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.costTracker == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "cost tracker is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		snap := s.costTracker.Snapshot()
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	status := map[string]any{
		"health": true,
	}
	if s.settings != nil {
		status["settings"] = s.settings.Get()
	}
	if s.schedulerStatus != nil {
		status["scheduler"] = schedulerSnapshot(s.schedulerStatus)
	}
	if s.probeStatus != nil {
		status["probe"] = s.probeStatus.Snapshot()
	}
	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}
