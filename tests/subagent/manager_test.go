package subagent_test

import (
	. "ccgateway/internal/subagent"
	"context"
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
