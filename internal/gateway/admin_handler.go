package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"ccgateway/internal/probe"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/settings"
	"ccgateway/internal/toolcatalog"
	"ccgateway/internal/upstream"
)

func (s *server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.settings == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "settings store is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(s.settings.Get())
	case http.MethodPut:
		var req settings.RuntimeSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		s.settings.Put(req)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(s.settings.Get())
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminModelMapping(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.settings == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "settings store is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := s.settings.Get()
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model_mappings":     cfg.ModelMappings,
			"model_map_strict":   cfg.ModelMapStrict,
			"model_map_fallback": cfg.ModelMapFallback,
		})
	case http.MethodPut:
		var req struct {
			ModelMappings    map[string]string `json:"model_mappings"`
			ModelMapStrict   bool              `json:"model_map_strict"`
			ModelMapFallback string            `json:"model_map_fallback"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		cfg := s.settings.Get()
		cfg.ModelMappings = req.ModelMappings
		cfg.ModelMapStrict = req.ModelMapStrict
		cfg.ModelMapFallback = strings.TrimSpace(req.ModelMapFallback)
		s.settings.Put(cfg)

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model_mappings":     cfg.ModelMappings,
			"model_map_strict":   cfg.ModelMapStrict,
			"model_map_fallback": cfg.ModelMapFallback,
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminUpstream(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	upstreamAdmin, ok := s.orchestrator.(interface {
		GetUpstreamConfig() upstream.UpstreamAdminConfig
		UpdateUpstreamConfig(cfg upstream.UpstreamAdminConfig) (upstream.UpstreamAdminConfig, error)
	})
	if !ok {
		s.writeError(w, http.StatusNotImplemented, "api_error", "orchestrator does not support upstream admin config")
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(upstreamAdmin.GetUpstreamConfig())
	case http.MethodPut:
		var cfg upstream.UpstreamAdminConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		updated, err := upstreamAdmin.UpdateUpstreamConfig(cfg)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(updated)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminTools(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.toolCatalog == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "tool catalog is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tools": s.toolCatalog.Snapshot(),
		})
	case http.MethodPut:
		var req struct {
			Tools []toolcatalog.ToolSpec `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		s.toolCatalog.Replace(req.Tools)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tools": s.toolCatalog.Snapshot(),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminScheduler(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.schedulerStatus == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "scheduler status is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scheduler": schedulerSnapshot(s.schedulerStatus),
		})
	case http.MethodPut:
		updater, ok := s.schedulerStatus.(interface {
			UpdateConfigPatch(patch scheduler.ConfigPatch) (scheduler.Config, error)
			AdminSnapshot() map[string]any
		})
		if !ok {
			s.writeError(w, http.StatusNotImplemented, "api_error", "scheduler update is not supported")
			return
		}
		var patch scheduler.ConfigPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if _, err := updater.UpdateConfigPatch(patch); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scheduler": updater.AdminSnapshot(),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminProbe(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.probeStatus == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "probe status is not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"probe": s.probeStatus.Snapshot(),
		})
	case http.MethodPut:
		updater, ok := s.probeStatus.(interface {
			UpdateConfigPatch(patch probe.ConfigPatch) (probe.Config, error)
			Snapshot() map[string]any
		})
		if !ok {
			s.writeError(w, http.StatusNotImplemented, "api_error", "probe update is not supported")
			return
		}
		var patch probe.ConfigPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if _, err := updater.UpdateConfigPatch(patch); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"probe": updater.Snapshot(),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleAdminAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	token := strings.TrimSpace(s.adminToken)
	authRequired := token != ""
	defaultTokenEnabled := token == DefaultAdminToken

	resp := map[string]any{
		"auth_required":         authRequired,
		"default_token_enabled": defaultTokenEnabled,
	}
	if defaultTokenEnabled {
		resp["default_token_warning"] = "default admin password is enabled; set ADMIN_TOKEN to a custom value"
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) authorizeAdmin(w http.ResponseWriter, r *http.Request) bool {
	if s.adminToken == "" {
		return true
	}

	token := strings.TrimSpace(r.Header.Get("x-admin-token"))
	if token == "" {
		auth := strings.TrimSpace(r.Header.Get("authorization"))
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			token = strings.TrimSpace(auth[7:])
		}
	}
	if token != s.adminToken {
		s.writeError(w, http.StatusUnauthorized, "authentication_error", "admin token is invalid")
		return false
	}
	return true
}

func schedulerSnapshot(status gatewayStatusProvider) map[string]any {
	if status == nil {
		return nil
	}
	if ext, ok := status.(interface{ AdminSnapshot() map[string]any }); ok {
		return ext.AdminSnapshot()
	}
	return status.Snapshot()
}

type gatewayStatusProvider interface {
	Snapshot() map[string]any
}
