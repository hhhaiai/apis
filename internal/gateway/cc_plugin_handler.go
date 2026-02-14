package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/plugin"
)

func (s *server) handleCCPlugins(w http.ResponseWriter, r *http.Request) {
	if s.pluginStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "plugin store is not configured")
		return
	}

	switch r.Method {
	case http.MethodPost:
		var req plugin.Plugin
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if err := s.pluginStore.Install(req); err != nil {
			writePluginStoreError(w, err)
			return
		}
		out, ok := s.pluginStore.Get(req.Name)
		if !ok {
			s.writeError(w, http.StatusInternalServerError, "api_error", "plugin install succeeded but cannot be loaded")
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "plugin.installed",
			Data: map[string]any{
				"name":        out.Name,
				"version":     out.Version,
				"enabled":     out.Enabled,
				"skill_count": len(out.Skills),
				"hook_count":  len(out.Hooks),
				"mcp_count":   len(out.MCPServers),
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
		enabledOnly := parseBoolQuery(r.URL.Query().Get("enabled_only"))
		items := s.pluginStore.List()
		filtered := make([]plugin.Plugin, 0, len(items))
		for _, item := range items {
			if enabledOnly && !item.Enabled {
				continue
			}
			filtered = append(filtered, item)
		}
		sort.SliceStable(filtered, func(i, j int) bool {
			return filtered[i].InstalledAt.After(filtered[j].InstalledAt)
		})
		total := len(filtered)
		if limit > 0 && limit < len(filtered) {
			filtered = filtered[:limit]
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  filtered,
			"count": len(filtered),
			"total": total,
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCPluginByPath(w http.ResponseWriter, r *http.Request) {
	if s.pluginStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "plugin store is not configured")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/plugins/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "plugin endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		s.handleCCPluginResource(w, r, parts[0])
		return
	}
	if len(parts) == 2 && (parts[1] == "enable" || parts[1] == "disable") {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCPluginToggle(w, parts[0], parts[1] == "enable")
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "plugin endpoint not found")
}

func (s *server) handleCCPluginResource(w http.ResponseWriter, r *http.Request, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		out, ok := s.pluginStore.Get(name)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "plugin not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodDelete:
		if err := s.pluginStore.Uninstall(name); err != nil {
			writePluginStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "plugin.uninstalled",
			Data: map[string]any{
				"name": name,
			},
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCPluginToggle(w http.ResponseWriter, name string, enabled bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}
	var err error
	eventType := "plugin.disabled"
	if enabled {
		err = s.pluginStore.Enable(name)
		eventType = "plugin.enabled"
	} else {
		err = s.pluginStore.Disable(name)
	}
	if err != nil {
		writePluginStoreError(w, err)
		return
	}
	out, ok := s.pluginStore.Get(name)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "plugin state updated but cannot be loaded")
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: eventType,
		Data: map[string]any{
			"name":    out.Name,
			"enabled": out.Enabled,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func writePluginStoreError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", msg)
	case strings.Contains(msg, "already"):
		writeErrorEnvelope(w, http.StatusConflict, "invalid_request_error", msg)
	default:
		writeErrorEnvelope(w, http.StatusBadRequest, "invalid_request_error", msg)
	}
}
