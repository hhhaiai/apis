package ccevent_test

import (
	. "ccgateway/internal/ccevent"
	"testing"
)

func TestStoreAppendListFilter(t *testing.T) {
	st := NewStore()
	first, err := st.Append(AppendInput{
		EventType: "run.created",
		SessionID: "sess_1",
		RunID:     "run_1",
		Data: map[string]any{
			"path": "/v1/messages",
		},
	})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	second, err := st.Append(AppendInput{
		EventType: "todo.updated",
		SessionID: "sess_1",
		TodoID:    "todo_1",
	})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if first.Type != "event" {
		t.Fatalf("expected type=event, got %q", first.Type)
	}
	if second.ID == "" {
		t.Fatalf("expected non-empty event id")
	}

	all := st.List(ListFilter{Limit: 10})
	if len(all) != 2 {
		t.Fatalf("expected 2 events, got %d", len(all))
	}
	if all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatalf("unexpected event order: %#v", []string{all[0].ID, all[1].ID})
	}

	bySession := st.List(ListFilter{SessionID: "sess_1"})
	if len(bySession) != 2 {
		t.Fatalf("expected 2 events by session, got %d", len(bySession))
	}
	byRun := st.List(ListFilter{RunID: "run_1"})
	if len(byRun) != 1 || byRun[0].RunID != "run_1" {
		t.Fatalf("unexpected run filter: %+v", byRun)
	}
	byType := st.List(ListFilter{EventType: "todo.updated"})
	if len(byType) != 1 || byType[0].EventType != "todo.updated" {
		t.Fatalf("unexpected event_type filter: %+v", byType)
	}
}

func TestStoreAppendValidation(t *testing.T) {
	st := NewStore()
	if _, err := st.Append(AppendInput{}); err == nil {
		t.Fatalf("expected event_type validation error")
	}
}
