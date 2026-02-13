package upstream_test

import (
	. "ccgateway/internal/upstream"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"ccgateway/internal/orchestrator"
)

type fixedSelector struct {
	order        []string
	successCount int
	failureCount int
}

func (s *fixedSelector) Order(_ orchestrator.Request, _ []string, _ bool) []string {
	return append([]string(nil), s.order...)
}

func (s *fixedSelector) ObserveSuccess(_ string, _ string, _ time.Duration) {
	s.successCount++
}

func (s *fixedSelector) ObserveFailure(_ string, _ string, _ error) {
	s.failureCount++
}

func TestRouterServiceFallback(t *testing.T) {
	svc := NewRouterService(RouterConfig{
		DefaultRoute: []string{"bad", "good"},
		Timeout:      2 * time.Second,
		Retries:      0,
	}, []Adapter{
		NewMockAdapter("bad", true),
		NewMockAdapter("good", false),
	})

	resp, err := svc.Complete(context.Background(), orchestrator.Request{
		Model:     "claude-test",
		MaxTokens: 128,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("expected success after fallback, got error: %v", err)
	}
	if !resp.Trace.FallbackUsed {
		t.Fatalf("expected fallback_used=true")
	}
	if resp.Trace.Provider != "good" {
		t.Fatalf("expected provider=good, got %q", resp.Trace.Provider)
	}
}

func TestRouterServiceModelRoute(t *testing.T) {
	svc := NewRouterService(RouterConfig{
		Routes: map[string][]string{
			"model/a": []string{"a2"},
		},
		DefaultRoute: []string{"a1"},
		Timeout:      2 * time.Second,
	}, []Adapter{
		NewMockAdapter("a1", false),
		NewMockAdapter("a2", false),
	})

	resp, err := svc.Complete(context.Background(), orchestrator.Request{
		Model:     "model/a",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello route"},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp.Trace.Provider != "a2" {
		t.Fatalf("expected provider a2 from route, got %q", resp.Trace.Provider)
	}
}

func TestRouterServiceUsesSelectorOrder(t *testing.T) {
	selector := &fixedSelector{order: []string{"a2", "a1"}}
	svc := NewRouterService(RouterConfig{
		DefaultRoute: []string{"a1", "a2"},
		Timeout:      2 * time.Second,
		Selector:     selector,
	}, []Adapter{
		NewMockAdapter("a1", false),
		NewMockAdapter("a2", false),
	})

	resp, err := svc.Complete(context.Background(), orchestrator.Request{
		Model:     "model/x",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello selector"},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp.Trace.Provider != "a2" {
		t.Fatalf("expected selector to prioritize a2, got %q", resp.Trace.Provider)
	}
	if selector.successCount == 0 {
		t.Fatalf("expected selector observe success to be called")
	}
}

func TestRouterServiceRetries(t *testing.T) {
	svc := NewRouterService(RouterConfig{
		DefaultRoute: []string{"always-fail"},
		Retries:      2,
		Timeout:      2 * time.Second,
	}, []Adapter{
		NewMockAdapter("always-fail", true),
	})

	_, err := svc.Complete(context.Background(), orchestrator.Request{
		Model:     "claude-test",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err == nil {
		t.Fatalf("expected error when all retries fail")
	}
	if !strings.Contains(err.Error(), "forced failure") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRouterServiceMetadataRouteOverride(t *testing.T) {
	svc := NewRouterService(RouterConfig{
		DefaultRoute: []string{"a1"},
		Timeout:      2 * time.Second,
	}, []Adapter{
		NewMockAdapter("a1", false),
		NewMockAdapter("a2", false),
	})

	resp, err := svc.Complete(context.Background(), orchestrator.Request{
		Model:     "x",
		MaxTokens: 32,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
		Metadata: map[string]any{
			"routing_adapter_route": []string{"a2", "a1"},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if resp.Trace.Provider != "a2" {
		t.Fatalf("expected provider a2 via metadata route, got %q", resp.Trace.Provider)
	}
}

type strictUnsupportedStreamAdapter struct {
	name string
}

type delayedTextAdapter struct {
	name  string
	delay time.Duration
	text  string
}

func (a *delayedTextAdapter) Name() string { return a.name }

func (a *delayedTextAdapter) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	time.Sleep(a.delay)
	if strings.TrimSpace(a.text) == "" {
		return orchestrator.Response{}, fmt.Errorf("empty text")
	}
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: a.text},
		},
		StopReason: "end_turn",
		Usage: orchestrator.Usage{
			InputTokens:  1,
			OutputTokens: len(strings.Fields(a.text)),
		},
	}, nil
}

func (a *strictUnsupportedStreamAdapter) Name() string { return a.name }

func (a *strictUnsupportedStreamAdapter) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "fallback-complete"},
		},
		StopReason: "end_turn",
		Usage: orchestrator.Usage{
			InputTokens:  1,
			OutputTokens: 1,
		},
	}, nil
}

func (a *strictUnsupportedStreamAdapter) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent)
	errs := make(chan error, 1)
	errs <- ErrStrictPassthroughUnsupported
	close(events)
	close(errs)
	return events, errs
}

func TestRouterServiceStrictSoftFallbackToComplete(t *testing.T) {
	svc := NewRouterService(RouterConfig{
		DefaultRoute: []string{"s1"},
		Timeout:      2 * time.Second,
	}, []Adapter{
		&strictUnsupportedStreamAdapter{name: "s1"},
	})

	events, errs := svc.Stream(context.Background(), orchestrator.Request{
		Model:     "m",
		MaxTokens: 16,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
		Metadata: map[string]any{
			"strict_stream_passthrough":      true,
			"strict_stream_passthrough_soft": true,
		},
	})

	gotAny := false
	for range events {
		gotAny = true
	}
	if !gotAny {
		t.Fatalf("expected synthetic stream fallback events")
	}
	for err := range errs {
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestRouterServiceParallelJudgeSelectsBetterCandidate(t *testing.T) {
	svc := NewRouterService(RouterConfig{
		DefaultRoute:        []string{"fast-short", "slow-better"},
		Timeout:             2 * time.Second,
		ParallelCandidates:  2,
		EnableResponseJudge: true,
		Judge:               NewHeuristicJudge(),
	}, []Adapter{
		&delayedTextAdapter{name: "fast-short", delay: 10 * time.Millisecond, text: "ok"},
		&delayedTextAdapter{
			name:  "slow-better",
			delay: 25 * time.Millisecond,
			text:  "This answer is more complete and contains several meaningful details.",
		},
	})

	resp, err := svc.Complete(context.Background(), orchestrator.Request{
		Model:     "m1",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "explain"},
		},
	})
	if err != nil {
		t.Fatalf("expected success, got err: %v", err)
	}
	if resp.Trace.Provider != "slow-better" {
		t.Fatalf("expected judged provider slow-better, got %q", resp.Trace.Provider)
	}
	if resp.Trace.SelectedBy != "judge" {
		t.Fatalf("expected selected_by judge, got %q", resp.Trace.SelectedBy)
	}
	if resp.Trace.CandidateCount != 2 {
		t.Fatalf("expected candidate_count=2, got %d", resp.Trace.CandidateCount)
	}
	if !resp.Trace.JudgeEnabled {
		t.Fatalf("expected judge_enabled=true")
	}
}
