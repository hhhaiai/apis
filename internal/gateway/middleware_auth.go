package gateway

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"

	"ccgateway/internal/token"
)

type contextKey string

const (
	tokenContextKey contextKey = "token"
	userContextKey  contextKey = "user"
)

func (s *server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Check for Admin Token (Backwards Compatibility & Admin Routes).
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// Try query param for simple file access or websocket.
			authHeader = r.URL.Query().Get("token")
			if authHeader != "" {
				authHeader = "Bearer " + authHeader
			}
		}

		tokenStr := bearerToken(authHeader)
		adminToken := strings.TrimSpace(s.adminToken)
		if adminToken != "" && tokenStr == adminToken {
			next(w, r)
			return
		}
		if adminToken == "" && s.tokenService == nil {
			// Fully open mode for local/dev compatibility when no auth is configured.
			next(w, r)
			return
		}

		// 2. Check User Token.
		if s.tokenService != nil {
			tk, err := s.tokenService.Validate(tokenStr)
			if err == nil {
				if err := enforceTokenIPAccess(tk, r); err != nil {
					s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
					return
				}
				ctx := context.WithValue(r.Context(), tokenContextKey, tk)
				next(w, r.WithContext(ctx))
				return
			}
		}

		// 3. Unauthorized.
		s.writeError(w, http.StatusUnauthorized, "auth_error", "invalid authentication credentials")
	}
}

// withTokenQuota performs a pre-check to block obviously exhausted tokens.
func (s *server) withTokenQuota(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tk, ok := r.Context().Value(tokenContextKey).(*token.Token)
		if !ok || tk == nil {
			// If no user token (e.g. Admin Token used), skip quota check.
			next(w, r)
			return
		}

		if !tk.UnlimitedQuota && tk.Quota <= 0 {
			s.writeError(w, http.StatusForbidden, "quota_error", "quota exceeded")
			return
		}

		next(w, r)
	}
}

func (s *server) reserveQuotaFromRequestContext(ctx context.Context, amount int64) error {
	if amount <= 0 || s.tokenService == nil {
		return nil
	}
	tk, ok := ctx.Value(tokenContextKey).(*token.Token)
	if !ok || tk == nil || strings.TrimSpace(tk.Value) == "" {
		return nil
	}
	return s.tokenService.DeductQuota(tk.Value, amount)
}

func (s *server) deductQuotaFromRequestContext(ctx context.Context, amount int64) error {
	return s.reserveQuotaFromRequestContext(ctx, amount)
}

func (s *server) refundQuotaFromRequestContext(ctx context.Context, amount int64) error {
	if amount <= 0 || s.tokenService == nil {
		return nil
	}
	tk, ok := ctx.Value(tokenContextKey).(*token.Token)
	if !ok || tk == nil || strings.TrimSpace(tk.Value) == "" {
		return nil
	}
	return s.tokenService.RefundQuota(tk.Value, amount)
}

func (s *server) settleQuotaFromRequestContext(ctx context.Context, reserved, actual int64) error {
	if reserved < 0 {
		reserved = 0
	}
	if actual <= 0 {
		actual = 1
	}
	switch {
	case reserved == 0:
		return s.reserveQuotaFromRequestContext(ctx, actual)
	case actual > reserved:
		return s.reserveQuotaFromRequestContext(ctx, actual-reserved)
	case reserved > actual:
		return s.refundQuotaFromRequestContext(ctx, reserved-actual)
	default:
		return nil
	}
}

func usageToQuotaAmount(inputTokens, outputTokens int) int64 {
	total := inputTokens + outputTokens
	if total <= 0 {
		return 1
	}
	return int64(total)
}

func (s *server) enforceTokenModelAccess(ctx context.Context, model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	tk, ok := ctx.Value(tokenContextKey).(*token.Token)
	if !ok || tk == nil {
		return nil
	}
	if tk.CanUseModel(model) {
		return nil
	}
	return fmt.Errorf("token is not allowed to access model %q", model)
}

func bearerToken(authHeader string) string {
	authHeader = strings.TrimSpace(authHeader)
	if authHeader == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return authHeader
	}
	return strings.TrimSpace(authHeader[len("Bearer "):])
}

func enforceTokenIPAccess(tk *token.Token, r *http.Request) error {
	if tk == nil {
		return nil
	}
	clientIP := requestClientIP(r)
	if tk.CanUseIP(clientIP) {
		return nil
	}
	if clientIP == "" {
		return fmt.Errorf("token is not allowed from this client ip")
	}
	return fmt.Errorf("token is not allowed from client ip %q", clientIP)
}

func requestClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if forwarded := firstHeaderValue(r.Header.Get("x-forwarded-for")); forwarded != "" {
		return forwarded
	}
	if realIP := strings.TrimSpace(r.Header.Get("x-real-ip")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil {
		return strings.TrimSpace(host)
	}
	return strings.TrimSpace(r.RemoteAddr)
}

func firstHeaderValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexByte(raw, ','); idx >= 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw)
}
