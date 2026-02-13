package upstream

import (
	"context"
	"fmt"
	"strings"

	"ccgateway/internal/orchestrator"
)

type MockAdapter struct {
	name      string
	alwaysErr bool
}

func NewMockAdapter(name string, alwaysErr bool) *MockAdapter {
	return &MockAdapter{name: name, alwaysErr: alwaysErr}
}

func (a *MockAdapter) Name() string {
	return a.name
}

func (a *MockAdapter) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	if a.alwaysErr {
		return orchestrator.Response{}, fmt.Errorf("adapter %s forced failure", a.name)
	}
	last := extractLastUserText(req.Messages)
	inputTokens := estimateTokens(last)

	if len(req.Tools) > 0 && strings.Contains(strings.ToLower(last), "tool") {
		return orchestrator.Response{
			Model: req.Model,
			Blocks: []orchestrator.AssistantBlock{
				{
					Type: "tool_use",
					ID:   "toolu_mock",
					Name: req.Tools[0].Name,
					Input: map[string]any{
						"query": strings.TrimSpace(last),
					},
				},
			},
			StopReason: "tool_use",
			Usage:      orchestrator.Usage{InputTokens: inputTokens, OutputTokens: 18},
		}, nil
	}

	text := fmt.Sprintf("[%s] Processed request: %s", a.name, strings.TrimSpace(last))
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: text},
		},
		StopReason: "end_turn",
		Usage:      orchestrator.Usage{InputTokens: inputTokens, OutputTokens: estimateTokens(text)},
	}, nil
}

func extractLastUserText(messages []orchestrator.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		switch c := messages[i].Content.(type) {
		case string:
			return c
		case []any:
			var parts []string
			for _, item := range c {
				block, ok := item.(map[string]any)
				if !ok {
					continue
				}
				if block["type"] == "text" {
					if text, ok := block["text"].(string); ok {
						parts = append(parts, text)
					}
				}
			}
			return strings.Join(parts, " ")
		}
	}
	return ""
}

func estimateTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 1
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return 1
	}
	return len(words)
}
