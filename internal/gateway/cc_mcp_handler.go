package gateway

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"ccgateway/internal/mcpregistry"
)

func (s *server) handleCCMCPServers(w http.ResponseWriter, r *http.Request) {
	if s.mcpRegistry == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "mcp registry is not configured")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req mcpregistry.RegisterInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.mcpRegistry.Register(req)
		if err != nil {
			writeMCPRegistryError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
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
		items := all
		if limit > 0 && limit < len(items) {
			items = items[:limit]
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
			"total": len(all),
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

	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/mcp/servers/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		s.handleCCMCPServerResource(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "health" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerHealth(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "reconnect" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerReconnect(w, r, parts[0])
		return
	}
	if len(parts) == 3 && parts[1] == "tools" && parts[2] == "list" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerToolsList(w, r, parts[0])
		return
	}
	if len(parts) == 3 && parts[1] == "tools" && parts[2] == "call" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCMCPServerToolsCall(w, r, parts[0])
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server endpoint not found")
}

func (s *server) handleCCMCPServerResource(w http.ResponseWriter, r *http.Request, serverID string) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		out, ok := s.mcpRegistry.Get(serverID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "mcp server not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodPut:
		var req mcpregistry.UpdateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.mcpRegistry.Update(serverID, req)
		if err != nil {
			writeMCPRegistryError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodDelete:
		if err := s.mcpRegistry.Delete(serverID); err != nil {
			writeMCPRegistryError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCMCPServerHealth(w http.ResponseWriter, r *http.Request, serverID string) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	out, err := s.mcpRegistry.CheckHealth(r.Context(), serverID)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) handleCCMCPServerReconnect(w http.ResponseWriter, r *http.Request, serverID string) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	out, err := s.mcpRegistry.Reconnect(r.Context(), serverID)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) handleCCMCPServerToolsList(w http.ResponseWriter, r *http.Request, serverID string) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	tools, err := s.mcpRegistry.ListTools(r.Context(), serverID)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"server_id": serverID,
		"tools":     tools,
		"count":     len(tools),
	})
}

func (s *server) handleCCMCPServerToolsCall(w http.ResponseWriter, r *http.Request, serverID string) {
	serverID = strings.TrimSpace(serverID)
	if serverID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "server id is required")
		return
	}
	var req struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	result, err := s.mcpRegistry.CallTool(r.Context(), serverID, req.Name, req.Arguments)
	if err != nil {
		writeMCPRegistryError(w, err)
		return
	}
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(result)
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
