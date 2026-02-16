package memory

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MemoryStore 记忆存储接口
type MemoryStore interface {
	// 工作记忆
	GetWorkingMemory(ctx context.Context, sessionID string) (*WorkingMemory, error)
	UpdateWorkingMemory(ctx context.Context, wm *WorkingMemory) error

	// 会话记忆
	GetSessionMemory(ctx context.Context, sessionID string) (*SessionMemory, error)
	UpdateSessionMemory(ctx context.Context, sm *SessionMemory) error

	// 长期记忆
	GetLongTermMemory(ctx context.Context, userID string) (*LongTermMemory, error)
	UpdateLongTermMemory(ctx context.Context, ltm *LongTermMemory) error

	// 清理过期记忆
	CleanupExpired(ctx context.Context, ttl time.Duration) error
}

// InMemoryStore 内存实现
type InMemoryStore struct {
	workingMemory  map[string]*WorkingMemory
	sessionMemory  map[string]*SessionMemory
	longTermMemory map[string]*LongTermMemory
	mu             sync.RWMutex
}

// NewInMemoryStore 创建内存存储
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		workingMemory:  make(map[string]*WorkingMemory),
		sessionMemory:  make(map[string]*SessionMemory),
		longTermMemory: make(map[string]*LongTermMemory),
	}
}

// GetWorkingMemory 获取工作记忆
func (s *InMemoryStore) GetWorkingMemory(ctx context.Context, sessionID string) (*WorkingMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	wm, ok := s.workingMemory[sessionID]
	if !ok {
		// 返回空的工作记忆
		return &WorkingMemory{
			SessionID:  sessionID,
			Messages:   []Message{},
			LastUpdate: time.Now(),
			TokenCount: 0,
		}, nil
	}

	return wm, nil
}

// UpdateWorkingMemory 更新工作记忆
func (s *InMemoryStore) UpdateWorkingMemory(ctx context.Context, wm *WorkingMemory) error {
	if wm == nil {
		return fmt.Errorf("working memory is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 只在 LastUpdate 为零值时设置为当前时间
	if wm.LastUpdate.IsZero() {
		wm.LastUpdate = time.Now()
	}
	s.workingMemory[wm.SessionID] = wm

	return nil
}

// GetSessionMemory 获取会话记忆
func (s *InMemoryStore) GetSessionMemory(ctx context.Context, sessionID string) (*SessionMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sm, ok := s.sessionMemory[sessionID]
	if !ok {
		// 返回空的会话记忆
		return &SessionMemory{
			SessionID:      sessionID,
			ProjectMeta:    make(map[string]interface{}),
			FileOperations: []FileOp{},
			UserPrefs:      make(map[string]string),
			Summary:        "",
			LastUpdate:     time.Now(),
			TokenCount:     0,
		}, nil
	}

	return sm, nil
}

// UpdateSessionMemory 更新会话记忆
func (s *InMemoryStore) UpdateSessionMemory(ctx context.Context, sm *SessionMemory) error {
	if sm == nil {
		return fmt.Errorf("session memory is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 只在 LastUpdate 为零值时设置为当前时间
	if sm.LastUpdate.IsZero() {
		sm.LastUpdate = time.Now()
	}
	s.sessionMemory[sm.SessionID] = sm

	return nil
}

// GetLongTermMemory 获取长期记忆
func (s *InMemoryStore) GetLongTermMemory(ctx context.Context, userID string) (*LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ltm, ok := s.longTermMemory[userID]
	if !ok {
		// 返回空的长期记忆
		return &LongTermMemory{
			UserID:         userID,
			CodingStyle:    "",
			TechStack:      []string{},
			ProjectHistory: []ProjectSummary{},
			Embeddings:     []float32{},
			CreatedAt:      time.Now(),
		}, nil
	}

	return ltm, nil
}

// UpdateLongTermMemory 更新长期记忆
func (s *InMemoryStore) UpdateLongTermMemory(ctx context.Context, ltm *LongTermMemory) error {
	if ltm == nil {
		return fmt.Errorf("long term memory is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.longTermMemory[ltm.UserID] = ltm

	return nil
}

// CleanupExpired 清理过期记忆
func (s *InMemoryStore) CleanupExpired(ctx context.Context, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-ttl)

	// 清理过期的工作记忆
	for id, wm := range s.workingMemory {
		if wm.LastUpdate.Before(cutoff) {
			delete(s.workingMemory, id)
		}
	}

	// 清理过期的会话记忆
	for id, sm := range s.sessionMemory {
		if sm.LastUpdate.Before(cutoff) {
			delete(s.sessionMemory, id)
		}
	}

	return nil
}

// GetStats 获取统计信息
func (s *InMemoryStore) GetStats() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]int{
		"working_memory_count":   len(s.workingMemory),
		"session_memory_count":   len(s.sessionMemory),
		"long_term_memory_count": len(s.longTermMemory),
	}
}
