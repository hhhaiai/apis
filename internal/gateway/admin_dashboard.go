package gateway

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed static/dashboard.html
var dashboardHTML []byte

func (s *server) handleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	if serveBuiltAdminUI(w, r) {
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.Header().Set("cache-control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dashboardHTML)
}

func serveBuiltAdminUI(w http.ResponseWriter, r *http.Request) bool {
	distDir := strings.TrimSpace(os.Getenv("ADMIN_UI_DIST_DIR"))
	if distDir == "" {
		distDir = "web/admin/dist"
	}
	baseAbs, err := filepath.Abs(filepath.Clean(distDir))
	if err != nil {
		return false
	}
	baseInfo, err := os.Stat(baseAbs)
	if err != nil || !baseInfo.IsDir() {
		return false
	}
	indexPath := filepath.Join(baseAbs, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return false
	}

	rel := strings.TrimPrefix(r.URL.Path, "/admin/")
	rel = strings.TrimPrefix(rel, "/")
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	if rel == "" {
		http.ServeFile(w, r, indexPath)
		return true
	}

	targetAbs, err := filepath.Abs(filepath.Join(baseAbs, filepath.FromSlash(rel)))
	if err != nil {
		http.NotFound(w, r)
		return true
	}
	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs, baseAbs+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return true
	}
	if info, err := os.Stat(targetAbs); err == nil && !info.IsDir() {
		http.ServeFile(w, r, targetAbs)
		return true
	}

	// SPA history fallback
	http.ServeFile(w, r, indexPath)
	return true
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
