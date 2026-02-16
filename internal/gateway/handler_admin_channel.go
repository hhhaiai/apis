package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ccgateway/internal/channel"
)

// handleAdminChannels handles channel management
// GET /admin/channels - List all channels
// POST /admin/channels - Create channel
func (s *server) handleAdminChannels(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.channelStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "channel store not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		channels := s.channelStore.ListChannels()
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": channels,
		})
	case http.MethodPost:
		var ch channel.Channel
		if err := decodeJSONBodyStrict(r, &ch, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}

		if ch.Name == "" {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "name is required")
			return
		}
		if ch.Type == "" {
			ch.Type = "openai"
		}
		if ch.Status == 0 {
			ch.Status = channel.StatusEnabled
		}
		if ch.Group == "" {
			ch.Group = "default"
		}

		err := s.channelStore.AddChannel(&ch)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "api_error", err.Error())
			return
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ch)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// handleAdminChannelByPath handles individual channel operations
// GET /admin/channels/{id} - Get channel
// PUT /admin/channels/{id} - Update channel
// DELETE /admin/channels/{id} - Delete channel
func (s *server) handleAdminChannelByPath(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.channelStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "channel store not configured")
		return
	}

	id, suffix, err := parseChannelPath(r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid channel id")
		return
	}
	if suffix != "" {
		switch suffix {
		case "status":
			s.handleAdminChannelStatusByID(w, r, id)
			return
		case "test":
			s.handleAdminChannelTestByID(w, r, id)
			return
		default:
			s.writeError(w, http.StatusNotFound, "not_found", "channel endpoint not found")
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		ch, ok := s.channelStore.GetChannel(id)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found", "channel not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(ch)
	case http.MethodPut:
		var req channel.Channel
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}

		existing, ok := s.channelStore.GetChannel(id)
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found", "channel not found")
			return
		}

		// Update fields
		if req.Name != "" {
			existing.Name = req.Name
		}
		if req.Type != "" {
			existing.Type = req.Type
		}
		if req.Key != "" {
			existing.Key = req.Key
		}
		if req.BaseURL != nil {
			existing.BaseURL = req.BaseURL
		}
		if req.Models != "" {
			existing.Models = req.Models
		}
		if req.Group != "" {
			existing.Group = req.Group
		}
		if req.Weight > 0 {
			existing.Weight = req.Weight
		}
		if req.Priority != 0 {
			existing.Priority = req.Priority
		}
		if req.Status != 0 {
			existing.Status = req.Status
		}
		if req.Config != "" {
			existing.Config = req.Config
		}
		if req.ModelMapping != nil {
			existing.ModelMapping = req.ModelMapping
		}

		err = s.channelStore.UpdateChannel(existing)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "api_error", err.Error())
			return
		}

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(existing)
	case http.MethodDelete:
		err = s.channelStore.DeleteChannel(id)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "api_error", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// handleAdminChannelStatus handles channel status updates
// PUT /admin/channels/{id}/status - Enable/disable channel
func (s *server) handleAdminChannelStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.channelStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "channel store not configured")
		return
	}

	id, suffix, err := parseChannelPath(r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid channel id")
		return
	}
	if suffix != "" && suffix != "status" {
		s.writeError(w, http.StatusNotFound, "not_found", "channel endpoint not found")
		return
	}
	s.handleAdminChannelStatusByID(w, r, id)
}

func (s *server) handleAdminChannelStatusByID(w http.ResponseWriter, r *http.Request, id int64) {

	if r.Method != http.MethodPut {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	var req struct {
		Status int `json:"status"`
	}
	if err := decodeJSONBodyStrict(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
		return
	}

	if err := s.channelStore.UpdateChannelStatus(id, req.Status); err != nil {
		s.writeError(w, http.StatusBadRequest, "api_error", err.Error())
		return
	}

	w.Header().Set("content-type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":  req.Status,
		"message": "channel status updated",
	})
}

// handleAdminChannelTest tests a channel
// POST /admin/channels/{id}/test - Test channel connectivity
func (s *server) handleAdminChannelTest(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.channelStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "channel store not configured")
		return
	}

	id, suffix, err := parseChannelPath(r.URL.Path)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid channel id")
		return
	}
	if suffix != "" && suffix != "test" {
		s.writeError(w, http.StatusNotFound, "not_found", "channel endpoint not found")
		return
	}
	s.handleAdminChannelTestByID(w, r, id)
}

func (s *server) handleAdminChannelTestByID(w http.ResponseWriter, r *http.Request, id int64) {

	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	ch, ok := s.channelStore.GetChannel(id)
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "channel not found")
		return
	}

	// Channels without base_url cannot be probed over network.
	if ch.BaseURL == nil || strings.TrimSpace(*ch.BaseURL) == "" {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "skipped",
			"message": "channel test skipped: base_url is empty",
		})
		return
	}

	target := strings.TrimSpace(*ch.BaseURL)
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "channel base_url must be http or https")
		return
	}

	started := time.Now()
	req, err := http.NewRequest(http.MethodHead, target, nil)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if strings.TrimSpace(ch.Key) != "" {
		req.Header.Set("authorization", "Bearer "+strings.TrimSpace(ch.Key))
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	latencyMS := time.Since(started).Milliseconds()
	if err != nil {
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":     "error",
			"message":    err.Error(),
			"latency_ms": latencyMS,
		})
		return
	}
	defer resp.Body.Close()

	result := "ok"
	if resp.StatusCode >= 500 {
		result = "degraded"
	}
	w.Header().Set("content-type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":      result,
		"http_status": resp.StatusCode,
		"latency_ms":  latencyMS,
		"url":         target,
	})
}

func parseChannelPath(rawPath string) (int64, string, error) {
	path := strings.TrimPrefix(rawPath, "/admin/channels/")
	path = strings.Trim(path, "/")
	if path == "" {
		return 0, "", fmt.Errorf("channel id is required")
	}
	parts := strings.Split(path, "/")
	if len(parts) > 2 {
		return 0, "", fmt.Errorf("invalid channel path")
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", err
	}
	if len(parts) == 1 {
		return id, "", nil
	}
	return id, strings.ToLower(strings.TrimSpace(parts[1])), nil
}
