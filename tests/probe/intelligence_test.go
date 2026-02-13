package probe_test

import (
	. "ccgateway/internal/probe"
	"context"
	"testing"
	"time"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/upstream"
)

type mockIntelAdapter struct {
	name      string
	responses map[string]string // question substring → answer
}

func (m *mockIntelAdapter) Name() string { return m.name }
func (m *mockIntelAdapter) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	msgs := req.Messages
	if len(msgs) == 0 {
		return orchestrator.Response{
			Blocks: []orchestrator.AssistantBlock{{Type: "text", Text: "no question"}},
		}, nil
	}
	question, _ := msgs[0].Content.(string)
	for sub, ans := range m.responses {
		if len(sub) > 0 && contains(question, sub) {
			return orchestrator.Response{
				Blocks:     []orchestrator.AssistantBlock{{Type: "text", Text: ans}},
				StopReason: "end_turn",
			}, nil
		}
	}
	return orchestrator.Response{
		Blocks:     []orchestrator.AssistantBlock{{Type: "text", Text: "I don't know"}},
		StopReason: "end_turn",
	}, nil
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchContains(s, sub)
}

func searchContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestProbeIntelligence_SmartModel(t *testing.T) {
	adapter := &mockIntelAdapter{
		name: "smart",
		responses: map[string]string{
			"sheep":     "9",
			"fibonacci": "def fibonacci(n):\n    if n <= 1:\n        return n\n    return fibonacci(n-1) + fibonacci(n-2)",
			"37 * 43":   "1591",
			"3 colors":  "Red\nBlue\nGreen",
			"pangram":   "The sentence is a pangram because it contains every letter of the alphabet.",
		},
	}

	result := ProbeIntelligence(context.Background(), adapter, "test-model", 5*time.Second)

	if result.Score < 60 {
		t.Errorf("smart model scored too low: %.1f (expected >= 60)", result.Score)
	}
	if len(result.Details) != 5 {
		t.Fatalf("expected 5 details, got %d", len(result.Details))
	}
	for _, d := range result.Details {
		t.Logf("  %s: %.1f/20", d.Category, d.Score)
	}
	t.Logf("Total: %.1f/100", result.Score)
}

func TestProbeIntelligence_DumbModel(t *testing.T) {
	adapter := &mockIntelAdapter{
		name:      "dumb",
		responses: map[string]string{},
	}

	result := ProbeIntelligence(context.Background(), adapter, "test-model", 5*time.Second)

	if result.Score > 20 {
		t.Errorf("dumb model scored too high: %.1f (expected <= 20)", result.Score)
	}
	t.Logf("Dumb model total: %.1f/100", result.Score)
}

func TestProbeIntelligence_Ranking(t *testing.T) {
	smart := &mockIntelAdapter{
		name: "smart",
		responses: map[string]string{
			"sheep":     "9",
			"fibonacci": "def fibonacci(n):\n    if n <= 1:\n        return n\n    return fibonacci(n-1) + fibonacci(n-2)",
			"37 * 43":   "1591",
			"3 colors":  "Red\nBlue\nGreen",
			"pangram":   "The sentence is a pangram because it contains every letter of the alphabet.",
		},
	}
	dumb := &mockIntelAdapter{
		name:      "dumb",
		responses: map[string]string{},
	}

	adapters := []upstream.Adapter{smart, dumb}
	results := make([]IntelligenceResult, 0, len(adapters))
	for _, a := range adapters {
		r := ProbeIntelligence(context.Background(), a, "test", 5*time.Second)
		results = append(results, r)
	}

	if results[0].Score <= results[1].Score {
		t.Errorf("smart (%0.f) should score higher than dumb (%.0f)", results[0].Score, results[1].Score)
	}
}
