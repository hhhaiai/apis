package ccevent

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	EventType  string         `json:"event_type"`
	SessionID  string         `json:"session_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	PlanID     string         `json:"plan_id,omitempty"`
	TodoID     string         `json:"todo_id,omitempty"`
	TeamID     string         `json:"team_id,omitempty"`
	SubagentID string         `json:"subagent_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

type AppendInput struct {
	EventType  string         `json:"event_type"`
	SessionID  string         `json:"session_id,omitempty"`
	RunID      string         `json:"run_id,omitempty"`
	PlanID     string         `json:"plan_id,omitempty"`
	TodoID     string         `json:"todo_id,omitempty"`
	TeamID     string         `json:"team_id,omitempty"`
	SubagentID string         `json:"subagent_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
}

type ListFilter struct {
	Limit      int
	EventType  string
	SessionID  string
	RunID      string
	PlanID     string
	TodoID     string
	TeamID     string
	SubagentID string
}

type Store struct {
	mu      sync.RWMutex
	events  []Event
	counter uint64
	subs    *SubscriberRegistry
}

func NewStore() *Store {
	return &Store{
		events: []Event{},
		subs:   NewSubscriberRegistry(),
	}
}

// Subscribe creates a filtered subscription for real-time events.
func (s *Store) Subscribe(filter ListFilter) (<-chan Event, func()) {
	return s.subs.Subscribe(filter)
}

func (s *Store) Append(in AppendInput) (Event, error) {
	eventType := strings.TrimSpace(in.EventType)
	if eventType == "" {
		return Event{}, fmt.Errorf("event_type is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e := Event{
		ID:         s.nextIDLocked(),
		Type:       "event",
		EventType:  eventType,
		SessionID:  strings.TrimSpace(in.SessionID),
		RunID:      strings.TrimSpace(in.RunID),
		PlanID:     strings.TrimSpace(in.PlanID),
		TodoID:     strings.TrimSpace(in.TodoID),
		TeamID:     strings.TrimSpace(in.TeamID),
		SubagentID: strings.TrimSpace(in.SubagentID),
		Data:       copyMap(in.Data),
		CreatedAt:  time.Now().UTC(),
	}
	if e.TeamID == "" {
		e.TeamID = strings.TrimSpace(valueAsString(in.Data["team_id"]))
	}
	s.events = append(s.events, e)
	// Notify SSE subscribers outside the lock
	cloned := cloneEvent(e)
	go s.subs.Notify(cloned)
	return cloned, nil
}

func (s *Store) List(filter ListFilter) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	eventType := strings.TrimSpace(filter.EventType)
	sessionID := strings.TrimSpace(filter.SessionID)
	runID := strings.TrimSpace(filter.RunID)
	planID := strings.TrimSpace(filter.PlanID)
	todoID := strings.TrimSpace(filter.TodoID)
	teamID := strings.TrimSpace(filter.TeamID)
	subagentID := strings.TrimSpace(filter.SubagentID)

	out := make([]Event, 0, limit)
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.events[i]
		if eventType != "" && e.EventType != eventType {
			continue
		}
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		if runID != "" && e.RunID != runID {
			continue
		}
		if planID != "" && e.PlanID != planID {
			continue
		}
		if todoID != "" && e.TodoID != todoID {
			continue
		}
		if teamID != "" && e.TeamID != teamID {
			continue
		}
		if subagentID != "" && e.SubagentID != subagentID {
			continue
		}
		out = append(out, cloneEvent(e))
	}
	return out
}

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("evt_%d_%x", time.Now().Unix(), n)
}

func cloneEvent(in Event) Event {
	out := in
	out.Data = copyMap(in.Data)
	return out
}

func copyMap(in map[string]any) map[string]any {
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

func valueAsString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
