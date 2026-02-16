package upstream_test

import (
	"context"
	"testing"
	"time"

	. "ccgateway/internal/upstream"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/scheduler"
)

func TestClassifyComplexity_WithTools(t *testing.T) {
	req := orchestrator.Request{
		Model: "test",
		Tools: []orchestrator.Tool{{Name: "bash"}},
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
	}
	// Use static fallback for tests
	if got := ClassifyComplexityStatic(req); got != "complex" {
		t.Errorf("expected complex for tool request, got %s", got)
	}
}

func TestClassifyComplexity_LongContext(t *testing.T) {
	long := make([]byte, 5000)
	for i := range long {
		long[i] = 'a'
	}
	req := orchestrator.Request{
		Model: "test",
		Messages: []orchestrator.Message{
			{Role: "user", Content: string(long)},
		},
	}
	// Use static fallback for tests
	if got := ClassifyComplexityStatic(req); got != "complex" {
		t.Errorf("expected complex for long context, got %s", got)
	}
}

func TestClassifyComplexity_PlanningKeyword(t *testing.T) {
	req := orchestrator.Request{
		Model:  "test",
		System: "You are an architect. Design a system.",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
	}
	// Use static fallback for tests
	if got := ClassifyComplexityStatic(req); got != "complex" {
		t.Errorf("expected complex for planning keyword, got %s", got)
	}
}

func TestClassifyComplexity_Simple(t *testing.T) {
	req := orchestrator.Request{
		Model: "test",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
	}
	// Use static fallback for tests
	if got := ClassifyComplexityStatic(req); got != "simple" {
		t.Errorf("expected simple, got %s", got)
	}
}

func TestDispatcher_ComplexRoutesToScheduler(t *testing.T) {
	election := scheduler.NewElection(scheduler.ElectionConfig{Enabled: true})
	election.UpdateScores([]scheduler.IntelligenceScore{
		{AdapterName: "smart", Model: "m", Score: 90, TestedAt: time.Now()},
		{AdapterName: "basic", Model: "m", Score: 50, TestedAt: time.Now()},
	})

	d := NewDispatcher(DispatchConfig{Enabled: true}, election)
	req := orchestrator.Request{
		Model: "test",
		Tools: []orchestrator.Tool{{Name: "bash"}},
		Messages: []orchestrator.Message{
			{Role: "user", Content: "run tests"},
		},
	}

	route := d.RouteRequest(context.Background(), req, []string{"smart", "basic"})
	if len(route) == 0 {
		t.Fatal("expected non-empty route")
	}
	if route[0] != "smart" {
		t.Errorf("expected scheduler 'smart' first for complex request, got %s", route[0])
	}
}

func TestDispatcher_SimpleRoundRobinsWorkers(t *testing.T) {
	election := scheduler.NewElection(scheduler.ElectionConfig{Enabled: true})
	election.UpdateScores([]scheduler.IntelligenceScore{
		{AdapterName: "smart", Model: "m", Score: 90, TestedAt: time.Now()},
		{AdapterName: "w1", Model: "m", Score: 60, TestedAt: time.Now()},
		{AdapterName: "w2", Model: "m", Score: 50, TestedAt: time.Now()},
	})

	d := NewDispatcher(DispatchConfig{Enabled: true, FallbackToScheduler: true}, election)
	simpleReq := orchestrator.Request{
		Model: "test",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
	}

	// Send multiple requests to check round-robin
	seen := map[string]int{}
	for i := 0; i < 10; i++ {
		route := d.RouteRequest(context.Background(), simpleReq, []string{"smart", "w1", "w2"})
		if len(route) == 0 {
			t.Fatal("expected non-empty route")
		}
		seen[route[0]]++
		// Scheduler should always be last fallback for simple
		if route[len(route)-1] != "smart" {
			t.Errorf("expected scheduler as last fallback, got %s", route[len(route)-1])
		}
	}

	// Workers should get traffic, not scheduler (as first choice)
	if seen["smart"] > 0 {
		t.Errorf("scheduler should not be first choice for simple requests, got %d times", seen["smart"])
	}
	if seen["w1"] == 0 || seen["w2"] == 0 {
		t.Errorf("expected both workers to get traffic: w1=%d, w2=%d", seen["w1"], seen["w2"])
	}
}

func TestDispatcher_DisabledReturnsNil(t *testing.T) {
	d := NewDispatcher(DispatchConfig{Enabled: false}, nil)
	route := d.RouteRequest(context.Background(), orchestrator.Request{}, nil)
	if route != nil {
		t.Error("expected nil when disabled")
	}
}

func TestDispatcher_NoElectionReturnsNil(t *testing.T) {
	election := scheduler.NewElection(scheduler.ElectionConfig{Enabled: true})
	// No scores → no election result
	d := NewDispatcher(DispatchConfig{Enabled: true}, election)
	route := d.RouteRequest(context.Background(), orchestrator.Request{}, nil)
	if route != nil {
		t.Error("expected nil when no election result")
	}
}

func TestDispatcher_UpdateConfig(t *testing.T) {
	d := NewDispatcher(DispatchConfig{Enabled: false}, nil)

	// Initially disabled
	cfg := d.GetConfig()
	if cfg.Enabled != false {
		t.Errorf("expected initial enabled=false, got %v", cfg.Enabled)
	}

	// Update to enabled
	d.UpdateConfig(DispatchConfig{Enabled: true})
	cfg = d.GetConfig()
	if cfg.Enabled != true {
		t.Errorf("expected updated enabled=true, got %v", cfg.Enabled)
	}
}

func TestDispatcher_UpdateConfigDynamically(t *testing.T) {
	election := scheduler.NewElection(scheduler.ElectionConfig{Enabled: true})
	election.UpdateScores([]scheduler.IntelligenceScore{
		{AdapterName: "smart", Model: "m", Score: 90, TestedAt: time.Now()},
		{AdapterName: "w1", Model: "m", Score: 60, TestedAt: time.Now()},
	})

	// Start disabled
	d := NewDispatcher(DispatchConfig{Enabled: false}, election)
	req := orchestrator.Request{
		Model: "test",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
	}

	// Should return nil when disabled
	route := d.RouteRequest(context.Background(), req, []string{"smart", "w1"})
	if route != nil {
		t.Error("expected nil when disabled")
	}

	// Enable dispatch
	d.UpdateConfig(DispatchConfig{Enabled: true})

	// Now should return a route
	route = d.RouteRequest(context.Background(), req, []string{"smart", "w1"})
	if route == nil {
		t.Error("expected route when enabled")
	}
}

func TestDispatcher_NilDispatcher(t *testing.T) {
	// Test nil dispatcher safety
	var d *Dispatcher

	// These should not panic
	cfg := d.GetConfig()
	if cfg.Enabled != false {
		t.Errorf("expected nil dispatcher to return disabled config, got %v", cfg.Enabled)
	}

	d.UpdateConfig(DispatchConfig{Enabled: true}) // should not panic

	snap := d.Snapshot()
	if snap != nil {
		t.Error("expected nil snapshot for nil dispatcher")
	}
}
