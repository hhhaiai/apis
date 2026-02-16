package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/marketplace"
	"ccgateway/internal/plugin"
	"ccgateway/internal/requestctx"
)

// MarketplaceService defines the interface for marketplace operations.
type MarketplaceService interface {
	ListAvailable() ([]marketplace.PluginManifest, error)
	GetManifest(name string) (marketplace.PluginManifest, error)
	Search(query string, tags []string) ([]marketplace.SearchResult, error)
	Install(name string, config map[string]string) error
	Uninstall(name string) error
	Update(name string) error
	CheckUpdates() ([]marketplace.UpdateInfo, error)
	GetRecommendations() ([]marketplace.PluginManifest, error)
	GetStats(pluginName string) (marketplace.PluginStats, bool)
	GetPopularPlugins(limit int) []marketplace.PluginStats
}

func (s *server) handleCCMarketplacePlugins(w http.ResponseWriter, r *http.Request) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	manifests, err := s.marketplaceService.ListAvailable()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  manifests,
		"count": len(manifests),
	})
}

func (s *server) handleCCMarketplacePluginByName(w http.ResponseWriter, r *http.Request, name string) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	manifest, err := s.marketplaceService.GetManifest(name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		}
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(manifest)
}

func (s *server) handleCCMarketplacePluginInstall(w http.ResponseWriter, r *http.Request, name string) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}

	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	// Parse optional configuration
	var req struct {
		Config map[string]string `json:"config"`
	}
	if err := decodeJSONBodyStrict(r, &req, true); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	scopeSel := resolveScopeSelection(r)

	// Project-scoped install uses namespaced plugin store entries.
	if scopeSel.Scope == scopeProject && scopeSel.ProjectID != requestctx.DefaultProjectID && s.pluginStore != nil {
		manifest, err := s.marketplaceService.GetManifest(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
			} else {
				s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
			}
			return
		}
		projectPlugin := plugin.Plugin{
			Name:        pluginStorageName(scopeSel.ProjectID, manifest.Name),
			Version:     manifest.Version,
			Description: manifest.Description,
			Skills:      manifest.Skills,
			Hooks:       manifest.Hooks,
			MCPServers:  manifest.MCPServers,
			Enabled:     true,
		}
		if err := s.pluginStore.Install(projectPlugin); err != nil {
			writePluginStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "plugin.installed",
			Data: map[string]any{
				"name":       name,
				"source":     "marketplace",
				"project_id": scopeSel.ProjectID,
				"scope":      scopeSel.Scope,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"message":    "plugin installed successfully",
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
		})
		return
	}

	// Install plugin
	if err := s.marketplaceService.Install(name, req.Config); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
		} else if strings.Contains(err.Error(), "already installed") {
			s.writeError(w, http.StatusConflict, "conflict", err.Error())
		} else if strings.Contains(err.Error(), "dependencies") {
			s.writeError(w, http.StatusBadRequest, "dependency_error", err.Error())
		} else if strings.Contains(err.Error(), "validation") || strings.Contains(err.Error(), "configuration") {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		}
		return
	}

	// Emit event
	s.appendEvent(ccevent.AppendInput{
		EventType: "plugin.installed",
		Data: map[string]any{
			"name":       name,
			"source":     "marketplace",
			"project_id": scopeSel.ProjectID,
			"scope":      scopeSel.Scope,
		},
	})

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"message":    "plugin installed successfully",
		"scope":      scopeSel.Scope,
		"project_id": scopeSel.ProjectID,
	})
}

func (s *server) handleCCMarketplaceSearch(w http.ResponseWriter, r *http.Request) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	query := r.URL.Query().Get("q")
	tagsParam := r.URL.Query().Get("tags")

	var tags []string
	if tagsParam != "" {
		tags = strings.Split(tagsParam, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	results, err := s.marketplaceService.Search(query, tags)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  results,
		"count": len(results),
	})
}

func (s *server) handleCCMarketplaceUpdates(w http.ResponseWriter, r *http.Request) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	updates, err := s.marketplaceService.CheckUpdates()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  updates,
		"count": len(updates),
	})
}

func (s *server) handleCCMarketplaceRecommendations(w http.ResponseWriter, r *http.Request) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	recommendations, err := s.marketplaceService.GetRecommendations()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  recommendations,
		"count": len(recommendations),
	})
}

func (s *server) handleCCMarketplaceStats(w http.ResponseWriter, r *http.Request, pluginName string) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	pluginName = strings.TrimSpace(pluginName)
	if pluginName == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	stats, ok := s.marketplaceService.GetStats(pluginName)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "no statistics found for plugin")
		return
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}

func (s *server) handleCCMarketplacePopular(w http.ResponseWriter, r *http.Request) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	// Default limit is 10
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		var parsedLimit int
		if _, err := fmt.Sscanf(limitStr, "%d", &parsedLimit); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	popular := s.marketplaceService.GetPopularPlugins(limit)

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"data":  popular,
		"count": len(popular),
	})
}

