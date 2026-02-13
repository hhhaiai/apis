package scheduler_test

import (
	. "ccgateway/internal/scheduler"
	"errors"
	"testing"
	"time"

	"ccgateway/internal/orchestrator"
)

func TestOrderUsesHealthScore(t *testing.T) {
	e := NewEngine(Config{
		FailureThreshold: 2,
		Cooldown:         5 * time.Second,
	}, []string{"a1", "a2"})

	req := orchestrator.Request{Model: "m1"}
	e.ObserveFailure("a1", "m1", errors.New("upstream timeout"))
	e.ObserveSuccess("a2", "m1", 50*time.Millisecond)

	got := e.Order(req, []string{"a1", "a2"}, false)
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if got[0] != "a2" {
		t.Fatalf("expected healthy adapter first, got %q", got[0])
	}
}

func TestCooldownExcludesAdapter(t *testing.T) {
	e := NewEngine(Config{
		FailureThreshold: 1,
		Cooldown:         30 * time.Second,
	}, []string{"a1", "a2"})

	req := orchestrator.Request{Model: "m1"}
	e.ObserveFailure("a1", "m1", errors.New("boom"))

	got := e.Order(req, []string{"a1", "a2"}, false)
	if len(got) != 1 {
		t.Fatalf("expected only one eligible candidate, got %d (%v)", len(got), got)
	}
	if got[0] != "a2" {
		t.Fatalf("expected a2 after a1 cooldown, got %q", got[0])
	}
}

func TestProbeMarksModelUnavailable(t *testing.T) {
	e := NewEngine(Config{
		FailureThreshold: 2,
		Cooldown:         5 * time.Second,
	}, []string{"a1", "a2"})

	e.UpdateProbe("a1", "m1", ProbeResult{
		CheckedAt: time.Now(),
		Exists:    false,
		Error:     "model not found",
	})
	e.UpdateProbe("a2", "m1", ProbeResult{
		CheckedAt: time.Now(),
		Exists:    true,
	})

	got := e.Order(orchestrator.Request{Model: "m1"}, []string{"a1", "a2"}, false)
	if len(got) != 1 || got[0] != "a2" {
		t.Fatalf("expected only available model endpoint a2, got %v", got)
	}
}

func TestRequireToolProbe(t *testing.T) {
	e := NewEngine(Config{
		FailureThreshold:   2,
		Cooldown:           5 * time.Second,
		RequireToolProbe:   true,
		RequireStreamProbe: false,
	}, []string{"a1", "a2"})

	now := time.Now()
	e.UpdateProbe("a1", "m1", ProbeResult{
		CheckedAt:   now,
		Exists:      true,
		ToolChecked: true,
		ToolOK:      false,
	})
	e.UpdateProbe("a2", "m1", ProbeResult{
		CheckedAt:   now,
		Exists:      true,
		ToolChecked: true,
		ToolOK:      true,
	})

	got := e.Order(orchestrator.Request{
		Model: "m1",
		Tools: []orchestrator.Tool{{Name: "get_weather"}},
	}, []string{"a1", "a2"}, false)
	if len(got) != 1 || got[0] != "a2" {
		t.Fatalf("expected tool-capable adapter a2, got %v", got)
	}
}

func TestUpdateConfigPatch(t *testing.T) {
	e := NewEngine(Config{
		FailureThreshold: 2,
		Cooldown:         5 * time.Second,
	}, []string{"a1"})

	n := 4
	cooldownMS := int64(12000)
	strict := true
	updated, err := e.UpdateConfigPatch(ConfigPatch{
		FailureThreshold: &n,
		CooldownMS:       &cooldownMS,
		StrictProbeGate:  &strict,
	})
	if err != nil {
		t.Fatalf("update config patch failed: %v", err)
	}
	if updated.FailureThreshold != 4 {
		t.Fatalf("expected threshold 4, got %d", updated.FailureThreshold)
	}
	if updated.Cooldown != 12*time.Second {
		t.Fatalf("expected cooldown 12s, got %v", updated.Cooldown)
	}
	if !updated.StrictProbeGate {
		t.Fatalf("expected strict gate true")
	}
}
