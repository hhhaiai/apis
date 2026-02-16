package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Service defines the interface for user management
type Service interface {
	Register(username, password, role string) (*User, error)
	RegisterWithEmail(username, email, password, role string) (*User, error)
	Login(username, password string) (*User, error)
	Get(id string) (*User, error)
	GetByEmail(email string) (*User, error)
	List() []*User
	Update(user *User) error
	Delete(id string) error

	// Quota management
	AddQuota(userID string, quota int64) error
	UseQuota(userID string, quota int64) error

	// SSO
	LinkGitHub(userID, githubID string) error
	LinkWeChat(userID, wechatID string) error
}

// InMemoryService implements Service using memory map
type InMemoryService struct {
	users     map[string]*User
	byEmail   map[string]*User
	byGitHub  map[string]*User
	byWeChat  map[string]*User
	byAffCode map[string]*User
	mu        sync.RWMutex
}

func NewInMemoryService() *InMemoryService {
	return &InMemoryService{
		users:     make(map[string]*User),
		byEmail:   make(map[string]*User),
		byGitHub:  make(map[string]*User),
		byWeChat:  make(map[string]*User),
		byAffCode: make(map[string]*User),
	}
}

func (s *InMemoryService) Register(username, password, role string) (*User, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	role = strings.TrimSpace(role)
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if role == "" {
		role = "user"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, u := range s.users {
		if u.Username == username {
			return nil, ErrUserAlreadyExists
		}
	}

	// Simple ID generation
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	user := NewUser(username, hashedPassword, role)

	s.users[user.ID] = user
	return cloneUser(user), nil
}

func (s *InMemoryService) Login(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	if username == "" || password == "" {
		return nil, ErrUserNotFound
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, u := range s.users {
		if u.Username == username && verifyPassword(u.Password, password) {
			if !u.IsEnabled() {
				return nil, ErrUserDisabled
			}
			return cloneUser(u), nil
		}
	}
	return nil, ErrUserNotFound
}

func (s *InMemoryService) Get(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if u, ok := s.users[id]; ok {
		return cloneUser(u), nil
	}
	return nil, ErrUserNotFound
}

func (s *InMemoryService) List() []*User {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*User, 0, len(s.users))
	for _, u := range s.users {
		list = append(list, cloneUser(u))
	}
	return list
}

func hashPassword(password string) (string, error) {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	salt := hex.EncodeToString(saltBytes)
	sum := sha256.Sum256([]byte(salt + ":" + password))
	return "sha256$" + salt + "$" + hex.EncodeToString(sum[:]), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 3 || parts[0] != "sha256" {
		// Backward-compatible plain text check for old records.
		return subtle.ConstantTimeCompare([]byte(encoded), []byte(password)) == 1
	}
	sum := sha256.Sum256([]byte(parts[1] + ":" + password))
	actual := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(actual), []byte(parts[2])) == 1
}

