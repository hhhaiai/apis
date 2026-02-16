package auth

import (
	"errors"
	"time"
)

const (
	RoleGuest  = "guest"
	RoleUser   = "user"
	RoleAdmin  = "admin"
	RoleRoot   = "root"
)

const (
	StatusEnabled  = 1
	StatusDisabled = 2
	StatusDeleted  = 3
)

// User represents a system user
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Password     string    `json:"-"` // Never expose in JSON
	DisplayName  string    `json:"display_name,omitempty"`
	Email        string    `json:"email,omitempty"`
	Role         string    `json:"role"` // root, admin, user, guest
	Status       int       `json:"status"`
	Group        string    `json:"group"` // VIP group: "default", "vip", "enterprise"
	Quota        int64     `json:"quota"` // Total quota (in credits)
	UsedQuota    int64     `json:"used_quota"`
	RequestCount int       `json:"request_count"`

	// SSO Integration
	GitHubID string `json:"github_id,omitempty"`
	WeChatID string `json:"wechat_id,omitempty"`
	LarkID   string `json:"lark_id,omitempty"`

	// Access token for API
	AccessToken string `json:"access_token,omitempty"`

	// Invitation system
	AffCode   string `json:"aff_code,omitempty"`   // User's invitation code
	InviterID string `json:"inviter_id,omitempty"`  // Who invited this user

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrUserDisabled      = errors.New("user is disabled")
)

// NewUser creates a new user with default values
func NewUser(username, password, role string) *User {
	now := time.Now()
	return &User{
		ID:        username,
		Username:  username,
		Password:  password,
		Role:      role,
		Status:    StatusEnabled,
		Group:     "default",
		Quota:     0,
		UsedQuota: 0,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// IsAdmin checks if user has admin privileges
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin || u.Role == RoleRoot
}

// IsEnabled checks if user is active
func (u *User) IsEnabled() bool {
	return u.Status == StatusEnabled
}

// RemainingQuota returns remaining quota
func (u *User) RemainingQuota() int64 {
	if u.Quota <= 0 {
		return 0
	}
	result := u.Quota - u.UsedQuota
	if result < 0 {
		return 0
	}
	return result
}
