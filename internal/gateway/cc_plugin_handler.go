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

	scopeSel := resolveScopeSelection(r)

	switch r.Method {
	case http.MethodPost:
		var req plugin.Plugin
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		displayName := strings.TrimSpace(req.Name)
		req.Name = pluginStorageName(scopeSel.ProjectID, req.Name)
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
				"project_id":  scopeSel.ProjectID,
				"scope":       scopeSel.Scope,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(projectPluginView(scopeSel.ProjectID, out, displayName))
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
			if !pluginBelongsToProject(scopeSel.ProjectID, item.Name) {
				continue
			}
			if enabledOnly && !item.Enabled {
				continue
			}
			filtered = append(filtered, projectPluginView(scopeSel.ProjectID, item, ""))
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
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
			"data":       filtered,
			"count":      len(filtered),
			"total":      total,
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

	scopeSel := resolveScopeSelection(r)

	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/plugins/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "plugin endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		s.handleCCPluginResource(w, r, parts[0], scopeSel)
		return
	}
	if len(parts) == 2 && (parts[1] == "enable" || parts[1] == "disable") {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCPluginToggle(w, parts[0], parts[1] == "enable", scopeSel)
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "plugin endpoint not found")
}

func (s *server) handleCCPluginResource(w http.ResponseWriter, r *http.Request, name string, scopeSel scopeSelection) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}
	storageName := pluginStorageName(scopeSel.ProjectID, name)

	switch r.Method {
	case http.MethodGet:
		out, ok := s.pluginStore.Get(storageName)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "plugin not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(projectPluginView(scopeSel.ProjectID, out, name))
	case http.MethodDelete:
		if err := s.pluginStore.Uninstall(storageName); err != nil {
			writePluginStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "plugin.uninstalled",
			Data: map[string]any{
				"name":       name,
				"project_id": scopeSel.ProjectID,
				"scope":      scopeSel.Scope,
			},
		})
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCPluginToggle(w http.ResponseWriter, name string, enabled bool, scopeSel scopeSelection) {
	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}
	storageName := pluginStorageName(scopeSel.ProjectID, name)
	var err error
	eventType := "plugin.disabled"
	if enabled {
		err = s.pluginStore.Enable(storageName)
		eventType = "plugin.enabled"
	} else {
		err = s.pluginStore.Disable(storageName)
	}
	if err != nil {
		writePluginStoreError(w, err)
		return
	}
	out, ok := s.pluginStore.Get(storageName)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "plugin state updated but cannot be loaded")
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: eventType,
		Data: map[string]any{
			"name":       name,
			"enabled":    out.Enabled,
			"project_id": scopeSel.ProjectID,
			"scope":      scopeSel.Scope,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(projectPluginView(scopeSel.ProjectID, out, name))
}

func projectPluginView(projectID string, in plugin.Plugin, displayFallback string) plugin.Plugin {
	out := in
	out.Name = pluginDisplayName(projectID, in.Name)
	if strings.TrimSpace(out.Name) == "" {
		out.Name = strings.TrimSpace(displayFallback)
	}
	return out
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
