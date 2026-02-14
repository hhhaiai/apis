package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"ccgateway/internal/agentteam"
	"ccgateway/internal/ccevent"
)

func (s *server) handleCCTeams(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "team store is not configured")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req agentteam.CreateInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.teamStore.Create(req)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "team.created",
			TeamID:    out.ID,
			Data: map[string]any{
				"team_id":     out.ID,
				"name":        out.Name,
				"agent_count": out.AgentCount,
				"task_count":  out.TaskCount,
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
		items := s.teamStore.List(limit)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCTeamByPath(w http.ResponseWriter, r *http.Request) {
	if s.teamStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "team store is not configured")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/teams/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "team endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		s.handleCCTeamResource(w, r, parts[0])
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "agents":
			s.handleCCTeamAgents(w, r, parts[0])
			return
		case "tasks":
			s.handleCCTeamTasks(w, r, parts[0])
			return
		case "messages":
			s.handleCCTeamMessages(w, r, parts[0])
			return
		case "orchestrate":
			if r.Method != http.MethodPost {
				s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
				return
			}
			s.handleCCTeamOrchestrate(w, r, parts[0])
			return
		}
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "team endpoint not found")
}

func (s *server) handleCCTeamResource(w http.ResponseWriter, r *http.Request, teamID string) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "team id is required")
		return
	}
	switch r.Method {
	case http.MethodGet:
		out, ok := s.teamStore.Get(teamID)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "team not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCTeamAgents(w http.ResponseWriter, r *http.Request, teamID string) {
	switch r.Method {
	case http.MethodPost:
		var req agentteam.Agent
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.teamStore.AddAgent(teamID, req)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "team.agent.added",
			TeamID:    teamID,
			Data: map[string]any{
				"team_id":  teamID,
				"agent_id": out.ID,
				"name":     out.Name,
				"role":     out.Role,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodGet:
		items, err := s.teamStore.ListAgents(teamID)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCTeamTasks(w http.ResponseWriter, r *http.Request, teamID string) {
	switch r.Method {
	case http.MethodPost:
		var req agentteam.CreateTaskInput
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.teamStore.AddTask(teamID, req)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "team.task.created",
			TeamID:    teamID,
			Data: map[string]any{
				"team_id":     teamID,
				"task_id":     out.ID,
				"title":       out.Title,
				"status":      out.Status,
				"assigned_to": out.AssignedTo,
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodGet:
		items, err := s.teamStore.ListTasks(teamID)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCTeamMessages(w http.ResponseWriter, r *http.Request, teamID string) {
	switch r.Method {
	case http.MethodPost:
		var req struct {
			From    string `json:"from"`
			To      string `json:"to"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.teamStore.SendMessage(teamID, req.From, req.To, req.Content)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "team.message.sent",
			TeamID:    teamID,
			Data: map[string]any{
				"team_id": teamID,
				"from":    out.From,
				"to":      out.To,
				"content": truncateText(normalizeSpaces(out.Content), 220),
			},
		})
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodGet:
		agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
		if agentID == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "agent_id is required")
			return
		}
		items, err := s.teamStore.ReadMailbox(teamID, agentID)
		if err != nil {
			writeTeamStoreError(w, err)
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agent_id": agentID,
			"data":     items,
			"count":    len(items),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCTeamOrchestrate(w http.ResponseWriter, r *http.Request, teamID string) {
	if err := s.teamStore.Orchestrate(r.Context(), teamID); err != nil {
		writeTeamStoreError(w, err)
		return
	}
	info, ok := s.teamStore.Get(teamID)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found_error", "team not found")
		return
	}
	tasks, err := s.teamStore.ListTasks(teamID)
	if err != nil {
		writeTeamStoreError(w, err)
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: "team.orchestrated",
		TeamID:    teamID,
		Data: map[string]any{
			"team_id": teamID,
			"count":   len(tasks),
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"team":  info,
		"tasks": tasks,
		"count": len(tasks),
	})
}

func writeTeamStoreError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", msg)
	case strings.Contains(msg, "already exists"):
		writeErrorEnvelope(w, http.StatusConflict, "invalid_request_error", msg)
	default:
		writeErrorEnvelope(w, http.StatusBadRequest, "invalid_request_error", msg)
	}
}
