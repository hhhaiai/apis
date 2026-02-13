package upstream_test

import (
	. "ccgateway/internal/upstream"
	"testing"
)

func TestNewJudgeFromEnvHeuristic(t *testing.T) {
	t.Setenv("JUDGE_MODE", "heuristic")
	judge, err := NewJudgeFromEnv([]Adapter{
		NewMockAdapter("a1", false),
	}, []string{"a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if judge == nil {
		t.Fatalf("expected judge")
	}
}

func TestNewJudgeFromEnvLLM(t *testing.T) {
	t.Setenv("JUDGE_MODE", "llm")
	t.Setenv("JUDGE_ROUTE", "judge-a")
	t.Setenv("JUDGE_MODEL", "judge-model")
	judge, err := NewJudgeFromEnv([]Adapter{
		&staticJudgeAdapter{name: "judge-a", text: "0"},
	}, []string{"judge-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if judge == nil {
		t.Fatalf("expected llm judge")
	}
}
