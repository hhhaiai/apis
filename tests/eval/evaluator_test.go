package eval_test

import (
	. "ccgateway/internal/eval"
	"context"
	"testing"
)

type mockCompleter struct {
	response string
	err      error
}

func (m *mockCompleter) CompleteSimple(_ context.Context, _, _, _ string) (string, error) {
	return m.response, m.err
}

func TestParseEvalOutput(t *testing.T) {
	raw := `{"accuracy":8.5,"completeness":7.0,"reasoning":9.0,"code_quality":7.0,"instruction_following":8.0,"analysis":"Good response overall."}`
	result, err := ParseEvalOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score < 7 || result.Score > 9 {
		t.Fatalf("unexpected score: %f", result.Score)
	}
	if result.Criteria["accuracy"] != 8.5 {
		t.Fatalf("expected accuracy 8.5, got %f", result.Criteria["accuracy"])
	}
	if result.Analysis != "Good response overall." {
		t.Fatalf("unexpected analysis: %s", result.Analysis)
	}
}

func TestParseEvalOutput_WithExtraText(t *testing.T) {
	raw := `Here is my evaluation:
{"accuracy":6,"completeness":5,"reasoning":7,"code_quality":7,"instruction_following":4,"analysis":"Moderate quality."}
That's my assessment.`
	result, err := ParseEvalOutput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Score < 4 || result.Score > 7 {
		t.Fatalf("unexpected score: %f", result.Score)
	}
}

func TestParseEvalOutput_InvalidJSON(t *testing.T) {
	_, err := ParseEvalOutput("no json here")
	if err == nil {
		t.Fatal("expected error for invalid output")
	}
}

func TestClamp(t *testing.T) {
	if Clamp(-1) != 0 {
		t.Fatal("expected 0 for negative")
	}
	if Clamp(15) != 10 {
		t.Fatal("expected 10 for over-max")
	}
	if Clamp(7.5) != 7.5 {
		t.Fatal("expected 7.5 for normal value")
	}
}

func TestEvaluator_Evaluate(t *testing.T) {
	mc := &mockCompleter{
		response: `{"accuracy":8,"completeness":7,"reasoning":9,"code_quality":7,"instruction_following":8,"analysis":"Well done."}`,
	}
	ev := NewEvaluator(mc, "test-judge")
	result, err := ev.Evaluate(context.Background(), "test-model", "hello", "world")
	if err != nil {
		t.Fatal(err)
	}
	if result.Model != "test-model" {
		t.Fatalf("expected model test-model, got %s", result.Model)
	}
	if result.Score < 7 || result.Score > 9 {
		t.Fatalf("unexpected score: %f", result.Score)
	}
}

func TestEvaluator_NilCompleter(t *testing.T) {
	ev := NewEvaluator(nil, "")
	_, err := ev.Evaluate(context.Background(), "", "", "")
	if err == nil {
		t.Fatal("expected error for nil completer")
	}
}
