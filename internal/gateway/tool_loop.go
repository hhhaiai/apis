package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/toolruntime"
)

func (s *server) completeWithToolLoop(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	enabled, maxSteps := toolLoopConfigFromMetadata(req.Metadata)
	if !enabled || len(req.Tools) == 0 {
		return s.orchestrator.Complete(ctx, req)
	}

	working := req
	working.Messages = append([]orchestrator.Message(nil), req.Messages...)
	allowedTools := allowedToolNames(req.Tools)
	totalUsage := orchestrator.Usage{}
	var last orchestrator.Response

	for i := 0; i < maxSteps; i++ {
		resp, err := s.orchestrator.Complete(ctx, working)
		if err != nil {
			return orchestrator.Response{}, err
		}
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		last = resp

		toolBlocks := toolUseBlocks(resp.Blocks)
		if len(toolBlocks) == 0 || !strings.EqualFold(strings.TrimSpace(resp.StopReason), "tool_use") {
			last.Usage = totalUsage
			return last, nil
		}

		working.Messages = append(working.Messages, orchestrator.Message{
			Role:    "assistant",
			Content: assistantBlocksToContent(resp.Blocks),
		})
		working.Messages = append(working.Messages, orchestrator.Message{
			Role:    "user",
			Content: s.executeToolBlocks(ctx, toolBlocks, allowedTools),
		})
	}

	if strings.EqualFold(strings.TrimSpace(last.StopReason), "tool_use") {
		last.StopReason = "max_turns"
	}
	last.Usage = totalUsage
	return last, nil
}

func toolLoopConfigFromMetadata(metadata map[string]any) (enabled bool, maxSteps int) {
	mode := ""
	if metadata != nil {
		if v, ok := metadata["tool_loop_mode"]; ok {
			if text, ok := v.(string); ok {
				mode = strings.ToLower(strings.TrimSpace(text))
			}
		}
	}
	switch mode {
	case "server", "server_loop":
		enabled = true
	default:
		enabled = false
	}
	maxSteps = 4
	if metadata != nil {
		if v, ok := metadata["tool_loop_max_steps"]; ok {
			if n, ok := parseInt(v); ok && n > 0 {
				maxSteps = n
			}
		}
	}
	return enabled, maxSteps
}

func parseInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case json.Number:
		x, err := n.Int64()
		return int(x), err == nil
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0, false
		}
		var parsed int
		_, err := fmt.Sscanf(n, "%d", &parsed)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func allowedToolNames(tools []orchestrator.Tool) map[string]struct{} {
	out := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		name := strings.ToLower(strings.TrimSpace(t.Name))
		if name == "" {
			continue
		}
		out[name] = struct{}{}
	}
	return out
}

func toolUseBlocks(blocks []orchestrator.AssistantBlock) []orchestrator.AssistantBlock {
	out := make([]orchestrator.AssistantBlock, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "tool_use" {
			out = append(out, b)
		}
	}
	return out
}

func assistantBlocksToContent(blocks []orchestrator.AssistantBlock) []any {
	out := make([]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case "text":
			out = append(out, map[string]any{
				"type": "text",
				"text": b.Text,
			})
		case "tool_use":
			callID := strings.TrimSpace(b.ID)
			if callID == "" {
				callID = "toolu_auto"
			}
			block := map[string]any{
				"type":  "tool_use",
				"id":    callID,
				"name":  strings.TrimSpace(b.Name),
				"input": b.Input,
			}
			if block["input"] == nil {
				block["input"] = map[string]any{}
			}
			out = append(out, block)
		}
	}
	return out
}

func (s *server) executeToolBlocks(ctx context.Context, calls []orchestrator.AssistantBlock, allowed map[string]struct{}) []any {
	out := make([]any, 0, len(calls))
	for _, call := range calls {
		name := strings.ToLower(strings.TrimSpace(call.Name))
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			callID = "toolu_auto"
		}
		if _, ok := allowed[name]; !ok {
			out = append(out, toolResultBlock(callID, "tool is not declared in request tools", true))
			continue
		}

		result, err := s.toolExecutor.Execute(ctx, toolruntime.Call{
			ID:    callID,
			Name:  name,
			Input: call.Input,
		})
		if err != nil {
			out = append(out, toolResultBlock(callID, err.Error(), true))
			continue
		}
		content := renderToolResultContent(result.Content)
		out = append(out, toolResultBlock(callID, content, result.IsError))
	}
	if len(out) == 0 {
		out = append(out, toolResultBlock("toolu_none", "no tool calls", true))
	}
	return out
}

func toolResultBlock(toolUseID, content string, isError bool) map[string]any {
	return map[string]any{
		"type":        "tool_result",
		"tool_use_id": toolUseID,
		"content":     content,
		"is_error":    isError,
	}
}

func renderToolResultContent(content any) string {
	switch v := content.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", content)
		}
		return string(raw)
	}
}
