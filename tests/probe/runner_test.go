package probe_test

import (
	. "ccgateway/internal/probe"
	"context"
	"errors"
	"testing"
	"time"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/upstream"
)

type fakeAdapter struct {
	name       string
	completeFn func(req orchestrator.Request) (orchestrator.Response, error)
}

func (a *fakeAdapter) Name() string { return a.name }

func (a *fakeAdapter) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	if a.completeFn != nil {
		return a.completeFn(req)
	}
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "ok"},
		},
		StopReason: "end_turn",
	}, nil
}

type fakeStreamAdapter struct {
	*fakeAdapter
	streamErr error
}

func (a *fakeStreamAdapter) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent, 1)
	errs := make(chan error, 1)
	if a.streamErr != nil {
		errs <- a.streamErr
		close(events)
		close(errs)
		return events, errs
	}
	events <- orchestrator.StreamEvent{Type: "message_start"}
	close(events)
	close(errs)
	return events, errs
}

func TestRunnerUpdatesProbeSuccess(t *testing.T) {
	health := scheduler.NewEngine(scheduler.Config{
		FailureThreshold: 2,
		Cooldown:         2 * time.Second,
		StrictProbeGate:  true,
	}, []string{"a1"})
	adapter := &fakeStreamAdapter{
		fakeAdapter: &fakeAdapter{
			name: "a1",
			completeFn: func(req orchestrator.Request) (orchestrator.Response, error) {
				if len(req.Tools) > 0 {
					return orchestrator.Response{
						Model:      req.Model,
						Blocks:     []orchestrator.AssistantBlock{{Type: "tool_use", Name: "get_weather", ID: "t1"}},
						StopReason: "tool_use",
					}, nil
				}
				return orchestrator.Response{
					Model:      req.Model,
					Blocks:     []orchestrator.AssistantBlock{{Type: "text", Text: "pong"}},
					StopReason: "end_turn",
				}, nil
			},
		},
	}

	r := NewRunner(Config{
		Enabled:       true,
		Interval:      100 * time.Millisecond,
		Timeout:       500 * time.Millisecond,
		StreamSmoke:   true,
		ToolSmoke:     true,
		DefaultModels: []string{"m1"},
	}, []upstream.Adapter{adapter}, health)

	r.RunOnce(context.Background())
	got := health.Snapshot()
	item, ok := got["a1"].(map[string]any)
	if !ok {
		t.Fatalf("expected adapter snapshot")
	}
	models, ok := item["models"].(map[string]any)
	if !ok {
		t.Fatalf("expected models snapshot")
	}
	mp, ok := models["m1"].(map[string]any)
	if !ok {
		t.Fatalf("expected model m1 snapshot, got %+v", models)
	}
	if exists, _ := mp["exists"].(bool); !exists {
		t.Fatalf("expected exists=true")
	}
	if streamOK, _ := mp["stream_ok"].(bool); !streamOK {
		t.Fatalf("expected stream_ok=true")
	}
	if toolOK, _ := mp["tool_ok"].(bool); !toolOK {
		t.Fatalf("expected tool_ok=true")
	}
}

func TestRunnerMarksMissingModel(t *testing.T) {
	health := scheduler.NewEngine(scheduler.Config{
		FailureThreshold: 2,
		Cooldown:         2 * time.Second,
		StrictProbeGate:  true,
	}, []string{"a1"})
	adapter := &fakeAdapter{
		name: "a1",
		completeFn: func(req orchestrator.Request) (orchestrator.Response, error) {
			return orchestrator.Response{}, errors.New("model not found")
		},
	}

	r := NewRunner(Config{
		Enabled:       true,
		Timeout:       200 * time.Millisecond,
		DefaultModels: []string{"missing-model"},
	}, []upstream.Adapter{adapter}, health)

	r.RunOnce(context.Background())
	got := health.Order(orchestrator.Request{Model: "missing-model"}, []string{"a1"}, false)
	if len(got) != 0 {
		t.Fatalf("expected unavailable endpoint to be filtered, got %v", got)
	}
}

func TestRunnerUpdateConfigPatch(t *testing.T) {
	health := scheduler.NewEngine(scheduler.Config{
		FailureThreshold: 2,
		Cooldown:         2 * time.Second,
	}, []string{"a1"})
	adapter := &fakeAdapter{name: "a1"}
	r := NewRunner(Config{
		Enabled:       true,
		Interval:      45 * time.Second,
		Timeout:       8 * time.Second,
		DefaultModels: []string{"m1"},
	}, []upstream.Adapter{adapter}, health)

	enabled := false
	intervalMS := int64(30000)
	timeoutMS := int64(6000)
	streamSmoke := true
	toolSmoke := true
	updated, err := r.UpdateConfigPatch(ConfigPatch{
		Enabled:       &enabled,
		IntervalMS:    &intervalMS,
		TimeoutMS:     &timeoutMS,
		DefaultModels: []string{"x1", "x2"},
		StreamSmoke:   &streamSmoke,
		ToolSmoke:     &toolSmoke,
	})
	if err != nil {
		t.Fatalf("update patch failed: %v", err)
	}
	if updated.Enabled {
		t.Fatalf("expected enabled=false")
	}
	if updated.Interval != 30*time.Second {
		t.Fatalf("expected interval 30s, got %v", updated.Interval)
	}
	if updated.Timeout != 6*time.Second {
		t.Fatalf("expected timeout 6s, got %v", updated.Timeout)
	}
	if len(updated.DefaultModels) != 2 {
		t.Fatalf("expected default models updated, got %+v", updated.DefaultModels)
	}
	if !updated.StreamSmoke || !updated.ToolSmoke {
		t.Fatalf("expected smoke flags true")
	}
}
