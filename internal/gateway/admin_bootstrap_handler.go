package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/plugin"
	"ccgateway/internal/requestctx"
	"ccgateway/internal/toolcatalog"
	"ccgateway/internal/upstream"
)

type adminBootstrapRequest struct {
	Scope     string                        `json:"scope,omitempty"`
	ProjectID string                        `json:"project_id,omitempty"`
	Tools     []toolcatalog.ToolSpec        `json:"tools,omitempty"`
	Plugins   []plugin.Plugin               `json:"plugins,omitempty"`
	MCP       []mcpregistry.RegisterInput   `json:"mcp_servers,omitempty"`
	Upstream  *upstream.UpstreamAdminConfig `json:"upstream,omitempty"`
}

func (s *server) handleAdminBootstrapApply(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}

	switch r.Method {
	case http.MethodGet:
		template := map[string]any{
			"scope":      "project",
			"project_id": "demo",
			"tools": []toolcatalog.ToolSpec{
				{Name: "web_search", Status: toolcatalog.StatusSupported},
				{Name: "image_recognition", Status: toolcatalog.StatusExperimental},
			},
			"plugins": []plugin.Plugin{
				{Name: "planner_pack", Version: "1.0.0", Description: "Planning and reflection bundle"},
			},
			"mcp_servers": []mcpregistry.RegisterInput{
				{Name: "local-files", Transport: mcpregistry.TransportStdio, Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}},
			},
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(template)
	case http.MethodPost:
		var req adminBootstrapRequest
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}

		scopeSel := resolveScopeSelection(r)
		if strings.TrimSpace(req.Scope) != "" {
			scopeRaw := strings.ToLower(strings.TrimSpace(req.Scope))
			if scopeRaw == scopeGlobal {
				scopeSel.Scope = scopeGlobal
				scopeSel.ProjectID = requestctx.DefaultProjectID
			} else {
				scopeSel.Scope = scopeProject
			}
		}
		if strings.TrimSpace(req.ProjectID) != "" {
			scopeSel.ProjectID = requestctx.NormalizeProjectID(req.ProjectID)
		}
		if scopeSel.Scope == scopeGlobal {
			scopeSel.ProjectID = requestctx.DefaultProjectID
		}

		type failedItem struct {
			Item  string `json:"item"`
			Error string `json:"error"`
		}
		failures := make([]failedItem, 0)
		result := map[string]any{
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
			"applied": map[string]any{
				"tools":       0,
				"plugins":     0,
				"mcp_servers": 0,
				"upstream":    false,
			},
		}

		if len(req.Tools) > 0 && s.toolCatalog != nil {
			if scoped, ok := s.toolCatalog.(interface {
				ReplaceForProject(projectID string, tools []toolcatalog.ToolSpec)
			}); ok && scopeSel.Scope == scopeProject {
				scoped.ReplaceForProject(scopeSel.ProjectID, req.Tools)
			} else {
				s.toolCatalog.Replace(req.Tools)
			}
			result["applied"].(map[string]any)["tools"] = len(req.Tools)
		}

		if len(req.Plugins) > 0 {
			if s.pluginStore == nil {
				failures = append(failures, failedItem{Item: "plugins", Error: "plugin store is not configured"})
			} else {
				applied := 0
				for _, item := range req.Plugins {
					originName := strings.TrimSpace(item.Name)
					if originName == "" {
						failures = append(failures, failedItem{Item: "plugin", Error: "plugin name is required"})
						continue
					}
					item.Name = pluginStorageName(scopeSel.ProjectID, originName)
					if err := s.pluginStore.Install(item); err != nil {
						failures = append(failures, failedItem{Item: originName, Error: strings.TrimSpace(err.Error())})
						continue
					}
					applied++
				}
				result["applied"].(map[string]any)["plugins"] = applied
			}
		}

		if len(req.MCP) > 0 {
			if s.mcpRegistry == nil {
				failures = append(failures, failedItem{Item: "mcp_servers", Error: "mcp registry is not configured"})
			} else {
				applied := 0
				for _, item := range req.MCP {
					if item.Metadata == nil {
						item.Metadata = map[string]any{}
					}
					item.Metadata["project_id"] = scopeSel.ProjectID
					item.ID = mcpStorageID(scopeSel.ProjectID, item.ID)
					if item.ID == "" && scopeSel.ProjectID != requestctx.DefaultProjectID {
						item.ID = mcpStorageID(scopeSel.ProjectID, s.nextID("mcp"))
					}
					if _, err := s.mcpRegistry.Register(item); err != nil {
						failures = append(failures, failedItem{Item: item.Name, Error: strings.TrimSpace(err.Error())})
						continue
					}
					applied++
				}
				result["applied"].(map[string]any)["mcp_servers"] = applied
			}
		}

		if req.Upstream != nil {
			if scopeSel.Scope != scopeGlobal {
				failures = append(failures, failedItem{Item: "upstream", Error: "upstream config can only be applied in global scope"})
			} else {
				upstreamAdmin, ok := s.orchestrator.(interface {
					UpdateUpstreamConfig(cfg upstream.UpstreamAdminConfig) (upstream.UpstreamAdminConfig, error)
				})
				if !ok {
					failures = append(failures, failedItem{Item: "upstream", Error: "orchestrator does not support upstream admin config"})
				} else if _, err := upstreamAdmin.UpdateUpstreamConfig(*req.Upstream); err != nil {
					failures = append(failures, failedItem{Item: "upstream", Error: strings.TrimSpace(err.Error())})
				} else {
					result["applied"].(map[string]any)["upstream"] = true
				}
			}
		}

		if len(failures) > 0 {
			result["failures"] = failures
		}
		result["ok"] = len(failures) == 0

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}
