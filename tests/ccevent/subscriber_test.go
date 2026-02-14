package ccevent_test

import (
	. "ccgateway/internal/ccevent"
	"testing"
	"time"
)

func TestSubscriberRegistry_BasicNotify(t *testing.T) {
	reg := NewSubscriberRegistry()
	ch, cancel := reg.Subscribe(ListFilter{})
	defer cancel()

	ev := Event{
		ID:        "evt_1",
		EventType: "test.event",
		SessionID: "sess_1",
	}
	reg.Notify(ev)

	select {
	case got := <-ch:
		if got.ID != "evt_1" {
			t.Fatalf("expected evt_1, got %s", got.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for event")
	}
}

func TestSubscriberRegistry_FilterBySessionID(t *testing.T) {
	reg := NewSubscriberRegistry()
	ch, cancel := reg.Subscribe(ListFilter{SessionID: "sess_a"})
	defer cancel()

	reg.Notify(Event{ID: "1", SessionID: "sess_b"})
	reg.Notify(Event{ID: "2", SessionID: "sess_a"})

	select {
	case got := <-ch:
		if got.ID != "2" {
			t.Fatalf("expected event 2 (sess_a), got %s", got.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out")
	}

	// Ensure event 1 was filtered out
	select {
	case ev := <-ch:
		t.Fatalf("unexpected event: %s", ev.ID)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestSubscriberRegistry_Cancel(t *testing.T) {
	reg := NewSubscriberRegistry()
	ch, cancel := reg.Subscribe(ListFilter{})
	cancel()

	reg.Notify(Event{ID: "1"})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("should not receive after cancel")
		}
	case <-time.After(50 * time.Millisecond):
		// expected — channel not closed, just unsubscribed
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name   string
		event  Event
		filter ListFilter
		want   bool
	}{
		{"empty filter matches all", Event{EventType: "foo"}, ListFilter{}, true},
		{"event type match", Event{EventType: "run.completed"}, ListFilter{EventType: "run.completed"}, true},
		{"event type mismatch", Event{EventType: "run.completed"}, ListFilter{EventType: "run.failed"}, false},
		{"session match", Event{SessionID: "s1"}, ListFilter{SessionID: "s1"}, true},
		{"session mismatch", Event{SessionID: "s1"}, ListFilter{SessionID: "s2"}, false},
		{"multi filter match", Event{SessionID: "s1", RunID: "r1"}, ListFilter{SessionID: "s1", RunID: "r1"}, true},
		{"multi filter partial mismatch", Event{SessionID: "s1", RunID: "r1"}, ListFilter{SessionID: "s1", RunID: "r2"}, false},
		{"team match", Event{TeamID: "team_1"}, ListFilter{TeamID: "team_1"}, true},
		{"team mismatch", Event{TeamID: "team_1"}, ListFilter{TeamID: "team_2"}, false},
		{"subagent match", Event{SubagentID: "sub_1"}, ListFilter{SubagentID: "sub_1"}, true},
		{"subagent mismatch", Event{SubagentID: "sub_1"}, ListFilter{SubagentID: "sub_2"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesFilter(tt.event, tt.filter)
			if got != tt.want {
				t.Errorf("matchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}