func (s *server) handleCCMarketplacePluginUninstall(w http.ResponseWriter, r *http.Request, name string) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}

	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	scopeSel := resolveScopeSelection(r)
	if scopeSel.Scope == scopeProject && scopeSel.ProjectID != requestctx.DefaultProjectID && s.pluginStore != nil {
		if err := s.pluginStore.Uninstall(pluginStorageName(scopeSel.ProjectID, name)); err != nil {
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
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"message":    "plugin uninstalled successfully",
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
		})
		return
	}

	if err := s.marketplaceService.Uninstall(name); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not installed") {
			s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		}
		return
	}

	// Emit event
	s.appendEvent(ccevent.AppendInput{
		EventType: "plugin.uninstalled",
		Data: map[string]any{
			"name":       name,
			"project_id": scopeSel.ProjectID,
			"scope":      scopeSel.Scope,
		},
	})

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"message":    "plugin uninstalled successfully",
		"scope":      scopeSel.Scope,
		"project_id": scopeSel.ProjectID,
	})
}

func (s *server) handleCCMarketplacePluginUpdate(w http.ResponseWriter, r *http.Request, name string) {
	if s.marketplaceService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "marketplace service is not configured")
		return
	}

	name = strings.TrimSpace(name)
	if name == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "plugin name is required")
		return
	}

	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	scopeSel := resolveScopeSelection(r)
	if scopeSel.Scope == scopeProject && scopeSel.ProjectID != requestctx.DefaultProjectID && s.pluginStore != nil {
		manifest, err := s.marketplaceService.GetManifest(name)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
			} else {
				s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
			}
			return
		}
		storageName := pluginStorageName(scopeSel.ProjectID, name)
		current, ok := s.pluginStore.Get(storageName)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "plugin is not installed in selected project scope")
			return
		}
		if current.Version == manifest.Version {
			s.writeError(w, http.StatusConflict, "conflict", "plugin is already at the latest version")
			return
		}
		if err := s.pluginStore.Uninstall(storageName); err != nil {
			writePluginStoreError(w, err)
			return
		}
		updated := plugin.Plugin{
			Name:        storageName,
			Version:     manifest.Version,
			Description: manifest.Description,
			Skills:      manifest.Skills,
			Hooks:       manifest.Hooks,
			MCPServers:  manifest.MCPServers,
			Enabled:     current.Enabled,
		}
		if err := s.pluginStore.Install(updated); err != nil {
			_ = s.pluginStore.Install(current)
			writePluginStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "plugin.updated",
			Data: map[string]any{
				"name":       name,
				"project_id": scopeSel.ProjectID,
				"scope":      scopeSel.Scope,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":    true,
			"message":    "plugin updated successfully",
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
		})
		return
	}

	if err := s.marketplaceService.Update(name); err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not installed") {
			s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
		} else if strings.Contains(err.Error(), "already at the latest version") {
			s.writeError(w, http.StatusConflict, "conflict", err.Error())
		} else {
			s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
		}
		return
	}

	// Emit event
	s.appendEvent(ccevent.AppendInput{
		EventType: "plugin.updated",
		Data: map[string]any{
			"name":       name,
			"project_id": scopeSel.ProjectID,
			"scope":      scopeSel.Scope,
		},
	})

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"message":    "plugin updated successfully",
		"scope":      scopeSel.Scope,
		"project_id": scopeSel.ProjectID,
	})
}

func (s *server) handleCCMarketplaceByPath(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/marketplace/")
	path = strings.Trim(path, "/")

	if path == "" || path == "plugins" {
		s.handleCCMarketplacePlugins(w, r)
		return
	}

	if path == "search" {
		s.handleCCMarketplaceSearch(w, r)
		return
	}

	if path == "updates" {
		s.handleCCMarketplaceUpdates(w, r)
		return
	}

	if path == "recommendations" {
		s.handleCCMarketplaceRecommendations(w, r)
		return
	}

	if path == "popular" {
		s.handleCCMarketplacePopular(w, r)
		return
	}

	// Handle /stats/{name}
	if strings.HasPrefix(path, "stats/") {
		pluginName := strings.TrimPrefix(path, "stats/")
		s.handleCCMarketplaceStats(w, r, pluginName)
		return
	}

	// Handle /plugins/{name} and /plugins/{name}/install and /plugins/{name}/uninstall
	if strings.HasPrefix(path, "plugins/") {
		pluginPath := strings.TrimPrefix(path, "plugins/")
		parts := strings.Split(pluginPath, "/")

		if len(parts) == 1 {
			s.handleCCMarketplacePluginByName(w, r, parts[0])
			return
		}

		if len(parts) == 2 && parts[1] == "install" {
			s.handleCCMarketplacePluginInstall(w, r, parts[0])
			return
		}

		if len(parts) == 2 && parts[1] == "uninstall" {
			s.handleCCMarketplacePluginUninstall(w, r, parts[0])
			return
		}

		if len(parts) == 2 && parts[1] == "update" {
			s.handleCCMarketplacePluginUpdate(w, r, parts[0])
			return
		}
	}

	s.writeError(w, http.StatusNotFound, "not_found_error", "marketplace endpoint not found")
}
