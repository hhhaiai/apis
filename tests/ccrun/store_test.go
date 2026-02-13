package ccrun_test

import (
	. "ccgateway/internal/ccrun"
	"testing"
)

func TestStoreCreateGetList(t *testing.T) {
	st := NewStore()
	first, err := st.Create(CreateInput{
		ID:        "run_a",
		SessionID: "sess_1",
		Path:      "/v1/messages",
		Mode:      "chat",
	})
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := st.Create(CreateInput{
		ID:        "run_b",
		SessionID: "sess_2",
		Path:      "/v1/chat/completions",
		Mode:      "plan",
		ToolCount: 2,
	})
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	if first.Status != StatusRunning {
		t.Fatalf("expected running status, got %q", first.Status)
	}
	if second.ToolCount != 2 {
		t.Fatalf("expected tool_count=2, got %d", second.ToolCount)
	}

	got, ok := st.Get(first.ID)
	if !ok {
		t.Fatalf("expected run found")
	}
	if got.Path != "/v1/messages" {
		t.Fatalf("unexpected path: %q", got.Path)
	}

	list := st.List(ListFilter{Limit: 10})
	if len(list) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(list))
	}
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("unexpected list order: %#v", []string{list[0].ID, list[1].ID})
	}
}

func TestStoreCompleteAndFilter(t *testing.T) {
	st := NewStore()
	a, err := st.Create(CreateInput{
		ID:        "run_ok",
		SessionID: "sess_1",
		Path:      "/v1/messages",
	})
	if err != nil {
		t.Fatalf("create run_ok: %v", err)
	}
	b, err := st.Create(CreateInput{
		ID:        "run_fail",
		SessionID: "sess_1",
		Path:      "/v1/messages",
	})
	if err != nil {
		t.Fatalf("create run_fail: %v", err)
	}
	if _, err := st.Complete(a.ID, CompleteInput{StatusCode: 200}); err != nil {
		t.Fatalf("complete run_ok: %v", err)
	}
	if _, err := st.Complete(b.ID, CompleteInput{StatusCode: 500, Error: "boom"}); err != nil {
		t.Fatalf("complete run_fail: %v", err)
	}

	okRuns := st.List(ListFilter{Status: "completed"})
	if len(okRuns) != 1 || okRuns[0].ID != a.ID {
		t.Fatalf("unexpected completed filter: %+v", okRuns)
	}
	failRuns := st.List(ListFilter{Status: "failed", SessionID: "sess_1", Path: "/v1/messages"})
	if len(failRuns) != 1 || failRuns[0].ID != b.ID {
		t.Fatalf("unexpected failed filter: %+v", failRuns)
	}
	if failRuns[0].CompletedAt == nil {
		t.Fatalf("expected completed_at set on failed run")
	}
}

func TestStoreValidation(t *testing.T) {
	st := NewStore()
	if _, err := st.Create(CreateInput{Path: ""}); err == nil {
		t.Fatalf("expected path required error")
	}
	if _, err := st.Create(CreateInput{ID: "run_dup", Path: "/v1/messages"}); err != nil {
		t.Fatalf("create dup seed: %v", err)
	}
	if _, err := st.Create(CreateInput{ID: "run_dup", Path: "/v1/messages"}); err == nil {
		t.Fatalf("expected duplicate id error")
	}
	if _, err := st.Complete("missing", CompleteInput{StatusCode: 200}); err == nil {
		t.Fatalf("expected missing run error")
	}
}

func TestStoreSnapshotRestoreAndOnChange(t *testing.T) {
	st := NewStore()
	changeCount := 0
	st.SetOnChange(func() {
		changeCount++
	})

	created, err := st.Create(CreateInput{
		ID:        "run_snap",
		SessionID: "sess_s",
		Path:      "/v1/messages",
	})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := st.Complete(created.ID, CompleteInput{StatusCode: 200}); err != nil {
		t.Fatalf("complete run: %v", err)
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
	if len(list) != 1 || list[0].ID != "run_snap" || list[0].Status != StatusCompleted {
		t.Fatalf("unexpected restored runs: %+v", list)
	}
}
