package gateway

import (
	"encoding/json"
	"net/http"
	"strings"

	"ccgateway/internal/skill"
)

// SkillEngine defines the interface for skill operations.
type SkillEngine interface {
	Register(s skill.Skill) error
	Get(name string) (skill.Skill, bool)
	List() []skill.Skill
	Delete(name string) error
	Execute(name string, params map[string]any) (string, error)
}

func (s *server) handleCCSkills(w http.ResponseWriter, r *http.Request) {
	if s.skillEngine == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "skill engine is not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		skills := s.skillEngine.List()
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  skills,
			"count": len(skills),
		})
	case http.MethodPost:
		var sk skill.Skill
		if err := decodeJSONBodyStrict(r, &sk, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		if err := s.skillEngine.Register(sk); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(sk)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCSkillByPath(w http.ResponseWriter, r *http.Request) {
	if s.skillEngine == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "skill engine is not configured")
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/v1/cc/skills/")
	// Check for execute sub-path
	if strings.HasSuffix(name, "/execute") {
		name = strings.TrimSuffix(name, "/execute")
		s.handleCCSkillExecute(w, r, name)
		return
	}

	switch r.Method {
	case http.MethodGet:
		sk, ok := s.skillEngine.Get(name)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "skill not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(sk)
	case http.MethodDelete:
		if err := s.skillEngine.Delete(name); err != nil {
			s.writeError(w, http.StatusNotFound, "not_found_error", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCSkillExecute(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	var body struct {
		Parameters map[string]any `json:"parameters"`
	}
	if err := decodeJSONBodyStrict(r, &body, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if body.Parameters == nil {
		body.Parameters = map[string]any{}
	}

	result, err := s.skillEngine.Execute(name, body.Parameters)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"result": result,
	})
}
