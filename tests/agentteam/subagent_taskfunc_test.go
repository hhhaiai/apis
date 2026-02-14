package agentteam_test

import (
	. "ccgateway/internal/agentteam"
	"context"
	"strings"
	"testing"

	"ccgateway/internal/subagent"
)

func TestNewSubagentTaskFunc_SpawnAndWait(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, a subagent.Agent) (string, error) {
		return "subagent-ok:" + a.Task, nil
	})
	taskFn := NewSubagentTaskFunc(manager)
	if taskFn == nil {
		t.Fatalf("expected non-nil task func")
	}

	result, err := taskFn(context.Background(), Agent{
		ID:    "lead_1",
		Name:  "Lead",
		Role:  "lead",
		Model: "gpt-test",
		Meta: map[string]any{
			"permissions": []any{"read", "write"},
		},
	}, Task{
		ID:          "task_1",
		Title:       "Analyze",
		Description: "Read code and summarize",
		Meta: map[string]any{
			"team_id": "team_1",
		},
	})
	if err != nil {
		t.Fatalf("task func error: %v", err)
	}
	if !strings.Contains(result, "subagent-ok:") {
		t.Fatalf("unexpected result: %q", result)
	}
	if !strings.Contains(result, "subagent=") {
		t.Fatalf("expected subagent marker in result: %q", result)
	}

	spawned := manager.List("team_1")
	if len(spawned) != 1 {
		t.Fatalf("expected one spawned subagent, got %d", len(spawned))
	}
	if spawned[0].ParentID != "team_1" {
		t.Fatalf("expected team parent id, got %q", spawned[0].ParentID)
	}
	if spawned[0].Model != "gpt-test" {
		t.Fatalf("expected propagated model, got %q", spawned[0].Model)
	}
	if len(spawned[0].Permissions) != 2 {
		t.Fatalf("expected propagated permissions, got %+v", spawned[0].Permissions)
	}
	if !strings.Contains(spawned[0].Task, "Analyze") || !strings.Contains(spawned[0].Task, "Read code") {
		t.Fatalf("unexpected spawned task text: %q", spawned[0].Task)
	}
}

func TestNewSubagentTaskFunc_Failure(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, _ subagent.Agent) (string, error) {
		return "", context.DeadlineExceeded
	})
	taskFn := NewSubagentTaskFunc(manager)
	_, err := taskFn(context.Background(), Agent{
		ID:   "lead_2",
		Name: "Lead",
		Role: "lead",
	}, Task{
		Title: "Do something",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}
