package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/toolruntime"
)

// toolAdapter 将不支持 tools 的 API 适配为支持 Claude Code Tools
type toolAdapter struct {
	executor     toolruntime.Executor
	toolLoopMode string
	maxSteps     int
	enableReAct  bool
	enableJSON   bool
}

// newToolAdapter creates a new tool adapter
func newToolAdapter(executor toolruntime.Executor, toolLoopMode string, maxSteps int) *toolAdapter {
	return &toolAdapter{
		executor:     executor,
		toolLoopMode: toolLoopMode,
		maxSteps:     maxSteps,
		enableReAct:  true,
		enableJSON:   true,
	}
}

// ToolInfo 工具信息
type ToolInfo struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// adaptToolsToPrompt 将 tools 转换为 prompt 指导模型使用模拟方式调用工具
func (a *toolAdapter) adaptToolsToPrompt(tools []ToolInfo) string {
	if len(tools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Available Tools\n")
	sb.WriteString("You can use the following tools to help answer the user's question:\n\n")

	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("### %s\n", tool.Name))
		if tool.Description != "" {
			sb.WriteString(fmt.Sprintf("%s\n", tool.Description))
		}
		if len(tool.InputSchema) > 0 {
			sb.WriteString("Parameters:\n")
			for key, prop := range tool.InputSchema {
				required := ""
				if isRequired(prop) {
					required = " (required)"
				}
				sb.WriteString(fmt.Sprintf("  - %s%s\n", key, required))
				if propMap, ok := prop.(map[string]any); ok {
					if desc, ok := propMap["description"].(string); ok && desc != "" {
						sb.WriteString(fmt.Sprintf("    - %s\n", desc))
					}
				}
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n## Tool Usage Instructions\n")
	sb.WriteString("Since the upstream API does not support tools natively, you must:\n")
	sb.WriteString("1. Decide which tool to use based on the user's request\n")
	sb.WriteString("2. Output your decision in the following format:\n\n")

	if a.enableJSON {
		sb.WriteString("**JSON Format (preferred):**\n")
		sb.WriteString("```json\n")
		sb.WriteString(`{"tool": "tool_name", "input": {"param1": "value1"}}`)
		sb.WriteString("\n```\n\n")
	}

	if a.enableReAct {
		sb.WriteString("**ReAct Format:**\n")
		sb.WriteString("Action: tool_name\n")
		sb.WriteString("Action Input: {\"param1\": \"value1\"}\n\n")
	}

	sb.WriteString("3. Wait for the tool result, then continue with your answer\n")
	sb.WriteString("4. When you have the final answer, output: FINAL_ANSWER\n")

	return sb.String()
}

// isRequired 检查工具参数是否必需
func isRequired(prop any) bool {
	m, ok := prop.(map[string]any)
	if !ok {
		return false
	}
	if required, ok := m["required"].(bool); ok && required {
		return true
	}
	return false
}

// ToolUseCall 工具调用
type ToolUseCall struct {
	ID    string
	Name  string
	Input map[string]any
}

// ToolResult 工具结果
type ToolResult struct {
	Content any
	IsError bool
}

// executeToolCall 执行工具调用
func (a *toolAdapter) executeToolCall(ctx context.Context, calls []ToolUseCall) ([]map[string]any, error) {
	if len(calls) == 0 {
		return nil, nil
	}

	var results []map[string]any
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}

		result, err := a.executor.Execute(ctx, toolruntime.Call{
			ID:    call.ID,
			Name:  name,
			Input: call.Input,
		})

		var content string
		var isError bool
		if err != nil {
			content = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			isError = result.IsError
			switch v := result.Content.(type) {
			case string:
				content = v
			case nil:
				content = "No result"
			default:
				b, _ := json.Marshal(v)
				content = string(b)
			}
		}

		results = append(results, map[string]any{
			"type":        "tool_result",
			"tool_use_id": call.ID,
			"content":     content,
			"is_error":    isError,
		})
	}

	return results, nil
}

// convertToolResultToUserMessage 将工具结果转换为用户消息
func (a *toolAdapter) convertToolResultToUserMessage(calls []ToolUseCall, results []map[string]any) string {
	if len(calls) == 0 || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Tool Results\n")

	for i, call := range calls {
		if i < len(results) {
			sb.WriteString(fmt.Sprintf("\n### %s Result:\n", call.Name))
			if content, ok := results[i]["content"].(string); ok {
				sb.WriteString(content)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// canHandleNativeTools 检查上游是否支持原生 tools
func canHandleNativeTools(model string, upstreamSupportsTools bool) bool {
	// 已知不支持 tools 的模型
	noToolModels := map[string]bool{
		"gpt-3.5-turbo":           true,
		"gpt-3.5-turbo-0301":      true,
		"gpt-3.5":                 true,
		"claude-3-haiku":          true,
		"claude-3-haiku-20240307": true,
	}

	modelLower := strings.ToLower(model)
	if noToolModels[modelLower] || noToolModels[strings.TrimPrefix(modelLower, "gpt-3.5-turbo-")] {
		return false
	}

	return upstreamSupportsTools
}

// selectBestToolMode 选择最佳的工具模式
func selectBestToolMode(upstreamSupportsTools bool, userPreference string) string {
	if upstreamSupportsTools {
		return toolEmulationNative
	}

	switch strings.ToLower(userPreference) {
	case "json":
		return toolEmulationJSON
	case "react":
		return toolEmulationReAct
	case "hybrid":
		return toolEmulationHybrid
	default:
		return toolEmulationHybrid
	}
}

// detectToolGap 检测工具缺口并记录
func (s *server) detectToolGap(reqMetadata map[string]any, toolName string, reason string) {
	if s.eventStore == nil {
		return
	}

	s.appendEvent(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: getSessionID(reqMetadata),
		RunID:     getRunID(reqMetadata),
		Data: map[string]any{
			"tool_name": toolName,
			"reason":    reason,
			"model":     getModel(reqMetadata),
			"upstream":  getUpstreamModel(reqMetadata),
		},
	})
}

func getSessionID(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["session_id"].(string); ok {
		return v
	}
	return ""
}

func getRunID(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["run_id"].(string); ok {
		return v
	}
	return ""
}

func getModel(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["model"].(string); ok {
		return v
	}
	return ""
}

func getUpstreamModel(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	if v, ok := metadata["upstream_model"].(string); ok {
		return v
	}
	return ""
}
