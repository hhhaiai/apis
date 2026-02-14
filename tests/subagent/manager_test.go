package subagent_test

import (
	. "ccgateway/internal/subagent"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManager_SpawnAndGet(t *testing.T) {
	m := NewManager(nil)
	agent, err := m.Spawn(context.Background(), SpawnConfig{
		Model: "test-model",
		Task:  "analyze code",
	})
	if err != nil {
		t.Fatal(err)
	}
	if agent.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if agent.Status != "pending" {
		t.Fatalf("expected pending status, got %s", agent.Status)
	}

	// Wait for async task to complete
	time.Sleep(100 * time.Millisecond)

	got, ok := m.Get(agent.ID)
	if !ok {
		t.Fatal("agent not found")
	}
	if got.Status != "completed" {
		t.Fatalf("expected completed, got %s", got.Status)
	}
}

func TestManager_List(t *testing.T) {
	m := NewManager(nil)
	_, _ = m.Spawn(context.Background(), SpawnConfig{ParentID: "p1", Task: "task1"})
	_, _ = m.Spawn(context.Background(), SpawnConfig{ParentID: "p2", Task: "task2"})

	all := m.List("")
	if len(all) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(all))
	}

	filtered := m.List("p1")
	if len(filtered) != 1 {
		t.Fatalf("expected 1 agent for p1, got %d", len(filtered))
	}
}

