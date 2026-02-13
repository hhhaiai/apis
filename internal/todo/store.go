package todo

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusBlocked    Status = "blocked"
	StatusCanceled   Status = "canceled"
)

type Todo struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	SessionID   string         `json:"session_id,omitempty"`
	RunID       string         `json:"run_id,omitempty"`
	PlanID      string         `json:"plan_id,omitempty"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Status      Status         `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

type CreateInput struct {
	ID          string         `json:"id,omitempty"`
	SessionID   string         `json:"session_id,omitempty"`
	RunID       string         `json:"run_id,omitempty"`
	PlanID      string         `json:"plan_id,omitempty"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

type UpdateInput struct {
	Title       *string         `json:"title,omitempty"`
	Description *string         `json:"description,omitempty"`
	Status      *string         `json:"status,omitempty"`
	Metadata    *map[string]any `json:"metadata,omitempty"`
}

type ListFilter struct {
	Limit     int
	Status    string
	SessionID string
	RunID     string
	PlanID    string
}

type StoreState struct {
	Counter uint64 `json:"counter"`
	Order   []string
	Todos   []Todo `json:"todos"`
}

type Store struct {
	mu       sync.RWMutex
	todos    map[string]Todo
	order    []string
	counter  uint64
	onChange func()
}

func NewStore() *Store {
	return &Store{
		todos: map[string]Todo{},
		order: []string{},
	}
}

func (s *Store) Create(in CreateInput) (Todo, error) {
	s.mu.Lock()
	out, err := s.createLocked(in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return out, err
}

func (s *Store) Get(id string) (Todo, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Todo{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	td, ok := s.todos[id]
	if !ok {
		return Todo{}, false
	}
	return cloneTodo(td), true
}

func (s *Store) Update(id string, in UpdateInput) (Todo, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Todo{}, fmt.Errorf("todo id is required")
	}
	s.mu.Lock()
	out, err := s.updateLocked(id, in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return out, err
}

func (s *Store) updateLocked(id string, in UpdateInput) (Todo, error) {
	td, ok := s.todos[id]
	if !ok {
		return Todo{}, fmt.Errorf("todo %q not found", id)
	}

	if in.Title != nil {
		td.Title = strings.TrimSpace(*in.Title)
		if td.Title == "" {
			return Todo{}, fmt.Errorf("todo title is required")
		}
	}
	if in.Description != nil {
		td.Description = strings.TrimSpace(*in.Description)
	}
	if in.Metadata != nil {
		td.Metadata = copyMetadata(*in.Metadata)
	}
	if in.Status != nil {
		next, err := parseStatus(*in.Status, td.Status)
		if err != nil {
			return Todo{}, err
		}
		td.Status = next
		if next == StatusCompleted {
			now := time.Now().UTC()
			td.CompletedAt = &now
		} else {
			td.CompletedAt = nil
		}
	}
	td.UpdatedAt = time.Now().UTC()

	s.todos[id] = td
	return cloneTodo(td), nil
}

func (s *Store) List(filter ListFilter) []Todo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	status := strings.TrimSpace(strings.ToLower(filter.Status))
	sessionID := strings.TrimSpace(filter.SessionID)
	runID := strings.TrimSpace(filter.RunID)
	planID := strings.TrimSpace(filter.PlanID)

	out := make([]Todo, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.order[i]
		td, ok := s.todos[id]
		if !ok {
			continue
		}
		if status != "" && string(td.Status) != status {
			continue
		}
		if sessionID != "" && td.SessionID != sessionID {
			continue
		}
		if runID != "" && td.RunID != runID {
			continue
		}
		if planID != "" && td.PlanID != planID {
			continue
		}
		out = append(out, cloneTodo(td))
	}
	return out
}

func (s *Store) createLocked(in CreateInput) (Todo, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.nextIDLocked()
	}
	if _, exists := s.todos[id]; exists {
		return Todo{}, fmt.Errorf("todo %q already exists", id)
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Todo{}, fmt.Errorf("todo title is required")
	}
	status, err := parseStatus(in.Status, StatusPending)
	if err != nil {
		return Todo{}, err
	}

	now := time.Now().UTC()
	td := Todo{
		ID:          id,
		Type:        "todo",
		SessionID:   strings.TrimSpace(in.SessionID),
		RunID:       strings.TrimSpace(in.RunID),
		PlanID:      strings.TrimSpace(in.PlanID),
		Title:       title,
		Description: strings.TrimSpace(in.Description),
		Status:      status,
		Metadata:    copyMetadata(in.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if status == StatusCompleted {
		td.CompletedAt = &now
	}

	s.todos[id] = td
	s.order = append(s.order, id)
	return cloneTodo(td), nil
}

func parseStatus(raw string, defaultValue Status) (Status, error) {
	text := strings.TrimSpace(strings.ToLower(raw))
	if text == "" {
		return defaultValue, nil
	}
	switch Status(text) {
	case StatusPending, StatusInProgress, StatusCompleted, StatusBlocked, StatusCanceled:
		return Status(text), nil
	default:
		return "", fmt.Errorf("invalid todo status %q", raw)
	}
}

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("todo_%d_%x", time.Now().Unix(), n)
}

func (s *Store) Snapshot() StoreState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := StoreState{
		Counter: s.counter,
		Order:   append([]string(nil), s.order...),
		Todos:   make([]Todo, 0, len(s.order)),
	}
	for _, id := range s.order {
		if td, ok := s.todos[id]; ok {
			out.Todos = append(out.Todos, cloneTodo(td))
		}
	}
	if len(out.Todos) == 0 && len(s.todos) > 0 {
		for _, td := range s.todos {
			out.Todos = append(out.Todos, cloneTodo(td))
		}
	}
	return out
}

func (s *Store) Restore(state StoreState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]Todo, len(state.Todos))
	for _, td := range state.Todos {
		id := strings.TrimSpace(td.ID)
		if id == "" {
			return fmt.Errorf("todo id is required in restore state")
		}
		if _, exists := next[id]; exists {
			return fmt.Errorf("duplicate todo id in restore state: %s", id)
		}
		next[id] = cloneTodo(td)
	}
	order := normalizeOrder(state.Order, next)
	s.todos = next
	s.order = order
	s.counter = state.Counter
	return nil
}

func (s *Store) SetOnChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

func (s *Store) notifyChanged() {
	s.mu.RLock()
	fn := s.onChange
	s.mu.RUnlock()
	if fn != nil {
		fn()
	}
}

func cloneTodo(in Todo) Todo {
	out := in
	out.Metadata = copyMetadata(in.Metadata)
	if in.CompletedAt != nil {
		t := *in.CompletedAt
		out.CompletedAt = &t
	}
	return out
}

func copyMetadata(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

func normalizeOrder(order []string, entries map[string]Todo) []string {
	if len(entries) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(entries))
	out := make([]string, 0, len(entries))
	for _, raw := range order {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, exists := entries[id]; !exists {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == len(entries) {
		return out
	}
	for id := range entries {
		if _, exists := seen[id]; exists {
			continue
		}
		out = append(out, id)
	}
	return out
}
