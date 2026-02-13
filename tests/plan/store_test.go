package plan_test

import (
	. "ccgateway/internal/plan"
	"testing"
)

func TestStoreCreateGetList(t *testing.T) {
	st := NewStore()
	first, err := st.Create(CreateInput{
		ID:        "plan_a",
		Title:     "first",
		SessionID: "sess_1",
		Steps: []Step{
			{Title: "analyze"},
			{Title: "implement"},
		},
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := st.Create(CreateInput{
		ID:      "plan_b",
		Title:   "second",
		Summary: "short",
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	if first.Status != StatusDraft {
		t.Fatalf("expected draft status, got %q", first.Status)
	}
	if len(first.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(first.Steps))
	}

	got, ok := st.Get(first.ID)
	if !ok {
		t.Fatalf("expected plan found")
	}
	if got.Title != first.Title {
		t.Fatalf("unexpected title: %q", got.Title)
	}

	list := st.List(ListFilter{Limit: 10})
	if len(list) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("unexpected list order: %#v", []string{list[0].ID, list[1].ID})
	}
}

func TestStoreApproveAndExecute(t *testing.T) {
	st := NewStore()
	p, err := st.Create(CreateInput{
		ID:    "plan_flow",
		Title: "flow",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	approved, err := st.Approve(p.ID, ApproveInput{})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != StatusApproved {
		t.Fatalf("expected approved, got %q", approved.Status)
	}
	if approved.ApprovedAt == nil {
		t.Fatalf("expected approved_at set")
	}

	executing, err := st.Execute(p.ID, ExecuteInput{})
	if err != nil {
		t.Fatalf("execute start: %v", err)
	}
	if executing.Status != StatusExecuting {
		t.Fatalf("expected executing, got %q", executing.Status)
	}
	if executing.StartedAt == nil {
		t.Fatalf("expected started_at set")
	}

	completed, err := st.Execute(p.ID, ExecuteInput{Complete: true})
	if err != nil {
		t.Fatalf("execute complete: %v", err)
	}
	if completed.Status != StatusCompleted {
		t.Fatalf("expected completed, got %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatalf("expected completed_at set")
	}
}

func TestStoreValidation(t *testing.T) {
	st := NewStore()
	if _, err := st.Create(CreateInput{Title: ""}); err == nil {
		t.Fatalf("expected create validation error")
	}
	if _, err := st.Create(CreateInput{ID: "plan_dup", Title: "x"}); err != nil {
		t.Fatalf("create dup seed: %v", err)
	}
	if _, err := st.Create(CreateInput{ID: "plan_dup", Title: "x2"}); err == nil {
		t.Fatalf("expected duplicate id error")
	}

	draft, err := st.Create(CreateInput{ID: "plan_draft", Title: "draft"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if _, err := st.Execute(draft.ID, ExecuteInput{}); err == nil {
		t.Fatalf("expected execute-before-approve error")
	}
	if _, err := st.Execute(draft.ID, ExecuteInput{Complete: true, Failed: true}); err == nil {
		t.Fatalf("expected complete+failed conflict error")
	}
}

func TestStoreSnapshotRestoreAndOnChange(t *testing.T) {
	st := NewStore()
	changeCount := 0
	st.SetOnChange(func() {
		changeCount++
	})

	p, err := st.Create(CreateInput{
		ID:    "plan_snap",
		Title: "snapshot",
		Steps: []Step{{Title: "one"}},
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := st.Approve(p.ID, ApproveInput{}); err != nil {
		t.Fatalf("approve plan: %v", err)
	}
	if _, err := st.Execute(p.ID, ExecuteInput{Complete: true}); err != nil {
		t.Fatalf("execute plan complete: %v", err)
	}
	if changeCount < 3 {
		t.Fatalf("expected onChange invoked at least three times, got %d", changeCount)
	}

	snapshot := st.Snapshot()
	restored := NewStore()
	if err := restored.Restore(snapshot); err != nil {
		t.Fatalf("restore snapshot: %v", err)
	}
	list := restored.List(ListFilter{})
	if len(list) != 1 || list[0].ID != "plan_snap" || list[0].Status != StatusCompleted {
		t.Fatalf("unexpected restored plans: %+v", list)
	}
}
