package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/requestctx"
)

func (s *server) handleCCMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.mcpRegistry == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "mcp registry is not configured")
		return
	}
	scopeSel := resolveScopeSelection(r)
	switch r.Method {
	case http.MethodPost:
		var req mcpregistry.RegisterInput
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if req.Metadata == nil {
			req.Metadata = map[string]any{}
		}
		req.Metadata["project_id"] = scopeSel.ProjectID
		req.ID = mcpStorageID(scopeSel.ProjectID, req.ID)
		if req.ID == "" && scopeSel.ProjectID != requestctx.DefaultProjectID {
			req.ID = mcpStorageID(scopeSel.ProjectID, s.nextID("mcp"))
		}
		out, err := s.mcpRegistry.Register(req)
		if err != nil {
			writeMCPRegistryError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(mcpServerForProject(scopeSel.ProjectID, out))
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
		all := s.mcpRegistry.List(0)
		filtered := make([]mcpregistry.Server, 0, len(all))
		for _, item := range all {
			if !mcpServerBelongsToProject(scopeSel.ProjectID, item) {
				continue
			}
			filtered = append(filtered, mcpServerForProject(scopeSel.ProjectID, item))
		}
		items := filtered
		if limit > 0 && limit < len(items) {
			items = items[:limit]
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"scope":      scopeSel.Scope,
			"project_id": scopeSel.ProjectID,
			"data":       items,
			"count":      len(items),
			"total":      len(filtered),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCMCPServerByPath(w http.ResponseWriter, r *http.Request) {
	if s.mcpRegistry == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "mcp registry is not configured")
		return
	}
	scopeSel := resolveScopeSelection(r)

	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/mcp/servers/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		s.handleCCMCPServerResource(w, r, parts[0], scopeSel)
		return
	}
	if len(parts) == 2 && parts[1] == "health" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerHealth(w, r, parts[0], scopeSel)
		return
	}
	if len(parts) == 2 && parts[1] == "reconnect" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerReconnect(w, r, parts[0], scopeSel)
		return
	}
	if len(parts) == 3 && parts[1] == "tools" && parts[2] == "list" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerToolsList(w, r, parts[0], scopeSel)
		return
	}
	if len(parts) == 3 && parts[1] == "tools" && parts[2] == "call" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerToolsCall(w, r, parts[0], scopeSel)
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server endpoint not found")
}

func (s *server) handleCCMCPServerResource(w http.ResponseWriter, r *http.Request, serverID string, scopeSel scopeSelection) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	storageID, ok := s.resolveScopedMCPServerID(scopeSel.ProjectID, serverID)
	if !ok && r.Method != http.MethodPut {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server not found")
		return
	}
	if !ok && r.Method == http.MethodPut {
		storageID = mcpStorageID(scopeSel.ProjectID, serverID)
	}
	switch r.Method {
	case http.MethodGet:
		out, _ := s.mcpRegistry.Get(storageID)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mcpServerForProject(scopeSel.ProjectID, out))
	case http.MethodPut:
		var req mcpregistry.UpdateInput
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if req.Metadata == nil {
			meta := map[string]any{
				"project_id": scopeSel.ProjectID,
			}
			req.Metadata = &meta
		} else {
			meta := *req.Metadata
			if meta == nil {
				meta = map[string]any{}
			}
			meta["project_id"] = scopeSel.ProjectID
			req.Metadata = &meta
		}
		out, err := s.mcpRegistry.Update(storageID, req)
		if err != nil {
			writeMCPRegistryError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(mcpServerForProject(scopeSel.ProjectID, out))
	case http.MethodDelete:
		if err := s.mcpRegistry.Delete(storageID); err != nil {
			writeMCPRegistryError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCMCPServerHealth(w http.ResponseWriter, r *http.Request, serverID string, scopeSel scopeSelection) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	storageID, ok := s.resolveScopedMCPServerID(scopeSel.ProjectID, serverID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server not found")
		return
	}
	out, err := s.mcpRegistry.CheckHealth(r.Context(), storageID)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mcpServerForProject(scopeSel.ProjectID, out))
}

func (s *server) handleCCMCPServerReconnect(w http.ResponseWriter, r *http.Request, serverID string, scopeSel scopeSelection) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	storageID, ok := s.resolveScopedMCPServerID(scopeSel.ProjectID, serverID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server not found")
		return
	}
	out, err := s.mcpRegistry.Reconnect(r.Context(), storageID)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(mcpServerForProject(scopeSel.ProjectID, out))
}

func (s *server) handleCCMCPServerToolsList(w http.ResponseWriter, r *http.Request, serverID string, scopeSel scopeSelection) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	storageID, ok := s.resolveScopedMCPServerID(scopeSel.ProjectID, serverID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server not found")
		return
	}
	tools, err := s.mcpRegistry.ListTools(r.Context(), storageID)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"scope":      scopeSel.Scope,
		"project_id": scopeSel.ProjectID,
		"server_id":  serverID,
		"tools":      tools,
		"count":      len(tools),
	})
}

func (s *server) handleCCMCPServerToolsCall(w http.ResponseWriter, r *http.Request, serverID string, scopeSel scopeSelection) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	storageID, ok := s.resolveScopedMCPServerID(scopeSel.ProjectID, serverID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server not found")
		return
	}
	var req struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := decodeJSONBodyStrict(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	result, err := s.mcpRegistry.CallTool(r.Context(), storageID, req.Name, req.Arguments)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
}

func (s *server) resolveScopedMCPServerID(projectID, requestedID string) (string, bool) {
	projectID = strings.TrimSpace(projectID)
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		return "", false
	}
	candidates := []string{mcpStorageID(projectID, requestedID)}
	if candidates[0] != requestedID {
		candidates = append(candidates, requestedID)
	}
	for _, candidate := range candidates {
		server, ok := s.mcpRegistry.Get(candidate)
		if !ok {
			continue
		}
		if !mcpServerBelongsToProject(projectID, server) {
			continue
		}
		return server.ID, true
	}
	return "", false
}

func writeMCPRegistryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, mcpregistry.ErrNotFound):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", strings.TrimSpace(err.Error()))
	case errors.Is(err, mcpregistry.ErrAlreadyExists):
		writeErrorEnvelope(w, http.StatusConflict, "invalid_request_error", strings.TrimSpace(err.Error()))
	case errors.Is(err, mcpregistry.ErrToolNotFound):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", strings.TrimSpace(err.Error()))
	default:
		writeErrorEnvelope(w, http.StatusBadRequest, "invalid_request_error", strings.TrimSpace(err.Error()))
	}
}
