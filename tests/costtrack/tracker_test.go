package costtrack_test

import (
	. "ccgateway/internal/costtrack"
	"math"
	"testing"
)

func TestTracker_Record(t *testing.T) {
	tracker := New(nil, 0)
	c := tracker.Record("sess_1", "gpt-4o", 1000, 500)
	if c.TotalCost <= 0 {
		t.Fatal("expected non-zero cost")
	}
	// gpt-4o: 2.5 per 1M input, 10.0 per 1M output
	expectedInput := 1000.0 / 1_000_000 * 2.5
	expectedOutput := 500.0 / 1_000_000 * 10.0
	if math.Abs(c.InputCost-expectedInput) > 0.0001 {
		t.Fatalf("input cost: expected %f, got %f", expectedInput, c.InputCost)
	}
	if math.Abs(c.OutputCost-expectedOutput) > 0.0001 {
		t.Fatalf("output cost: expected %f, got %f", expectedOutput, c.OutputCost)
	}
}

func TestTracker_Accumulation(t *testing.T) {
	tracker := New(nil, 0)
	tracker.Record("sess_1", "gpt-4o", 1000, 500)
	tracker.Record("sess_1", "gpt-4o", 1000, 500)
	total := tracker.Total("sess_1")
	single := 1000.0/1_000_000*2.5 + 500.0/1_000_000*10.0
	expected := single * 2
	if math.Abs(total.TotalCost-expected) > 0.0001 {
		t.Fatalf("accumulated cost: expected %f, got %f", expected, total.TotalCost)
	}
}

func TestTracker_BudgetCheck(t *testing.T) {
	tracker := New(nil, 0.001)                                      // very low budget
	tracker.Record("sess_1", "claude-3-opus", 1_000_000, 1_000_000) // expensive
	err := tracker.CheckBudget("sess_1")
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}
}

func TestTracker_NoBudgetLimit(t *testing.T) {
	tracker := New(nil, 0) // no limit
	tracker.Record("sess_1", "claude-3-opus", 10_000_000, 10_000_000)
	err := tracker.CheckBudget("sess_1")
	if err != nil {
		t.Fatalf("no budget limit should not error: %v", err)
	}
}

func TestTracker_UnknownModelDefaultPricing(t *testing.T) {
	tracker := New(nil, 0)
	c := tracker.Record("sess_1", "unknown-model-xyz", 1000, 500)
	if c.TotalCost <= 0 {
		t.Fatal("unknown model should use wildcard (*) pricing")
	}
}

func TestTracker_GlobalTotal(t *testing.T) {
	tracker := New(nil, 0)
	tracker.Record("sess_1", "gpt-4o", 1000, 500)
	tracker.Record("sess_2", "gpt-4o", 1000, 500)
	global := tracker.GlobalTotal()
	single := 1000.0/1_000_000*2.5 + 500.0/1_000_000*10.0
	expected := single * 2
	if math.Abs(global.TotalCost-expected) > 0.0001 {
		t.Fatalf("global total: expected %f, got %f", expected, global.TotalCost)
	}
}
