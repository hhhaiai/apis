package upstream_test

import (
	. "ccgateway/internal/upstream"
	"context"
	"testing"
	"time"

	"ccgateway/internal/orchestrator"
)

type staticJudgeAdapter struct {
	name string
	text string
}

func (a *staticJudgeAdapter) Name() string { return a.name }

func (a *staticJudgeAdapter) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: a.text},
		},
		StopReason: "end_turn",
	}, nil
}

func TestLLMJudgeSelect(t *testing.T) {
	judge, err := NewLLMJudge(LLMJudgeConfig{
		Route:     []string{"judge-a"},
		Model:     "judge-model",
		Timeout:   2 * time.Second,
		MaxTokens: 32,
	}, []Adapter{
		&staticJudgeAdapter{name: "judge-a", text: "1"},
	})
	if err != nil {
		t.Fatalf("new llm judge: %v", err)
	}

	idx, err := judge.Select(context.Background(), orchestrator.Request{
		Model: "m1",
	}, []JudgedCandidate{
		{
			AdapterName: "a1",
			Latency:     20 * time.Millisecond,
			Order:       0,
			Response: orchestrator.Response{
				StopReason: "end_turn",
				Blocks:     []orchestrator.AssistantBlock{{Type: "text", Text: "short"}},
			},
		},
		{
			AdapterName: "a2",
			Latency:     30 * time.Millisecond,
			Order:       1,
			Response: orchestrator.Response{
				StopReason: "end_turn",
				Blocks:     []orchestrator.AssistantBlock{{Type: "text", Text: "better"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("judge select failed: %v", err)
	}
	if idx != 1 {
		t.Fatalf("expected index=1, got %d", idx)
	}
}

func TestParseJudgeIndex(t *testing.T) {
	idx, err := ParseJudgeIndex(orchestrator.Response{
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: `{"index":0}`},
		},
	}, 2)
	if err != nil {
		t.Fatalf("parse judge json: %v", err)
	}
	if idx != 0 {
		t.Fatalf("expected 0, got %d", idx)
	}
}
