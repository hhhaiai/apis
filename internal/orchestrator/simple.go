package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type SimpleService struct{}

func NewSimpleService() *SimpleService {
	return &SimpleService{}
}

func (s *SimpleService) Complete(_ context.Context, req Request) (Response, error) {
	last := extractLastUserText(req.Messages)

	resp := Response{
		Model: req.Model,
		Trace: Trace{
			Provider:         "skeleton-provider",
			Model:            req.Model,
			FallbackUsed:     false,
			ReflectionPasses: 1,
		},
	}

	inputTokens := estimateTokens(last)
	if len(req.Tools) > 0 && strings.Contains(strings.ToLower(last), "tool") {
		tool := req.Tools[0]
		resp.Blocks = []AssistantBlock{
			{
				Type: "tool_use",
				ID:   "toolu_" + shortID(),
				Name: tool.Name,
				Input: map[string]any{
					"query": strings.TrimSpace(last),
				},
			},
		}
		resp.StopReason = "tool_use"
		resp.Usage = Usage{InputTokens: inputTokens, OutputTokens: 24}
		return resp, nil
	}

	text := fmt.Sprintf("Processed request: %s", strings.TrimSpace(last))
	resp.Blocks = []AssistantBlock{
		{
			Type: "text",
			Text: text,
		},
	}
	resp.StopReason = "end_turn"
	resp.Usage = Usage{InputTokens: inputTokens, OutputTokens: estimateTokens(text)}
	return resp, nil
}

func (s *SimpleService) Stream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		resp, err := s.Complete(ctx, req)
		if err != nil {
			errs <- err
			return
		}

		events <- StreamEvent{Type: "message_start"}
		for i, b := range resp.Blocks {
			events <- StreamEvent{Type: "content_block_start", Index: i, Block: b}

			switch b.Type {
			case "text":
				for _, c := range splitTextDeltas(b.Text, 24) {
					if c == "" {
						continue
					}
					events <- StreamEvent{
						Type:      "content_block_delta",
						Index:     i,
						DeltaText: c,
					}
				}
			case "tool_use":
				raw, _ := json.Marshal(b.Input)
				events <- StreamEvent{
					Type:      "content_block_delta",
					Index:     i,
					DeltaJSON: string(raw),
				}
			}

			events <- StreamEvent{Type: "content_block_stop", Index: i}
		}

		events <- StreamEvent{
			Type:       "message_delta",
			StopReason: resp.StopReason,
			Usage:      resp.Usage,
		}
		events <- StreamEvent{Type: "message_stop"}
	}()

	return events, errs
}

func extractLastUserText(messages []Message) string {
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

func splitTextDeltas(text string, n int) []string {
	if n <= 0 {
		return []string{text}
	}
	var out []string
	for len(text) > n {
		out = append(out, text[:n])
		text = text[n:]
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}

func shortID() string {
	return fmt.Sprintf("%x", time.Now().UnixNano()&0xffffff)
}
