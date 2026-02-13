package scheduler_test

import (
	. "ccgateway/internal/scheduler"
	"testing"
	"time"
)

func TestElection_SingleAdapter(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "only-one", Model: "model-a", Score: 80, TestedAt: time.Now()},
	})

	r := e.Result()
	if r == nil {
		t.Fatal("expected election result")
	}
	if r.SchedulerAdapter != "only-one" {
		t.Errorf("expected scheduler=only-one, got %s", r.SchedulerAdapter)
	}
	if r.Reason != "single_adapter" {
		t.Errorf("expected reason=single_adapter, got %s", r.Reason)
	}
	if len(r.Workers) != 0 {
		t.Errorf("expected 0 workers, got %d", len(r.Workers))
	}
}

func TestElection_MultipleAdapters(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true, MinScoreDifference: 5})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "weak", Model: "model-b", Score: 40, TestedAt: time.Now()},
		{AdapterName: "strong", Model: "model-a", Score: 85, TestedAt: time.Now()},
		{AdapterName: "medium", Model: "model-c", Score: 60, TestedAt: time.Now()},
	})

	r := e.Result()
	if r == nil {
		t.Fatal("expected election result")
	}
	if r.SchedulerAdapter != "strong" {
		t.Errorf("expected scheduler=strong, got %s", r.SchedulerAdapter)
	}
	if r.SchedulerScore != 85 {
		t.Errorf("expected score=85, got %.1f", r.SchedulerScore)
	}
	if len(r.Workers) != 2 {
		t.Errorf("expected 2 workers, got %d", len(r.Workers))
	}
	if r.Reason != "highest_intelligence_score" {
		t.Errorf("expected reason=highest_intelligence_score, got %s", r.Reason)
	}
}

func TestElection_CloseScores(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true, MinScoreDifference: 10})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "a", Model: "m", Score: 80, TestedAt: time.Now()},
		{AdapterName: "b", Model: "m", Score: 78, TestedAt: time.Now()},
	})

	r := e.Result()
	if r == nil {
		t.Fatal("expected election result")
	}
	if r.Reason != "close_scores_tiebreak" {
		t.Errorf("expected reason=close_scores_tiebreak, got %s", r.Reason)
	}
}

func TestElection_IsScheduler(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "leader", Model: "m", Score: 90, TestedAt: time.Now()},
		{AdapterName: "follower", Model: "m", Score: 50, TestedAt: time.Now()},
	})

	if !e.IsScheduler("leader") {
		t.Error("leader should be scheduler")
	}
	if e.IsScheduler("follower") {
		t.Error("follower should not be scheduler")
	}
}

func TestElection_WorkerAdapters(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "a", Model: "m", Score: 90, TestedAt: time.Now()},
		{AdapterName: "b", Model: "m", Score: 70, TestedAt: time.Now()},
		{AdapterName: "c", Model: "m", Score: 60, TestedAt: time.Now()},
	})

	workers := e.WorkerAdapters()
	if len(workers) != 2 {
		t.Fatalf("expected 2 workers, got %d", len(workers))
	}
	// Workers should be b and c (not a, since a is scheduler)
	found := map[string]bool{}
	for _, w := range workers {
		found[w] = true
	}
	if found["a"] {
		t.Error("scheduler 'a' should not be in workers")
	}
	if !found["b"] || !found["c"] {
		t.Error("expected b and c as workers")
	}
}

func TestElection_OnChange(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true})
	called := false
	e.SetOnChange(func(result ElectionResult) {
		called = true
		if result.SchedulerAdapter != "top" {
			t.Errorf("expected scheduler=top in callback, got %s", result.SchedulerAdapter)
		}
	})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "top", Model: "m", Score: 95, TestedAt: time.Now()},
	})
	if !called {
		t.Error("onChange should have been called")
	}
}

func TestElection_Snapshot(t *testing.T) {
	e := NewElection(ElectionConfig{Enabled: true})
	e.UpdateScores([]IntelligenceScore{
		{AdapterName: "a", Model: "m", Score: 80, TestedAt: time.Now()},
	})
	snap := e.Snapshot()
	if snap["enabled"] != true {
		t.Error("expected enabled=true")
	}
	if snap["scheduler_adapter"] != "a" {
		t.Errorf("expected scheduler_adapter=a, got %v", snap["scheduler_adapter"])
	}
}
