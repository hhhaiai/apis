package todo_test

import (
	. "ccgateway/internal/todo"
	"testing"
)

func TestStoreCreateGetList(t *testing.T) {
	st := NewStore()
	first, err := st.Create(CreateInput{
		ID:        "todo_a",
		Title:     "first",
		SessionID: "sess_1",
		RunID:     "run_1",
		PlanID:    "plan_1",
		Metadata: map[string]any{
			"owner": "alpha",
		},
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := st.Create(CreateInput{
		ID:     "todo_b",
		Title:  "second",
		Status: "in_progress",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	if first.Type != "todo" {
		t.Fatalf("expected type=todo, got %q", first.Type)
	}
	if second.Status != StatusInProgress {
		t.Fatalf("expected status in_progress, got %q", second.Status)
	}
	if first.PlanID != "plan_1" {
		t.Fatalf("expected plan_id=plan_1, got %q", first.PlanID)
	}

	got, ok := st.Get("todo_a")
	if !ok {
		t.Fatalf("expected todo found")
	}
	got.Metadata["owner"] = "mutated"
	gotAgain, ok := st.Get("todo_a")
	if !ok {
		t.Fatalf("expected todo found on second get")
	}
	if gotAgain.Metadata["owner"] != "alpha" {
		t.Fatalf("expected metadata clone isolation, got %#v", gotAgain.Metadata["owner"])
	}

	list := st.List(ListFilter{Limit: 10})
	if len(list) != 2 {
		t.Fatalf("expected 2 todos, got %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("expected reverse chronological list order, got %#v", []string{list[0].ID, list[1].ID})
	}
}

func TestStoreUpdateLifecycle(t *testing.T) {
	st := NewStore()
	td, err := st.Create(CreateInput{
		ID:     "todo_flow",
		Title:  "flow",
		Status: "pending",
	})
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}
	if td.CompletedAt != nil {
		t.Fatalf("expected nil completed_at on pending")
	}

	inProgress := "in_progress"
	updated, err := st.Update(td.ID, UpdateInput{
		Status: &inProgress,
	})
	if err != nil {
		t.Fatalf("update in_progress: %v", err)
	}
	if updated.Status != StatusInProgress {
		t.Fatalf("expected in_progress, got %q", updated.Status)
	}
	if updated.CompletedAt != nil {
		t.Fatalf("expected nil completed_at on in_progress")
	}

	completed := "completed"
	updated, err = st.Update(td.ID, UpdateInput{
		Status: &completed,
	})
	if err != nil {
		t.Fatalf("update completed: %v", err)
	}
	if updated.Status != StatusCompleted {
		t.Fatalf("expected completed, got %q", updated.Status)
	}
	if updated.CompletedAt == nil {
		t.Fatalf("expected completed_at set")
	}
}

func TestStoreFiltersAndValidation(t *testing.T) {
	st := NewStore()
	if _, err := st.Create(CreateInput{Title: "a", SessionID: "sess_1", PlanID: "plan_1", Status: "pending"}); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if _, err := st.Create(CreateInput{Title: "b", SessionID: "sess_2", PlanID: "plan_2", Status: "in_progress"}); err != nil {
		t.Fatalf("create b: %v", err)
	}
	if _, err := st.Create(CreateInput{Title: "c", SessionID: "sess_1", RunID: "run_2", PlanID: "plan_1", Status: "blocked"}); err != nil {
		t.Fatalf("create c: %v", err)
	}

	bySession := st.List(ListFilter{SessionID: "sess_1"})
	if len(bySession) != 2 {
		t.Fatalf("expected 2 by session, got %d", len(bySession))
	}
	byStatus := st.List(ListFilter{Status: "in_progress"})
	if len(byStatus) != 1 || byStatus[0].Status != StatusInProgress {
		t.Fatalf("unexpected status filter result: %+v", byStatus)
	}
	byRun := st.List(ListFilter{RunID: "run_2"})
	if len(byRun) != 1 || byRun[0].RunID != "run_2" {
		t.Fatalf("unexpected run filter result: %+v", byRun)
	}
	byPlan := st.List(ListFilter{PlanID: "plan_1"})
	if len(byPlan) != 2 {
		t.Fatalf("expected 2 by plan, got %d", len(byPlan))
	}

	if _, err := st.Create(CreateInput{Title: "", Status: "pending"}); err == nil {
		t.Fatalf("expected empty title error")
	}
	if _, err := st.Create(CreateInput{Title: "bad", Status: "unknown"}); err == nil {
		t.Fatalf("expected invalid status error")
	}
}

func TestStoreSnapshotRestoreAndOnChange(t *testing.T) {
	st := NewStore()
	changeCount := 0
	st.SetOnChange(func() {
		changeCount++
	})

	td, err := st.Create(CreateInput{
		ID:        "todo_snap",
		Title:     "snapshot",
		SessionID: "sess_s",
		PlanID:    "plan_s",
		Status:    "pending",
	})
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}
	completed := "completed"
	if _, err := st.Update(td.ID, UpdateInput{Status: &completed}); err != nil {
		t.Fatalf("update todo: %v", err)
	}
	if changeCount < 2 {
		t.Fatalf("expected onChange invoked at least twice, got %d", changeCount)
	}

	snapshot := st.Snapshot()
	restored := NewStore()
	if err := restored.Restore(snapshot); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	list := restored.List(ListFilter{})
	if len(list) != 1 || list[0].ID != "todo_snap" || list[0].Status != StatusCompleted {
		t.Fatalf("unexpected restored todos: %+v", list)
	}
}
