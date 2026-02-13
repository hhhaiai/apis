package session

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Session struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"`
	ParentID  string           `json:"parent_id,omitempty"`
	Title     string           `json:"title,omitempty"`
	Metadata  map[string]any   `json:"metadata,omitempty"`
	Messages  []SessionMessage `json:"messages,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

// SessionMessage represents a message in a session's conversation history.
type SessionMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateInput struct {
	ID       string         `json:"id,omitempty"`
	Title    string         `json:"title,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type Store struct {
	mu       sync.RWMutex
	sessions map[string]Session
	order    []string
	counter  uint64
}

func NewStore() *Store {
	return &Store{
		sessions: map[string]Session{},
		order:    []string{},
	}
}

func (s *Store) Create(in CreateInput) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createLocked("", in)
}

func (s *Store) Fork(parentID string, in CreateInput) (Session, error) {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return Session{}, fmt.Errorf("parent session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	parent, ok := s.sessions[parentID]
	if !ok {
		return Session{}, fmt.Errorf("session %q not found", parentID)
	}

	if strings.TrimSpace(in.Title) == "" {
		in.Title = parent.Title
	}
	if in.Metadata == nil {
		in.Metadata = copyMetadata(parent.Metadata)
	}
	return s.createLocked(parentID, in)
}

func (s *Store) Get(id string) (Session, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Session{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.sessions[id]
	if !ok {
		return Session{}, false
	}
	return cloneSession(v), true
}

func (s *Store) List(limit int) []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	out := make([]Session, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.order[i]
		if sess, ok := s.sessions[id]; ok {
			out = append(out, cloneSession(sess))
		}
	}
	return out
}

// AppendMessage adds a message to a session's conversation history.
func (s *Store) AppendMessage(sessionID string, msg SessionMessage) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return fmt.Errorf("session id is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session %q not found", sessionID)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	sess.Messages = append(sess.Messages, msg)
	sess.UpdatedAt = time.Now().UTC()
	s.sessions[sessionID] = sess
	return nil
}

// GetMessages returns a copy of a session's conversation history.
func (s *Store) GetMessages(sessionID string) ([]SessionMessage, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sess, ok := s.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session %q not found", sessionID)
	}
	return cloneMessages(sess.Messages), nil
}

func (s *Store) createLocked(parentID string, in CreateInput) (Session, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.nextIDLocked()
	}
	if _, exists := s.sessions[id]; exists {
		return Session{}, fmt.Errorf("session %q already exists", id)
	}

	now := time.Now().UTC()
	sess := Session{
		ID:        id,
		Type:      "session",
		ParentID:  strings.TrimSpace(parentID),
		Title:     strings.TrimSpace(in.Title),
		Metadata:  copyMetadata(in.Metadata),
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.sessions[id] = sess
	s.order = append(s.order, id)
	return cloneSession(sess), nil
}

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("sess_%d_%x", time.Now().Unix(), n)
}

func cloneSession(in Session) Session {
	out := in
	out.Metadata = copyMetadata(in.Metadata)
	out.Messages = cloneMessages(in.Messages)
	return out
}

func cloneMessages(msgs []SessionMessage) []SessionMessage {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]SessionMessage, len(msgs))
	copy(out, msgs)
	return out
}

func copyMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}
