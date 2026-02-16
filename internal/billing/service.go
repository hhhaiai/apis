package billing

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrInsufficientQuota = errors.New("insufficient quota")
	ErrTokenDisabled     = errors.New("token is disabled")
	ErrTokenExpired     = errors.New("token has expired")
)

// QuotaService manages quota consumption
type QuotaService struct {
	tokenQuota  map[string]int64
	userQuota   map[string]int64
	mu          sync.RWMutex
}

func NewQuotaService() *QuotaService {
	return &QuotaService{
		tokenQuota: make(map[string]int64),
		userQuota:  make(map[string]int64),
	}
}

// PreConsume checks if quota is available before request
// Returns the amount to deduct
func (s *QuotaService) PreConsume(tokenValue string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if amount <= 0 {
		return nil
	}

	quota, ok := s.tokenQuota[tokenValue]
	if !ok {
		return ErrInsufficientQuota
	}

	if quota < amount {
		return ErrInsufficientQuota
	}

	// Reserve the quota
	s.tokenQuota[tokenValue] = quota - amount

	return nil
}

// PostConsume finalizes quota consumption after request
func (s *QuotaService) PostConsume(tokenValue, userID string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if amount < 0 {
		// Refund
		s.tokenQuota[tokenValue] += -amount
		s.userQuota[userID] += -amount
		return nil
	}

	// Already deducted in PreConsume
	// Just update user quota
	s.userQuota[userID] += amount

	return nil
}

// CancelPreConsume cancels a pre-consumption (if request failed)
func (s *QuotaService) CancelPreConsume(tokenValue string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if amount <= 0 {
		return nil
	}

	s.tokenQuota[tokenValue] += amount
	return nil
}

// AddTokenQuota adds quota to a token
func (s *QuotaService) AddTokenQuota(tokenValue string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	s.tokenQuota[tokenValue] += amount
	return nil
}

// AddUserQuota adds quota to a user
func (s *QuotaService) AddUserQuota(userID string, amount int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if amount <= 0 {
		return errors.New("amount must be positive")
	}

	s.userQuota[userID] += amount
	return nil
}

// GetTokenQuota returns remaining token quota
func (s *QuotaService) GetTokenQuota(tokenValue string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.tokenQuota[tokenValue]
}

// GetUserQuota returns remaining user quota
func (s *QuotaService) GetUserQuota(userID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.userQuota[userID]
}

// SetTokenQuota sets token quota to a specific value
func (s *QuotaService) SetTokenQuota(tokenValue string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokenQuota[tokenValue] = amount
}

// SetUserQuota sets user quota to a specific value
func (s *QuotaService) SetUserQuota(userID string, amount int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.userQuota[userID] = amount
}

// BatchUpdater handles batch quota updates for performance
type BatchUpdater struct {
	updates   chan quotaUpdate
	closed    bool
	mu        sync.Mutex
}

type quotaUpdate struct {
	tokenValue string
	userID     string
	amount     int64
	isUser    bool
}

// NewBatchUpdater creates a new batch updater
func NewBatchUpdater(interval time.Duration) *BatchUpdater {
	b := &BatchUpdater{
		updates: make(chan quotaUpdate, 1000),
	}

	go b.processBatch(interval)

	return b
}

// QueueUpdate queues a quota update for batch processing
func (b *BatchUpdater) QueueUpdate(tokenValue, userID string, amount int64, isUser bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	select {
	case b.updates <- quotaUpdate{tokenValue, userID, amount, isUser}:
	default:
		// Queue full, process immediately
	}
}

func (b *BatchUpdater) processBatch(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Process in batches
	batch := make([]quotaUpdate, 0, 100)

	for {
		select {
		case update := <-b.updates:
			batch = append(batch, update)
			if len(batch) >= 100 {
				b.flushBatch(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				b.flushBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (b *BatchUpdater) flushBatch(batch []quotaUpdate) {
	// Process batch updates
	// In a real implementation, this would be a single DB transaction
	_ = batch
}

// Close closes the batch updater
func (b *BatchUpdater) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	close(b.updates)
}
