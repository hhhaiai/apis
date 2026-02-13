package plan

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusApproved  Status = "approved"
	StatusExecuting Status = "executing"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

type Step struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type Plan struct {
	ID          string         `json:"id"`
	Type        string         `json:"type"`
	SessionID   string         `json:"session_id,omitempty"`
	RunID       string         `json:"run_id,omitempty"`
	Title       string         `json:"title"`
	Summary     string         `json:"summary,omitempty"`
	Steps       []Step         `json:"steps,omitempty"`
	Status      Status         `json:"status"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ApprovedAt  *time.Time     `json:"approved_at,omitempty"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

type CreateInput struct {
	ID        string         `json:"id,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary,omitempty"`
	Steps     []Step         `json:"steps,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type ApproveInput struct {
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ExecuteInput struct {
	Complete bool           `json:"complete,omitempty"`
	Failed   bool           `json:"failed,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type ListFilter struct {
	Limit     int
	Status    string
	SessionID string
	RunID     string
}

type StoreState struct {
	Counter uint64 `json:"counter"`
	Order   []string
	Plans   []Plan `json:"plans"`
}

type Store struct {
	mu          sync.RWMutex
	plans       map[string]Plan
	order       []string
	counter     uint64
	onChange    func()
	checkpoints map[string][]Checkpoint
}

func NewStore() *Store {
	return &Store{
		plans: map[string]Plan{},
		order: []string{},
	}
}

func (s *Store) Create(in CreateInput) (Plan, error) {
	s.mu.Lock()
	out, err := s.createLocked(in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return out, err
}

func (s *Store) createLocked(in CreateInput) (Plan, error) {
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.nextIDLocked()
	}
	if _, exists := s.plans[id]; exists {
		return Plan{}, fmt.Errorf("plan %q already exists", id)
	}

	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Plan{}, fmt.Errorf("plan title is required")
	}

	now := time.Now().UTC()
	p := Plan{
		ID:        id,
		Type:      "plan",
		SessionID: strings.TrimSpace(in.SessionID),
		RunID:     strings.TrimSpace(in.RunID),
		Title:     title,
		Summary:   strings.TrimSpace(in.Summary),
		Steps:     cloneSteps(in.Steps),
		Status:    StatusDraft,
		Metadata:  copyMetadata(in.Metadata),
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.plans[id] = p
	s.order = append(s.order, id)
	return clonePlan(p), nil
}

func (s *Store) Get(id string) (Plan, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Plan{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.plans[id]
	if !ok {
		return Plan{}, false
	}
	return clonePlan(p), true
}

func (s *Store) List(filter ListFilter) []Plan {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	status := strings.TrimSpace(strings.ToLower(filter.Status))
	sessionID := strings.TrimSpace(filter.SessionID)
	runID := strings.TrimSpace(filter.RunID)

	out := make([]Plan, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.order[i]
		p, ok := s.plans[id]
		if !ok {
			continue
		}
		if status != "" && string(p.Status) != status {
			continue
		}
		if sessionID != "" && p.SessionID != sessionID {
			continue
		}
		if runID != "" && p.RunID != runID {
			continue
		}
		out = append(out, clonePlan(p))
	}
	return out
}

func (s *Store) Approve(id string, in ApproveInput) (Plan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Plan{}, fmt.Errorf("plan id is required")
	}

	s.mu.Lock()
	out, err := s.approveLocked(id, in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return out, err
}

func (s *Store) approveLocked(id string, in ApproveInput) (Plan, error) {
	p, ok := s.plans[id]
	if !ok {
		return Plan{}, fmt.Errorf("plan %q not found", id)
	}
	if p.Status != StatusDraft {
		return Plan{}, fmt.Errorf("plan %q cannot be approved from status %q", id, p.Status)
	}

	now := time.Now().UTC()
	p.Status = StatusApproved
	p.ApprovedAt = &now
	if len(in.Metadata) > 0 {
		p.Metadata = copyMetadata(in.Metadata)
	}
	p.UpdatedAt = now
	s.plans[id] = p
	return clonePlan(p), nil
}

func (s *Store) Execute(id string, in ExecuteInput) (Plan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Plan{}, fmt.Errorf("plan id is required")
	}
	if in.Complete && in.Failed {
		return Plan{}, fmt.Errorf("execute input cannot set both complete and failed")
	}

	s.mu.Lock()
	out, err := s.executeLocked(id, in)
	s.mu.Unlock()
	if err == nil {
		s.notifyChanged()
	}
	return out, err
}

func (s *Store) executeLocked(id string, in ExecuteInput) (Plan, error) {
	p, ok := s.plans[id]
	if !ok {
		return Plan{}, fmt.Errorf("plan %q not found", id)
	}
	if p.Status == StatusDraft {
		return Plan{}, fmt.Errorf("plan %q must be approved before execute", id)
	}
	if p.Status == StatusCanceled || p.Status == StatusCompleted || p.Status == StatusFailed {
		return Plan{}, fmt.Errorf("plan %q is already in terminal status %q", id, p.Status)
	}

	now := time.Now().UTC()
	if p.StartedAt == nil {
		p.StartedAt = &now
	}
	switch {
	case in.Complete:
		p.Status = StatusCompleted
		p.CompletedAt = &now
	case in.Failed:
		p.Status = StatusFailed
		p.CompletedAt = &now
	default:
		p.Status = StatusExecuting
	}
	if len(in.Metadata) > 0 {
		p.Metadata = copyMetadata(in.Metadata)
	}
	p.UpdatedAt = now
	s.plans[id] = p
	return clonePlan(p), nil
}

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("plan_%d_%x", time.Now().Unix(), n)
}

func (s *Store) Snapshot() StoreState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := StoreState{
		Counter: s.counter,
		Order:   append([]string(nil), s.order...),
		Plans:   make([]Plan, 0, len(s.order)),
	}
	for _, id := range s.order {
		if p, ok := s.plans[id]; ok {
			out.Plans = append(out.Plans, clonePlan(p))
		}
	}
	if len(out.Plans) == 0 && len(s.plans) > 0 {
		for _, p := range s.plans {
			out.Plans = append(out.Plans, clonePlan(p))
		}
	}
	return out
}

func (s *Store) Restore(state StoreState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := make(map[string]Plan, len(state.Plans))
	for _, p := range state.Plans {
		id := strings.TrimSpace(p.ID)
		if id == "" {
			return fmt.Errorf("plan id is required in restore state")
		}
		if _, exists := next[id]; exists {
			return fmt.Errorf("duplicate plan id in restore state: %s", id)
		}
		next[id] = clonePlan(p)
	}
	order := normalizeOrder(state.Order, next)
	s.plans = next
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

func clonePlan(in Plan) Plan {
	out := in
	out.Metadata = copyMetadata(in.Metadata)
	out.Steps = cloneSteps(in.Steps)
	if in.ApprovedAt != nil {
		t := *in.ApprovedAt
		out.ApprovedAt = &t
	}
	if in.StartedAt != nil {
		t := *in.StartedAt
		out.StartedAt = &t
	}
	if in.CompletedAt != nil {
		t := *in.CompletedAt
		out.CompletedAt = &t
	}
	return out
}

func cloneSteps(in []Step) []Step {
	if len(in) == 0 {
		return []Step{}
	}
	out := make([]Step, 0, len(in))
	for _, s := range in {
		title := strings.TrimSpace(s.Title)
		if title == "" {
			continue
		}
		out = append(out, Step{
			Title:       title,
			Description: strings.TrimSpace(s.Description),
		})
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

func normalizeOrder(order []string, entries map[string]Plan) []string {
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
