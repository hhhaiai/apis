package token

import (
	"errors"
	"strings"
	"time"
)

const (
	StatusEnabled   = 1
	StatusDisabled  = 2
	StatusExpired   = 3
	StatusExhausted = 4
)

// Token represents an API access token.
// Quota is remaining quota when UnlimitedQuota is false.
type Token struct {
	ID     int64  `json:"id"`
	Value  string `json:"value"` // sk-xxxx
	UserID string `json:"user_id"`
	Name   string `json:"name,omitempty"` // Token name for identification

	Status         int   `json:"status"` // enabled, disabled, expired, exhausted
	Quota          int64 `json:"quota"`  // remaining quota (0 when exhausted)
	UnlimitedQuota bool  `json:"unlimited_quota"`
	Used           int64 `json:"used"` // total used

	// Restrictions
	Models *string `json:"models,omitempty"` // Comma-separated allowed models (empty = all)
	Subnet *string `json:"subnet,omitempty"` // Allowed IP addresses (empty = all)

	// Expiration
	CreatedAt  time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at,omitempty"`
	ExpiredAt  int64     `json:"expired_at"` // -1 = never expires, timestamp = expires at
}

var (
	ErrInvalidToken  = errors.New("invalid or expired token")
	ErrTokenDisabled = errors.New("token is disabled")
	ErrTokenExpired  = errors.New("token has expired")
	ErrQuotaExceeded = errors.New("quota exceeded")
)

// NewToken creates a new token
func NewToken(userID string, quota int64) *Token {
	now := time.Now()
	return &Token{
		UserID:         userID,
		Status:         StatusEnabled,
		Quota:          quota,
		UnlimitedQuota: quota <= 0,
		Used:           0,
		CreatedAt:      now,
		AccessedAt:     now,
		ExpiredAt:      -1, // Never expires by default
	}
}

// IsValid checks if token is valid
func (t *Token) IsValid() bool {
	if t.Status != StatusEnabled {
		return false
	}
	// Check expiration
	if t.ExpiredAt > 0 && t.ExpiredAt < time.Now().Unix() {
		return false
	}
	// Check quota
	if !t.UnlimitedQuota && t.Quota <= 0 {
		return false
	}
	return true
}

// RemainingQuota returns remaining quota
func (t *Token) RemainingQuota() int64 {
	if t.UnlimitedQuota {
		return -1 // Unlimited
	}
	if t.Quota < 0 {
		return 0
	}
	return t.Quota
}

// CanUseModel checks if token allows using specific model
func (t *Token) CanUseModel(model string) bool {
	if t.Models == nil || *t.Models == "" {
		return true // No restriction
	}
	allowed := *t.Models
	// Simple check - could be enhanced with more sophisticated matching
	return containsModel(allowed, model)
}

// CanUseIP checks if token allows using from specific IP
func (t *Token) CanUseIP(ip string) bool {
	if t.Subnet == nil || *t.Subnet == "" {
		return true // No restriction
	}
	allowed := *t.Subnet
	// Simple check - could be enhanced with CIDR matching
	return matchesIP(allowed, ip)
}

func containsModel(allowed, model string) bool {
	allowedList := splitAndTrim(allowed, ",")
	for _, m := range allowedList {
		if m == model {
			return true
		}
	}
	return false
}

func matchesIP(allowed, ip string) bool {
	// Simple single IP match - could be enhanced with CIDR
	ips := splitAndTrim(allowed, ",")
	for _, allowedIP := range ips {
		if allowedIP == ip {
			return true
		}
	}
	return false
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
