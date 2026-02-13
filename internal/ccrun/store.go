package ccrun

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

type Run struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	SessionID      string         `json:"session_id,omitempty"`
	Path           string         `json:"path"`
	Mode           string         `json:"mode,omitempty"`
	ClientModel    string         `json:"client_model,omitempty"`
	RequestedModel string         `json:"requested_model,omitempty"`
	UpstreamModel  string         `json:"upstream_model,omitempty"`
	Stream         bool           `json:"stream"`
	ToolCount      int            `json:"tool_count"`
	Status         Status         `json:"status"`
	StatusCode     int            `json:"status_code"`
	Error          string         `json:"error,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	CompletedAt    *time.Time     `json:"completed_at,omitempty"`
}

type CreateInput struct {
	ID             string         `json:"id,omitempty"`
	SessionID      string         `json:"session_id,omitempty"`
	Path           string         `json:"path"`
	Mode           string         `json:"mode,omitempty"`
	ClientModel    string         `json:"client_model,omitempty"`
	RequestedModel string         `json:"requested_model,omitempty"`
	UpstreamModel  string         `json:"upstream_model,omitempty"`
	Stream         bool           `json:"stream,omitempty"`
	ToolCount      int            `json:"tool_count,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type CompleteInput struct {
	StatusCode int    `json:"status_code"`
	Error      string `json:"error,omitempty"`
}

type ListFilter struct {
	Limit     int
	SessionID string
	Status    string
	Path      string
}

type StoreState struct {
	Counter uint64 `json:"counter"`
	Order   []string
	Runs    []Run `json:"runs"`
}

type Store struct {
	mu       sync.RWMutex
	runs     map[string]Run
	order    []string
	counter  uint64
	onChange func()
}

func NewStore() *Store {
	return &Store{
		runs:  map[string]Run{},
		order: []string{},
	}
}

func (s *Store) Create(in CreateInput) (Run, error) {
	s.mu.Lock()
	run, err := s.createLocked(in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return run, err
}

func (s *Store) createLocked(in CreateInput) (Run, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.nextIDLocked()
	}
	if _, exists := s.runs[id]; exists {
		return Run{}, fmt.Errorf("run %q already exists", id)
	}
	path := strings.TrimSpace(in.Path)
	if path == "" {
		return Run{}, fmt.Errorf("run path is required")
	}
	now := time.Now().UTC()
	run := Run{
		ID:             id,
		Type:           "run",
		SessionID:      strings.TrimSpace(in.SessionID),
		Path:           path,
		Mode:           strings.TrimSpace(in.Mode),
		ClientModel:    strings.TrimSpace(in.ClientModel),
		RequestedModel: strings.TrimSpace(in.RequestedModel),
		UpstreamModel:  strings.TrimSpace(in.UpstreamModel),
		Stream:         in.Stream,
		ToolCount:      maxInt(0, in.ToolCount),
		Status:         StatusRunning,
		Metadata:       copyMetadata(in.Metadata),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.runs[id] = run
	s.order = append(s.order, id)
	return cloneRun(run), nil
}

func (s *Store) Complete(id string, in CompleteInput) (Run, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Run{}, fmt.Errorf("run id is required")
	}
	s.mu.Lock()
	run, err := s.completeLocked(id, in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return run, err
}

func (s *Store) completeLocked(id string, in CompleteInput) (Run, error) {
	run, ok := s.runs[id]
	if !ok {
		return Run{}, fmt.Errorf("run %q not found", id)
	}
	if run.Status != StatusRunning {
		return cloneRun(run), nil
	}

	now := time.Now().UTC()
	run.StatusCode = in.StatusCode
	run.Error = strings.TrimSpace(in.Error)
	run.UpdatedAt = now
	run.CompletedAt = &now
	if in.StatusCode >= 400 {
		run.Status = StatusFailed
	} else {
		run.Status = StatusCompleted
	}
	s.runs[id] = run
	return cloneRun(run), nil
}

func (s *Store) Get(id string) (Run, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Run{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	if !ok {
		return Run{}, false
	}
	return cloneRun(run), true
}

func (s *Store) List(filter ListFilter) []Run {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	sessionID := strings.TrimSpace(filter.SessionID)
	status := strings.TrimSpace(strings.ToLower(filter.Status))
	path := strings.TrimSpace(filter.Path)

	out := make([]Run, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.order[i]
		run, ok := s.runs[id]
		if !ok {
			continue
		}
		if sessionID != "" && run.SessionID != sessionID {
			continue
		}
		if status != "" && string(run.Status) != status {
			continue
		}
		if path != "" && run.Path != path {
			continue
		}
		out = append(out, cloneRun(run))
	}
	return out
}

func (s *Store) Snapshot() StoreState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := StoreState{
		Counter: s.counter,
		Order:   append([]string(nil), s.order...),
		Runs:    make([]Run, 0, len(s.order)),
	}
	for _, id := range s.order {
		if run, ok := s.runs[id]; ok {
			out.Runs = append(out.Runs, cloneRun(run))
		}
	}
	if len(out.Runs) == 0 && len(s.runs) > 0 {
		for _, run := range s.runs {
			out.Runs = append(out.Runs, cloneRun(run))
		}
	}
	return out
}

func (s *Store) Restore(state StoreState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]Run, len(state.Runs))
	for _, run := range state.Runs {
		id := strings.TrimSpace(run.ID)
		if id == "" {
			return fmt.Errorf("run id is required in restore state")
		}
		if _, exists := next[id]; exists {
			return fmt.Errorf("duplicate run id in restore state: %s", id)
		}
		next[id] = cloneRun(run)
	}

	order := normalizeOrder(state.Order, next)
	s.runs = next
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

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("run_%d_%x", time.Now().Unix(), n)
}

func cloneRun(in Run) Run {
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

func maxInt(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

func normalizeOrder(order []string, entries map[string]Run) []string {
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