func TestManager_Terminate(t *testing.T) {
	m := NewManager(func(ctx context.Context, a Agent) (string, error) {
		// Simulate long task
		time.Sleep(1 * time.Second)
		return "done", nil
	})

	agent, _ := m.Spawn(context.Background(), SpawnConfig{Task: "long task"})
	if err := m.Terminate(agent.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Get(agent.ID)
	if got.Status != "terminated" {
		t.Fatalf("expected terminated, got %s", got.Status)
	}
}

func TestManager_EmptyTask(t *testing.T) {
	m := NewManager(nil)
	_, err := m.Spawn(context.Background(), SpawnConfig{Task: ""})
	if err == nil {
		t.Fatal("expected error for empty task")
	}
}

func TestManager_DefaultModel(t *testing.T) {
	m := NewManager(nil)
	agent, _ := m.Spawn(context.Background(), SpawnConfig{Task: "test"})
	if agent.Model != "default" {
		t.Fatalf("expected default model, got %s", agent.Model)
	}
}

func TestManager_TerminateNotFound(t *testing.T) {
	m := NewManager(nil)
	err := m.Terminate("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestManager_TerminateKeepsTerminalStatus(t *testing.T) {
	m := NewManager(func(_ context.Context, _ Agent) (string, error) {
		time.Sleep(80 * time.Millisecond)
		return "done", nil
	})
	agent, err := m.Spawn(context.Background(), SpawnConfig{Task: "long running"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if err := m.Terminate(agent.ID); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	time.Sleep(120 * time.Millisecond)
	got, ok := m.Get(agent.ID)
	if !ok {
		t.Fatalf("agent not found")
	}
	if got.Status != "terminated" {
		t.Fatalf("expected terminated status preserved, got %s", got.Status)
	}
}

func TestManager_TerminateWithMeta(t *testing.T) {
	m := NewManager(nil)
	created, err := m.Spawn(context.Background(), SpawnConfig{Task: "x"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	updated, err := m.TerminateWithMeta(created.ID, "lead", "manual stop")
	if err != nil {
		t.Fatalf("terminate with meta: %v", err)
	}
	if updated.Status != "terminated" {
		t.Fatalf("expected terminated status, got %q", updated.Status)
	}
	if updated.TerminatedBy != "lead" {
		t.Fatalf("expected terminated_by lead, got %q", updated.TerminatedBy)
	}
	if updated.TerminationReason != "manual stop" {
		t.Fatalf("expected termination reason, got %q", updated.TerminationReason)
	}
	if updated.TerminatedAt == nil {
		t.Fatalf("expected terminated_at set")
	}
}

func TestManager_DeleteKeepsDeletedStatus(t *testing.T) {
	m := NewManager(func(_ context.Context, _ Agent) (string, error) {
		time.Sleep(60 * time.Millisecond)
		return "done", nil
	})
	created, err := m.Spawn(context.Background(), SpawnConfig{Task: "long"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	deleted, err := m.Delete(created.ID, "admin", "cleanup")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted.Status != "deleted" {
		t.Fatalf("expected deleted status, got %q", deleted.Status)
	}
	if deleted.DeletedBy != "admin" || deleted.DeletionReason != "cleanup" {
		t.Fatalf("unexpected delete audit fields: %+v", deleted)
	}
	time.Sleep(120 * time.Millisecond)
	got, ok := m.Get(created.ID)
	if !ok {
		t.Fatalf("agent not found")
	}
	if got.Status != "deleted" {
		t.Fatalf("expected deleted status preserved, got %q", got.Status)
	}
}

func TestManager_WaitCompleted(t *testing.T) {
	m := NewManager(func(_ context.Context, _ Agent) (string, error) {
		time.Sleep(20 * time.Millisecond)
		return "done", nil
	})
	agent, err := m.Spawn(context.Background(), SpawnConfig{Task: "wait-success"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	done, err := m.Wait(context.Background(), agent.ID, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if done.Status != "completed" {
		t.Fatalf("expected completed, got %s", done.Status)
	}
	if done.Result != "done" {
		t.Fatalf("unexpected result: %q", done.Result)
	}
}

func TestManager_WaitFailed(t *testing.T) {
	m := NewManager(func(_ context.Context, _ Agent) (string, error) {
		return "", context.Canceled
	})
	agent, err := m.Spawn(context.Background(), SpawnConfig{Task: "wait-failed"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	_, err = m.Wait(context.Background(), agent.ID, 5*time.Millisecond)
	if err == nil {
		t.Fatalf("expected wait error")
	}
	if !strings.Contains(err.Error(), "canceled") {
		t.Fatalf("unexpected wait error: %v", err)
	}
}

func TestManager_WaitContextCanceled(t *testing.T) {
	m := NewManager(func(_ context.Context, _ Agent) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "late", nil
	})
	agent, err := m.Spawn(context.Background(), SpawnConfig{Task: "wait-timeout"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = m.Wait(ctx, agent.ID, 10*time.Millisecond)
	if err == nil {
		t.Fatalf("expected context deadline exceeded")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		t.Fatalf("unexpected wait error: %v", err)
	}
}

func TestManager_WaitDeleted(t *testing.T) {
	m := NewManager(func(_ context.Context, _ Agent) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return "late", nil
	})
	created, err := m.Spawn(context.Background(), SpawnConfig{Task: "delete-then-wait"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := m.Delete(created.ID, "admin", "cleanup"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = m.Wait(context.Background(), created.ID, 5*time.Millisecond)
	if err == nil {
		t.Fatalf("expected deleted wait error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "deleted") {
		t.Fatalf("unexpected wait deleted error: %v", err)
	}
}

func TestManager_LifecycleHookRecords(t *testing.T) {
	var (
		mu     sync.Mutex
		events []LifecycleEvent
	)
	manager := NewManagerWithLifecycle(func(_ context.Context, _ Agent) (string, error) {
		return "lifecycle-ok", nil
	}, func(event LifecycleEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	created, err := manager.Spawn(context.Background(), SpawnConfig{
		ParentID: "team_lifecycle",
		Model:    "model-lc",
		Task:     "do lifecycle test",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if _, err := manager.Wait(context.Background(), created.ID, 5*time.Millisecond); err != nil {
		t.Fatalf("wait: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) < 3 {
		t.Fatalf("expected at least 3 lifecycle events, got %d: %+v", len(events), events)
	}
	types := map[string]bool{}
	for _, event := range events {
		types[event.EventType] = true
		if strings.TrimSpace(event.RecordText) == "" {
			t.Fatalf("expected non-empty record text in lifecycle event: %+v", event)
		}
	}
	for _, typ := range []string{"subagent.created", "subagent.running", "subagent.completed"} {
		if !types[typ] {
			t.Fatalf("missing lifecycle event type %q in %+v", typ, events)
		}
	}
}

func TestManager_LifecycleHookFailedRecord(t *testing.T) {
	var (
		mu     sync.Mutex
		events []LifecycleEvent
	)
	manager := NewManagerWithLifecycle(func(_ context.Context, _ Agent) (string, error) {
		return "", context.DeadlineExceeded
	}, func(event LifecycleEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	created, err := manager.Spawn(context.Background(), SpawnConfig{
		Task: "fail lifecycle",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	_, _ = manager.Wait(context.Background(), created.ID, 5*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	foundFailed := false
	for _, event := range events {
		if event.EventType != "subagent.failed" {
			continue
		}
		foundFailed = true
		if !strings.Contains(strings.ToLower(event.RecordText), "error=") {
			t.Fatalf("expected failed lifecycle record include error, got %q", event.RecordText)
		}
	}
	if !foundFailed {
		t.Fatalf("expected subagent.failed lifecycle event in %+v", events)
	}
}