func (s *InMemoryService) RegisterWithEmail(username, email, password, role string) (*User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	password = strings.TrimSpace(password)
	role = strings.TrimSpace(role)
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}
	if role == "" {
		role = RoleUser
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check username
	for _, u := range s.users {
		if u.Username == username {
			return nil, ErrUserAlreadyExists
		}
	}

	// Check email
	if email != "" {
		if _, ok := s.byEmail[email]; ok {
			return nil, fmt.Errorf("email already in use")
		}
	}

	// Generate unique ID
	id := fmt.Sprintf("user-%d", time.Now().UnixNano())
	hashedPassword, err := hashPassword(password)
	if err != nil {
		return nil, err
	}

	// Generate access token and invitation code
	accessToken := fmt.Sprintf("at-%s-%d", username, time.Now().UnixNano())
	affCode := generateAffCode(username)

	user := &User{
		ID:          id,
		Username:    username,
		Password:    hashedPassword,
		Email:       email,
		Role:        role,
		Status:      StatusEnabled,
		Group:       "default",
		Quota:       0,
		UsedQuota:   0,
		AccessToken: accessToken,
		AffCode:     affCode,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	s.users[id] = user
	if email != "" {
		s.byEmail[email] = user
	}
	s.byAffCode[affCode] = user

	return cloneUser(user), nil
}

func (s *InMemoryService) GetByEmail(email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if u, ok := s.byEmail[email]; ok {
		return cloneUser(u), nil
	}
	return nil, ErrUserNotFound
}

func (s *InMemoryService) Update(user *User) error {
	if user == nil {
		return fmt.Errorf("user is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.users[user.ID]
	if !ok {
		return ErrUserNotFound
	}
	oldEmail := strings.TrimSpace(existing.Email)
	newEmail := strings.TrimSpace(user.Email)
	if oldEmail != newEmail {
		if newEmail != "" {
			if conflict, exists := s.byEmail[newEmail]; exists && conflict.ID != existing.ID {
				return fmt.Errorf("email already in use")
			}
		}
		if oldEmail != "" {
			delete(s.byEmail, oldEmail)
		}
		if newEmail != "" {
			s.byEmail[newEmail] = existing
		}
	}

	// Update fields
	existing.Username = user.Username
	existing.DisplayName = user.DisplayName
	existing.Email = newEmail
	existing.Role = user.Role
	existing.Status = user.Status
	existing.Group = user.Group
	existing.Quota = user.Quota
	existing.UsedQuota = user.UsedQuota
	existing.UpdatedAt = time.Now()

	return nil
}

func (s *InMemoryService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[id]
	if !ok {
		return ErrUserNotFound
	}

	// Soft delete
	user.Status = StatusDeleted
	user.Username = fmt.Sprintf("deleted_%s", user.ID)
	if email := strings.TrimSpace(user.Email); email != "" {
		delete(s.byEmail, email)
	}
	if githubID := strings.TrimSpace(user.GitHubID); githubID != "" {
		delete(s.byGitHub, githubID)
	}
	if wechatID := strings.TrimSpace(user.WeChatID); wechatID != "" {
		delete(s.byWeChat, wechatID)
	}
	if affCode := strings.TrimSpace(user.AffCode); affCode != "" {
		delete(s.byAffCode, affCode)
	}

	return nil
}

func (s *InMemoryService) AddQuota(userID string, quota int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return ErrUserNotFound
	}

	if quota < 0 {
		return fmt.Errorf("quota cannot be negative")
	}

	user.Quota += quota
	user.UpdatedAt = time.Now()

	return nil
}

func (s *InMemoryService) UseQuota(userID string, quota int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return ErrUserNotFound
	}

	if quota < 0 {
		return fmt.Errorf("quota cannot be negative")
	}

	user.UsedQuota += quota
	user.RequestCount++
	user.UpdatedAt = time.Now()

	return nil
}

func (s *InMemoryService) LinkGitHub(userID, githubID string) error {
	userID = strings.TrimSpace(userID)
	githubID = strings.TrimSpace(githubID)
	if userID == "" || githubID == "" {
		return fmt.Errorf("user id and github id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return ErrUserNotFound
	}

	if linked, exists := s.byGitHub[githubID]; exists && linked.ID != userID {
		return fmt.Errorf("github account already linked")
	}
	if old := strings.TrimSpace(user.GitHubID); old != "" && old != githubID {
		delete(s.byGitHub, old)
	}
	user.GitHubID = githubID
	s.byGitHub[githubID] = user
	user.UpdatedAt = time.Now()

	return nil
}

func (s *InMemoryService) LinkWeChat(userID, wechatID string) error {
	userID = strings.TrimSpace(userID)
	wechatID = strings.TrimSpace(wechatID)
	if userID == "" || wechatID == "" {
		return fmt.Errorf("user id and wechat id are required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.users[userID]
	if !ok {
		return ErrUserNotFound
	}

	if linked, exists := s.byWeChat[wechatID]; exists && linked.ID != userID {
		return fmt.Errorf("wechat account already linked")
	}
	if old := strings.TrimSpace(user.WeChatID); old != "" && old != wechatID {
		delete(s.byWeChat, old)
	}
	user.WeChatID = wechatID
	s.byWeChat[wechatID] = user
	user.UpdatedAt = time.Now()

	return nil
}

// Helper functions
func generateAffCode(username string) string {
	// Simple generation - could be enhanced
	if len(username) < 4 {
		return fmt.Sprintf("%s%d", username, time.Now().Unix()%10000)
	}
	return fmt.Sprintf("%s%d", username[:4], time.Now().Unix()%10000)
}

func cloneUser(u *User) *User {
	if u == nil {
		return nil
	}
	cp := *u
	return &cp
}
