package token

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Service defines the interface for token management
type Service interface {
	Generate(userID string, quota int64) (*Token, error)
	Validate(tokenValue string) (*Token, error)
	DeductQuota(tokenValue string, amount int64) error
	RefundQuota(tokenValue string, amount int64) error
	List(userID string) []*Token
	Get(tokenValue string) (*Token, error)
	Update(token *Token) error
	Delete(tokenValue string) error
}

// InMemoryService implements Service using memory map
type InMemoryService struct {
	tokens   map[string]*Token
	tokenIDs map[int64]*Token
	nextID   int64
	mu       sync.RWMutex
}

func NewInMemoryService() *InMemoryService {
	return &InMemoryService{
		tokens:   make(map[string]*Token),
		tokenIDs: make(map[int64]*Token),
		nextID:   1,
	}
}

func (s *InMemoryService) Generate(userID string, quota int64) (*Token, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tokenValue, err := newTokenValue()
	if err != nil {
		return nil, err
	}

	token := &Token{
		ID:             s.nextID,
		Value:          tokenValue,
		UserID:         userID,
		Name:           "default",
		Status:         StatusEnabled,
		Quota:          maxInt64(0, quota),
		UnlimitedQuota: quota <= 0,
		Used:           0,
		CreatedAt:      time.Now(),
		AccessedAt:     time.Now(),
		ExpiredAt:      -1,
	}

	s.nextID++
	s.tokens[tokenValue] = token
	s.tokenIDs[token.ID] = token
	return token, nil
}

func (s *InMemoryService) Validate(tokenValue string) (*Token, error) {
	tokenValue = strings.TrimSpace(tokenValue)
	if tokenValue == "" {
		return nil, ErrInvalidToken
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	token, ok := s.tokens[tokenValue]
	if !ok {
		return nil, ErrInvalidToken
	}

	status := normalizeTokenStatus(token.Status)
	// Check status
	if status == StatusDisabled {
		return nil, ErrTokenDisabled
	}
	if status == StatusExpired || (token.ExpiredAt > 0 && token.ExpiredAt < time.Now().Unix()) {
		return nil, ErrTokenExpired
	}
	if status == StatusExhausted {
		return nil, ErrQuotaExceeded
	}

	// Check quota
	if !token.UnlimitedQuota && token.Quota <= 0 {
		return nil, ErrQuotaExceeded
	}

	return token, nil
}

func (s *InMemoryService) DeductQuota(tokenValue string, amount int64) error {
	tokenValue = strings.TrimSpace(tokenValue)
	if tokenValue == "" {
		return ErrInvalidToken
	}
	if amount <= 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.tokens[tokenValue]
	if !ok {
		return ErrInvalidToken
	}

	if !token.UnlimitedQuota && token.Quota < amount {
		token.Status = StatusExhausted
		return ErrQuotaExceeded
	}

	token.Used += amount
	if !token.UnlimitedQuota {
		token.Quota -= amount
		if token.Quota <= 0 {
			token.Status = StatusExhausted
		}
	}
	token.AccessedAt = time.Now()

	return nil
}

func (s *InMemoryService) RefundQuota(tokenValue string, amount int64) error {
	tokenValue = strings.TrimSpace(tokenValue)
	if tokenValue == "" {
		return ErrInvalidToken
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.tokens[tokenValue]
	if !ok {
		return ErrInvalidToken
	}

	if amount < 0 {
		amount = -amount
		token.Used -= amount
		if token.Used < 0 {
			token.Used = 0
		}
	} else {
		if !token.UnlimitedQuota {
			token.Quota += amount
		}
	}

	// Restore status if was exhausted
	if token.Status == StatusExhausted && token.RemainingQuota() > 0 {
		token.Status = StatusEnabled
	}

	token.AccessedAt = time.Now()
	return nil
}

func (s *InMemoryService) List(userID string) []*Token {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Token, 0)
	for _, t := range s.tokens {
		if t.UserID == userID {
			list = append(list, t)
		}
	}
	return list
}

func (s *InMemoryService) Get(tokenValue string) (*Token, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	token, ok := s.tokens[tokenValue]
	if !ok {
		return nil, ErrInvalidToken
	}
	return token, nil
}

func (s *InMemoryService) Update(token *Token) error {
	if token == nil {
		return ErrInvalidToken
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	value := strings.TrimSpace(token.Value)
	existing, ok := s.tokens[value]
	if !ok {
		return ErrInvalidToken
	}

	// Update fields
	existing.Name = token.Name
	existing.Quota = maxInt64(0, token.Quota)
	existing.UnlimitedQuota = token.UnlimitedQuota || token.Quota <= 0
	status := normalizeTokenStatus(token.Status)
	if status == StatusEnabled && !existing.UnlimitedQuota && existing.Quota <= 0 {
		status = StatusExhausted
	}
	existing.Status = status
	existing.Models = token.Models
	existing.Subnet = token.Subnet
	existing.ExpiredAt = token.ExpiredAt

	return nil
}

func (s *InMemoryService) Delete(tokenValue string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, ok := s.tokens[tokenValue]
	if !ok {
		return ErrInvalidToken
	}

	delete(s.tokens, tokenValue)
	delete(s.tokenIDs, token.ID)
	return nil
}

func newTokenValue() (string, error) {
	seed := make([]byte, 24)
	if _, err := rand.Read(seed); err != nil {
		return "", fmt.Errorf("generate token value: %w", err)
	}
	return "sk-" + hex.EncodeToString(seed), nil
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func normalizeTokenStatus(status int) int {
	switch status {
	case 0:
		return StatusDisabled
	case StatusEnabled, StatusDisabled, StatusExpired, StatusExhausted:
		return status
	default:
		return StatusEnabled
	}
}
