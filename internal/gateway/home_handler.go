package gateway

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed static/home.html
var homeHTML []byte

func (s *server) handleRootHome(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		http.Redirect(w, r, "/admin/", http.StatusFound)
		return
	case "/admin":
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		http.Redirect(w, r, "/admin/", http.StatusFound)
		return
	case "/home":
		// continue to render legacy landing page
	default:
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	body := strings.ReplaceAll(string(homeHTML), "{{DEFAULT_ADMIN_TOKEN}}", DefaultAdminToken)
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.Header().Set("cache-control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}
