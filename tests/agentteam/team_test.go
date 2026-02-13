package agentteam_test

import (
	. "ccgateway/internal/agentteam"
	"context"
	"fmt"
	"testing"
)

func TestTeam_AddRemoveAgent(t *testing.T) {
	team := NewTeam("t1", "test-team", nil)
	err := team.AddAgent(Agent{ID: "a1", Name: "Alice", Role: "lead"})
	if err != nil {
		t.Fatal(err)
	}
	if agents := team.ListAgents(); len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	team.RemoveAgent("a1")
	if agents := team.ListAgents(); len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

func TestTeam_AddAgentValidation(t *testing.T) {
	team := NewTeam("t1", "test", nil)
	err := team.AddAgent(Agent{})
	if err == nil {
		t.Fatal("expected error for empty agent")
	}
}

func TestTeam_TaskCRUD(t *testing.T) {
	team := NewTeam("t1", "test", nil)
	task, err := team.AddTask("Build feature", "Implement the thing", "a1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if task.Title != "Build feature" {
		t.Fatalf("unexpected title: %s", task.Title)
	}
	got, ok := team.GetTask(task.ID)
	if !ok {
		t.Fatal("task not found")
	}
	if got.Status != TaskPending {
		t.Fatalf("expected pending, got %s", got.Status)
	}
	tasks := team.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
}

func TestTeam_Messaging(t *testing.T) {
	team := NewTeam("t1", "test", nil)
	_ = team.AddAgent(Agent{ID: "a1", Name: "Alice", Role: "lead"})
	_ = team.AddAgent(Agent{ID: "a2", Name: "Bob", Role: "implementer"})

	// Direct message
	team.SendMessage("a1", "a2", "Please start implementing")
	msgs := team.ReadMailbox("a2")
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "Please start implementing" {
		t.Fatalf("unexpected content: %s", msgs[0].Content)
	}

	// Broadcast
	team.SendMessage("a1", "*", "Team standup in 5 min")
	msgs = team.ReadMailbox("a2")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages after broadcast, got %d", len(msgs))
	}
	// a1 shouldn't get own broadcast
	a1msgs := team.ReadMailbox("a1")
	if len(a1msgs) != 0 {
		t.Fatalf("sender should not receive own broadcast, got %d", len(a1msgs))
	}
}

func TestTeam_Orchestrate(t *testing.T) {
	results := map[string]string{}
	team := NewTeam("t1", "test", func(_ context.Context, _ Agent, task Task) (string, error) {
		results[task.Title] = "done"
		return "ok", nil
	})
	_ = team.AddAgent(Agent{ID: "a1", Name: "Alice", Role: "lead"})

	t1, _ := team.AddTask("Step 1", "", "a1", nil)
	_, _ = team.AddTask("Step 2", "", "a1", []string{t1.ID})

	if err := team.Orchestrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 completed tasks, got %d", len(results))
	}
}

func TestTeam_OrchestrateWithFailure(t *testing.T) {
	team := NewTeam("t1", "test", func(_ context.Context, _ Agent, task Task) (string, error) {
		if task.Title == "Failing" {
			return "", fmt.Errorf("intentional failure")
		}
		return "ok", nil
	})
	_ = team.AddAgent(Agent{ID: "a1", Name: "Alice", Role: "lead"})

	failing, _ := team.AddTask("Failing", "", "a1", nil)
	_, _ = team.AddTask("Dependent", "", "a1", []string{failing.ID})

	err := team.Orchestrate(context.Background())
	// Should error because "Dependent" can never run (its dependency failed)
	if err == nil {
		t.Fatal("expected error due to unresolvable dependency")
	}
}
