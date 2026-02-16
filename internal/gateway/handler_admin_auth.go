package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ccgateway/internal/auth"
	"ccgateway/internal/token"
)

func (s *server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.authService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "auth service not configured")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Support pagination and search
		limit := 0
		offset := 0
		search := ""

		if l := strings.TrimSpace(r.URL.Query().Get("limit")); l != "" {
			if n, err := strconv.Atoi(l); err == nil && n > 0 {
				limit = n
			}
		}
		if o := strings.TrimSpace(r.URL.Query().Get("offset")); o != "" {
			if n, err := strconv.Atoi(o); err == nil && n > 0 {
				offset = n
			}
		}
		if s := strings.TrimSpace(r.URL.Query().Get("search")); s != "" {
			search = s
		}

		users := s.authService.List()

		// Simple search filter
		if search != "" {
			filtered := make([]*auth.User, 0)
			search = strings.ToLower(search)
			for _, u := range users {
				if strings.Contains(strings.ToLower(u.Username), search) ||
					strings.Contains(strings.ToLower(u.Email), search) ||
					strings.Contains(strings.ToLower(u.DisplayName), search) {
					filtered = append(filtered, u)
				}
			}
			users = filtered
		}
		sortUsersForAdmin(users)

		// Simple pagination
		total := len(users)
		if offset > 0 {
			if offset >= total {
				users = []*auth.User{}
			} else {
				users = users[offset:]
			}
		}
		if limit > 0 && limit < len(users) {
			users = users[:limit]
		}

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data":  users,
			"total": total,
		})
	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Email    string `json:"email"`
			Role     string `json:"role"`
			Group    string `json:"group"`
			Quota    int64  `json:"quota"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}

		var user *auth.User
		var err error
		if req.Email != "" {
			user, err = s.authService.RegisterWithEmail(req.Username, req.Email, req.Password, req.Role)
		} else {
			user, err = s.authService.Register(req.Username, req.Password, req.Role)
		}
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		// Set group if specified
		if req.Group != "" {
			user.Group = req.Group
			if err := s.authService.Update(user); err != nil {
				s.cleanupCreatedUserOnAdminCreate(user)
				s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
		}

		// Add quota if specified
		if req.Quota > 0 {
			if err := s.authService.AddQuota(user.ID, req.Quota); err != nil {
				s.cleanupCreatedUserOnAdminCreate(user)
				s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
				return
			}
			updated, err := s.authService.Get(user.ID)
			if err == nil && updated != nil {
				user = updated
			}
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(user)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *server) handleAdminUserByPath(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.authService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "auth service not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin/auth/users/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		s.writeError(w, http.StatusNotFound, "not_found", "user not found")
		return
	}
	userID := parts[0]

	// Handle sub-paths
	if len(parts) >= 2 {
		switch parts[1] {
		case "tokens":
			// /admin/auth/users/{userID}/tokens/{tokenID}
			if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
				s.handleAdminTokenByID(w, r, userID, parts[2])
				return
			}
			// /admin/auth/users/{userID}/tokens
			s.handleAdminUserTokens(w, r, userID)
			return
		case "quota":
			s.handleAdminUserQuota(w, r, userID)
			return
		}
	}

	// Handle user CRUD
	switch r.Method {
	case http.MethodGet:
		user, err := s.authService.Get(userID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(user)
	case http.MethodPut:
		var req struct {
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Email       string `json:"email"`
			Role        string `json:"role"`
			Group       string `json:"group"`
			Status      *int   `json:"status"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}

		user, err := s.authService.Get(userID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}

		if req.Username != "" {
			user.Username = req.Username
		}
		if req.DisplayName != "" {
			user.DisplayName = req.DisplayName
		}
		if req.Email != "" {
			user.Email = req.Email
		}
		if req.Role != "" {
			user.Role = req.Role
		}
		if req.Group != "" {
			user.Group = req.Group
		}
		if req.Status != nil {
			user.Status = normalizeUserStatusInput(*req.Status)
		}

		err = s.authService.Update(user)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(user)
	case http.MethodDelete:
		err := s.authService.Delete(userID)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *server) handleAdminUserQuota(w http.ResponseWriter, r *http.Request, userID string) {
	if s.authService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "auth service not configured")
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, err := s.authService.Get(userID)
		if err != nil {
			s.writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"quota":         user.Quota,
			"used_quota":    user.UsedQuota,
			"remaining":     user.Quota - user.UsedQuota,
			"request_count": user.RequestCount,
		})
	case http.MethodPost:
		var req struct {
			Amount int64 `json:"amount"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}

		if req.Amount == 0 {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "amount is required")
			return
		}

		var err error
		if req.Amount > 0 {
			err = s.authService.AddQuota(userID, req.Amount)
		} else {
			user, getErr := s.authService.Get(userID)
			if getErr != nil {
				s.writeError(w, http.StatusNotFound, "not_found", getErr.Error())
				return
			}
			nextQuota := user.Quota + req.Amount
			if nextQuota < user.UsedQuota {
				s.writeError(w, http.StatusBadRequest, "invalid_request_error", "quota cannot be lower than used_quota")
				return
			}
			user.Quota = nextQuota
			err = s.authService.Update(user)
		}
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		user, _ := s.authService.Get(userID)
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"quota":   user.Quota,
			"message": "quota updated successfully",
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (s *server) handleAdminUserTokens(w http.ResponseWriter, r *http.Request, userID string) {
	if s.tokenService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "token service not configured")
		return
	}
	if s.authService != nil {
		if _, err := s.authService.Get(userID); err != nil {
			s.writeError(w, http.StatusNotFound, "not_found", err.Error())
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		tokens := s.tokenService.List(userID)
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data": tokens,
		})
	case http.MethodPost:
		var req struct {
			Quota     int64  `json:"quota"`
			Name      string `json:"name"`
			Models    string `json:"models"`
			Subnet    string `json:"subnet"`
			ExpiredAt int64  `json:"expired_at"` // Unix timestamp, -1 = never
			Status    *int   `json:"status,omitempty"`
		}
		// Allow empty body for default token
		if err := decodeJSONBodyStrict(r, &req, true); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}

		tk, err := s.tokenService.Generate(userID, req.Quota)
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "api_error", err.Error())
			return
		}

		// Apply additional settings
		if req.Name != "" {
			tk.Name = req.Name
		}
		if req.Models != "" {
			tk.Models = &req.Models
		}
		if req.Subnet != "" {
			tk.Subnet = &req.Subnet
		}
		if req.ExpiredAt != 0 {
			tk.ExpiredAt = req.ExpiredAt
		}
		if req.Status != nil {
			tk.Status = normalizeTokenStatusInput(*req.Status)
		}

		if err := s.tokenService.Update(tk); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(tk)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// handleAdminTokenByPath handles individual token operations (legacy path alias).
// Path: /admin/auth/tokens/{userID}/{tokenID}
func (s *server) handleAdminTokenByPath(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(w, r) {
		return
	}
	if s.tokenService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "token service not configured")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/admin/auth/tokens/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		s.writeError(w, http.StatusNotFound, "not_found", "token not found")
		return
	}

	userID := parts[0]
	tokenID := parts[1]
	s.handleAdminTokenByID(w, r, userID, tokenID)
}

func (s *server) handleAdminTokenByID(w http.ResponseWriter, r *http.Request, userID, tokenID string) {
	if s.tokenService == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "token service not configured")
		return
	}
	tk, err := s.getTokenByID(userID, tokenID)
	if err != nil {
		s.writeError(w, http.StatusNotFound, "not_found", err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(tk)
	case http.MethodPut:
		var req struct {
			Name      *string `json:"name"`
			Quota     *int64  `json:"quota"`
			Status    *int    `json:"status"`
			Models    *string `json:"models"`
			Subnet    *string `json:"subnet"`
			ExpiredAt *int64  `json:"expired_at"`
		}
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid json")
			return
		}

		if req.Name != nil {
			tk.Name = *req.Name
		}
		if req.Quota != nil {
			tk.Quota = *req.Quota
			tk.UnlimitedQuota = *req.Quota <= 0
		}
		if req.Status != nil {
			tk.Status = normalizeTokenStatusInput(*req.Status)
		}
		if req.Models != nil {
			tk.Models = req.Models
		}
		if req.Subnet != nil {
			tk.Subnet = req.Subnet
		}
		if req.ExpiredAt != nil {
			tk.ExpiredAt = *req.ExpiredAt
		}

		err := s.tokenService.Update(tk)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		w.Header().Set("content-type", "application/json")
		json.NewEncoder(w).Encode(tk)
	case http.MethodDelete:
		err := s.tokenService.Delete(tk.Value)
		if err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

// getTokenByID retrieves a token by user ID and token ID
func (s *server) getTokenByID(userID, tokenID string) (*token.Token, error) {
	if s.tokenService == nil {
		return nil, token.ErrInvalidToken
	}
	tokens := s.tokenService.List(userID)
	id, err := strconv.ParseInt(tokenID, 10, 64)
	if err != nil {
		return nil, err
	}
	for _, tk := range tokens {
		if tk.ID == id {
			return tk, nil
		}
	}
	return nil, token.ErrInvalidToken
}

func normalizeUserStatusInput(status int) int {
	switch status {
	case 0:
		return auth.StatusDisabled
	case auth.StatusEnabled, auth.StatusDisabled, auth.StatusDeleted:
		return status
	default:
		return auth.StatusEnabled
	}
}

func normalizeTokenStatusInput(status int) int {
	switch status {
	case 0:
		return token.StatusDisabled
	case token.StatusEnabled, token.StatusDisabled, token.StatusExpired, token.StatusExhausted:
		return status
	default:
		return token.StatusEnabled
	}
}

func (s *server) cleanupCreatedUserOnAdminCreate(user *auth.User) {
	if s.authService == nil || user == nil {
		return
	}
	_ = s.authService.Delete(user.ID)
}

func sortUsersForAdmin(users []*auth.User) {
	sort.Slice(users, func(i, j int) bool {
		left := users[i]
		right := users[j]
		if left == nil {
			return false
		}
		if right == nil {
			return true
		}
		lu := strings.ToLower(strings.TrimSpace(left.Username))
		ru := strings.ToLower(strings.TrimSpace(right.Username))
		if lu == ru {
			return left.ID < right.ID
		}
		return lu < ru
	})
}
