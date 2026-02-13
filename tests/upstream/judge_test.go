package upstream_test

import (
	. "ccgateway/internal/upstream"
	"context"
	"testing"
	"time"

	"ccgateway/internal/orchestrator"
)

func TestHeuristicJudgePrefersToolUseWhenToolsExpected(t *testing.T) {
	judge := NewHeuristicJudge()
	req := orchestrator.Request{
		Model: "m1",
		Tools: []orchestrator.Tool{
			{Name: "get_weather"},
		},
	}
	candidates := []JudgedCandidate{
		{
			AdapterName: "a1",
			Latency:     10 * time.Millisecond,
			Order:       0,
			Response: orchestrator.Response{
				Blocks:     []orchestrator.AssistantBlock{{Type: "text", Text: "plain text"}},
				StopReason: "end_turn",
			},
		},
		{
			AdapterName: "a2",
			Latency:     20 * time.Millisecond,
			Order:       1,
			Response: orchestrator.Response{
				Blocks:     []orchestrator.AssistantBlock{{Type: "tool_use", Name: "get_weather", ID: "t1"}},
				StopReason: "tool_use",
			},
		},
	}

	idx, err := judge.Select(context.Background(), req, candidates)
	if err != nil {
		t.Fatalf("unexpected judge error: %v", err)
	}
	if idx != 1 {
		t.Fatalf("expected tool_use candidate selected, got %d", idx)
	}
}
