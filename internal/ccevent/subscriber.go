package ccevent

import "sync"

// Subscriber receives events matching a filter.
type Subscriber struct {
	ch     chan Event
	filter ListFilter
}

// SubscriberRegistry manages SSE event subscribers.
type SubscriberRegistry struct {
	mu   sync.RWMutex
	subs map[*Subscriber]struct{}
}

// NewSubscriberRegistry creates a new subscriber registry.
func NewSubscriberRegistry() *SubscriberRegistry {
	return &SubscriberRegistry{
		subs: make(map[*Subscriber]struct{}),
	}
}

// Subscribe creates a new subscriber with the given filter.
// Returns the event channel and a cancel function.
func (r *SubscriberRegistry) Subscribe(filter ListFilter) (<-chan Event, func()) {
	ch := make(chan Event, 64)
	sub := &Subscriber{ch: ch, filter: filter}

	r.mu.Lock()
	r.subs[sub] = struct{}{}
	r.mu.Unlock()

	cancel := func() {
		r.mu.Lock()
		delete(r.subs, sub)
		r.mu.Unlock()
		// Drain remaining events
		for {
			select {
			case <-ch:
			default:
				return
			}
		}
	}
	return ch, cancel
}

// Notify sends an event to all matching subscribers.
func (r *SubscriberRegistry) Notify(e Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for sub := range r.subs {
		if matchesFilter(e, sub.filter) {
			select {
			case sub.ch <- e:
			default:
				// Drop if subscriber is slow — avoid blocking
			}
		}
	}
}

// matchesFilter checks if an event matches a subscriber's filter.
func matchesFilter(e Event, f ListFilter) bool {
	if f.EventType != "" && e.EventType != f.EventType {
		return false
	}
	if f.SessionID != "" && e.SessionID != f.SessionID {
		return false
	}
	if f.RunID != "" && e.RunID != f.RunID {
		return false
	}
	if f.PlanID != "" && e.PlanID != f.PlanID {
		return false
	}
	if f.TodoID != "" && e.TodoID != f.TodoID {
		return false
	}
	return true
}
