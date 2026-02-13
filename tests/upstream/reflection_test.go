package upstream_test

import (
	. "ccgateway/internal/upstream"
	"context"
	"strings"
	"testing"

	"ccgateway/internal/orchestrator"
)

func TestApplyReflectionLoop_ZeroPasses(t *testing.T) {
	svc := NewRouterService(RouterConfig{}, nil)
	resp := orchestrator.Response{
		Blocks: []orchestrator.AssistantBlock{{Type: "text", Text: "hello"}},
	}
	got := svc.ApplyReflectionLoop(context.Background(), resp, orchestrator.Request{}, 0)
	if got.Blocks[0].Text != "hello" {
		t.Fatalf("expected original text, got %q", got.Blocks[0].Text)
	}
	if got.Trace.ReflectionPasses != 0 {
		t.Fatalf("expected 0 passes, got %d", got.Trace.ReflectionPasses)
	}
}

func TestApplyReflectionLoop_WithMockAdapter(t *testing.T) {
	callCount := 0
	mock := NewMockAdapter("mock-reflect", false)

	svc := NewRouterService(RouterConfig{
		DefaultRoute:     []string{"mock-reflect"},
		ReflectionPasses: 1,
	}, []Adapter{mock})

	resp := orchestrator.Response{
		Blocks: []orchestrator.AssistantBlock{{Type: "text", Text: "initial draft"}},
		Usage:  orchestrator.Usage{InputTokens: 10, OutputTokens: 20},
	}

	req := orchestrator.Request{
		Model:    "test-model",
		Metadata: map[string]any{},
	}

	_ = callCount
	got := svc.ApplyReflectionLoop(context.Background(), resp, req, 1)

	if got.Trace.ReflectionPasses != 1 {
		t.Fatalf("expected 1 reflect pass, got %d", got.Trace.ReflectionPasses)
	}
	// Usage should be accumulated (original + critique + fix)
	if got.Usage.InputTokens < 10 {
		t.Fatalf("expected accumulated input tokens >= 10, got %d", got.Usage.InputTokens)
	}
}

func TestExtractTextFromBlocks(t *testing.T) {
	blocks := []orchestrator.AssistantBlock{
		{Type: "text", Text: "hello"},
		{Type: "tool_use", Text: "ignored"},
		{Type: "text", Text: "world"},
	}
	got := ExtractTextFromBlocks(blocks)
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Fatalf("expected hello and world, got %q", got)
	}
	if strings.Contains(got, "ignored") {
		t.Fatalf("should not contain tool_use text")
	}
}
