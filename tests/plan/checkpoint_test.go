package plan_test

import (
	. "ccgateway/internal/plan"
	"testing"
)

func TestCheckpoint_CreateAndList(t *testing.T) {
	s := NewStore()
	p, _ := s.Create(CreateInput{Title: "Test Plan", Steps: []Step{{Title: "Step 1"}}})

	cp, err := s.CreateCheckpoint(p.ID, "before approve")
	if err != nil {
		t.Fatal(err)
	}
	if cp.Version != 1 {
		t.Fatalf("expected version 1, got %d", cp.Version)
	}
	if cp.Reason != "before approve" {
		t.Fatalf("expected reason, got %s", cp.Reason)
	}

	// Modify plan
	s.Approve(p.ID, ApproveInput{})
	s.CreateCheckpoint(p.ID, "after approve")

	cps := s.ListCheckpoints(p.ID)
	if len(cps) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(cps))
	}
	// First checkpoint should have draft status
	if cps[0].PlanState.Status != StatusDraft {
		t.Fatalf("expected draft in checkpoint 1, got %s", cps[0].PlanState.Status)
	}
	// Second checkpoint should have approved status
	if cps[1].PlanState.Status != StatusApproved {
		t.Fatalf("expected approved in checkpoint 2, got %s", cps[1].PlanState.Status)
	}
}

func TestCheckpoint_Rollback(t *testing.T) {
	s := NewStore()
	p, _ := s.Create(CreateInput{Title: "Test Plan", Steps: []Step{{Title: "Step 1"}}})

	// Save draft state
	s.CreateCheckpoint(p.ID, "draft state")

	// Modify
	s.Approve(p.ID, ApproveInput{})

	// Verify current is approved
	cur, _ := s.Get(p.ID)
	if cur.Status != StatusApproved {
		t.Fatalf("expected approved, got %s", cur.Status)
	}

	// Rollback to version 1
	rolled, err := s.Rollback(p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if rolled.Status != StatusDraft {
		t.Fatalf("expected draft after rollback, got %s", rolled.Status)
	}

	// Verify store has rolled back
	cur2, _ := s.Get(p.ID)
	if cur2.Status != StatusDraft {
		t.Fatalf("expected draft in store, got %s", cur2.Status)
	}
}

func TestCheckpoint_RollbackInvalidVersion(t *testing.T) {
	s := NewStore()
	p, _ := s.Create(CreateInput{Title: "Test"})
	s.CreateCheckpoint(p.ID, "v1")

	_, err := s.Rollback(p.ID, 0)
	if err == nil {
		t.Fatal("expected error for version 0")
	}
	_, err = s.Rollback(p.ID, 99)
	if err == nil {
		t.Fatal("expected error for out of range version")
	}
}

func TestCheckpoint_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.CreateCheckpoint("nonexistent", "test")
	if err == nil {
		t.Fatal("expected error for nonexistent plan")
	}
}
