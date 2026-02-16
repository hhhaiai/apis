package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/toolruntime"
)

const (
	toolLoopModeClient = "client_loop"
	toolLoopModeServer = "server_loop"

	toolEmulationNative = "native"
	toolEmulationJSON   = "json"
	toolEmulationReAct  = "react"
	toolEmulationHybrid = "hybrid"
)

var jsonFenceRE = regexp.MustCompile("(?is)```(?:json)?\\s*([\\[{].*?[\\]}])\\s*```")

type toolLoopConfig struct {
	enabled       bool
	maxSteps      int
	emulationMode string
	plannerModel  string
}

func (s *server) completeWithToolLoop(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	cfg := toolLoopConfigFromMetadata(req.Metadata)
	if !cfg.enabled || len(req.Tools) == 0 {
		return s.orchestrator.Complete(ctx, req)
	}

	working := req
	working.Messages = append([]orchestrator.Message(nil), req.Messages...)
	allowedTools := allowedToolNames(req.Tools)
	totalUsage := orchestrator.Usage{}
	executedTools := false
	var last orchestrator.Response

	for i := 0; i < cfg.maxSteps; i++ {
		callReq := working
		callReq.Model = planningModel(req.Model, cfg.plannerModel)
		callReq.System = withToolEmulationSystem(req.System, cfg.emulationMode, req.Tools)

		resp, err := s.orchestrator.Complete(ctx, callReq)
		if err != nil {
			return orchestrator.Response{}, err
		}
		totalUsage.InputTokens += resp.Usage.InputTokens
		totalUsage.OutputTokens += resp.Usage.OutputTokens
		last = resp

		toolBlocks := toolUseBlocks(resp.Blocks)
		parsedBy := ""
		if len(toolBlocks) == 0 {
			toolBlocks, parsedBy = emulatedToolUseBlocks(resp.Blocks, cfg.emulationMode)
			if len(toolBlocks) > 0 {
				s.appendToolEmulationEvent(working, cfg.emulationMode, parsedBy, toolBlocks)
			}
		}
		if len(toolBlocks) == 0 {
			if executedTools && shouldFinalizeWithPrimaryModel(req.Model, cfg.plannerModel) {
				finalReq := working
				finalReq.Model = req.Model
				finalReq.System = req.System
				finalResp, err := s.orchestrator.Complete(ctx, finalReq)
				if err != nil {
					return orchestrator.Response{}, err
				}
				totalUsage.InputTokens += finalResp.Usage.InputTokens
				totalUsage.OutputTokens += finalResp.Usage.OutputTokens
				finalResp.Usage = totalUsage
				return finalResp, nil
			}
			last.Usage = totalUsage
			return last, nil
		}

		executedTools = true
		assistantBlocks := resp.Blocks
		if parsedBy != "" {
			assistantBlocks = toolBlocks
		}
		working.Messages = append(working.Messages, orchestrator.Message{
			Role:    "assistant",
			Content: assistantBlocksToContent(assistantBlocks),
		})
		working.Messages = append(working.Messages, orchestrator.Message{
			Role:    "user",
			Content: s.executeToolBlocks(ctx, working, toolBlocks, allowedTools),
		})
	}

	last.StopReason = "max_turns"
	last.Usage = totalUsage
	return last, nil
}

func planningModel(primaryModel, plannerModel string) string {
	primaryModel = strings.TrimSpace(primaryModel)
	plannerModel = strings.TrimSpace(plannerModel)
	if plannerModel == "" {
		return primaryModel
	}
	return plannerModel
}

func shouldFinalizeWithPrimaryModel(primaryModel, plannerModel string) bool {
	primaryModel = strings.TrimSpace(primaryModel)
	plannerModel = strings.TrimSpace(plannerModel)
	if plannerModel == "" || primaryModel == "" {
		return false
	}
	return !strings.EqualFold(primaryModel, plannerModel)
}

func toolLoopConfigFromMetadata(metadata map[string]any) toolLoopConfig {
	cfg := toolLoopConfig{
		enabled:       false,
		maxSteps:      4,
		emulationMode: toolEmulationNative,
	}
	mode := ""
	if metadata != nil {
		if v, ok := metadata["tool_loop_mode"]; ok {
			if text, ok := v.(string); ok {
				mode = strings.ToLower(strings.TrimSpace(text))
			}
		}
	}
	switch mode {
	case "server", toolLoopModeServer:
		cfg.enabled = true
	case toolEmulationNative, toolEmulationJSON, toolEmulationReAct, toolEmulationHybrid:
		cfg.enabled = true
		cfg.emulationMode = mode
	case "", toolLoopModeClient:
		cfg.enabled = false
	default:
		cfg.enabled = false
	}
	if metadata != nil {
		if v, ok := metadata["tool_loop_max_steps"]; ok {
			if n, ok := parseInt(v); ok && n > 0 {
				cfg.maxSteps = n
			}
		}
		if text := stringFromAny(metadata["tool_emulation_mode"]); text != "" {
			cfg.emulationMode = normalizeToolEmulationMode(text)
		}
		if text := stringFromAny(metadata["tool_planner_model"]); text != "" {
			cfg.plannerModel = text
		}
	}
	cfg.emulationMode = normalizeToolEmulationMode(cfg.emulationMode)
	if !cfg.enabled && mode == "" && cfg.emulationMode != toolEmulationNative {
		cfg.enabled = true
	}
	if cfg.maxSteps <= 0 {
		cfg.maxSteps = 4
	}
	return cfg
}

func normalizeToolEmulationMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case toolEmulationJSON, toolEmulationReAct, toolEmulationHybrid, toolEmulationNative:
		return mode
	default:
		return toolEmulationNative
	}
}

func withToolEmulationSystem(system any, mode string, tools []orchestrator.Tool) any {
	mode = normalizeToolEmulationMode(mode)
	if mode == toolEmulationNative {
		return system
	}
	base := strings.TrimSpace(systemToText(system))
	if strings.Contains(base, "[CC_TOOL_EMULATION]") {
		return base
	}
	toolNames := make([]string, 0, len(tools))
	for _, t := range tools {
		name := strings.TrimSpace(t.Name)
		if name != "" {
			toolNames = append(toolNames, name)
		}
	}
	instruction := buildToolEmulationInstruction(mode, toolNames)
	if base == "" {
		return instruction
	}
	return base + "\n\n" + instruction
}

func buildToolEmulationInstruction(mode string, toolNames []string) string {
	names := strings.Join(toolNames, ", ")
	if names == "" {
		names = "(none)"
	}
	switch mode {
	case toolEmulationJSON:
		return "[CC_TOOL_EMULATION]\nWhen you need a tool, output only JSON: {\"tool\":\"<name>\",\"input\":{...}}.\nNever wrap JSON with markdown.\nAvailable tools: " + names
	case toolEmulationReAct:
		return "[CC_TOOL_EMULATION]\nWhen you need a tool, use exact format:\nAction: <tool_name>\nAction Input: <json object>\nAvailable tools: " + names
	case toolEmulationHybrid:
		return "[CC_TOOL_EMULATION]\nWhen you need a tool, prefer JSON: {\"tool\":\"<name>\",\"input\":{...}}. If not possible, use:\nAction: <tool_name>\nAction Input: <json object>\nAvailable tools: " + names
	default:
		return ""
	}
}

func emulatedToolUseBlocks(blocks []orchestrator.AssistantBlock, mode string) ([]orchestrator.AssistantBlock, string) {
	mode = normalizeToolEmulationMode(mode)
	if mode == toolEmulationNative {
		return nil, ""
	}
	text := collectAssistantText(blocks)
	switch mode {
	case toolEmulationJSON:
		return withEmulatedCallIDs(parseJSONToolUseBlocks(text)), "json"
	case toolEmulationReAct:
		return withEmulatedCallIDs(parseReActToolUseBlocks(text)), "react"
	case toolEmulationHybrid:
		if calls := withEmulatedCallIDs(parseJSONToolUseBlocks(text)); len(calls) > 0 {
			return calls, "json"
		}
		return withEmulatedCallIDs(parseReActToolUseBlocks(text)), "react"
	default:
		return nil, ""
	}
}

func withEmulatedCallIDs(calls []orchestrator.AssistantBlock) []orchestrator.AssistantBlock {
	out := make([]orchestrator.AssistantBlock, 0, len(calls))
	for i, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		call.Type = "tool_use"
		if strings.TrimSpace(call.ID) == "" {
			call.ID = fmt.Sprintf("toolu_emu_%d", i+1)
		}
		if call.Input == nil {
			call.Input = map[string]any{}
		}
		out = append(out, call)
	}
	return out
}

func collectAssistantText(blocks []orchestrator.AssistantBlock) string {
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Type != "text" {
			continue
		}
		text := strings.TrimSpace(b.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func parseJSONToolUseBlocks(text string) []orchestrator.AssistantBlock {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	for _, raw := range collectJSONCandidates(text) {
		calls := parseToolCallsFromJSON(raw)
		if len(calls) > 0 {
			return calls
		}
	}
	return nil
}

func collectJSONCandidates(text string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	matches := jsonFenceRE.FindAllStringSubmatch(text, 8)
	for _, m := range matches {
		if len(m) > 1 {
			add(m[1])
		}
	}
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		add(trimmed)
	}
	for _, candidate := range extractJSONObjectCandidates(text, 8) {
		add(candidate)
	}
	return out
}

func extractJSONObjectCandidates(text string, limit int) []string {
	if limit <= 0 {
		limit = 8
	}
	out := make([]string, 0, limit)
	start := -1
	depth := 0
	inString := false
	escaped := false

	for i := 0; i < len(text) && len(out) < limit; i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}
		switch ch {
		case '"':
			inString = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth <= 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, text[start:i+1])
				start = -1
			}
		}
	}
	return out
}

func parseToolCallsFromJSON(raw string) []orchestrator.AssistantBlock {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return flattenToolCalls(payload)
}

func flattenToolCalls(payload any) []orchestrator.AssistantBlock {
	switch v := payload.(type) {
	case []any:
		out := make([]orchestrator.AssistantBlock, 0, len(v))
		for _, item := range v {
			out = append(out, flattenToolCalls(item)...)
		}
		return out
	case map[string]any:
		if arr, ok := v["tool_calls"].([]any); ok {
			out := make([]orchestrator.AssistantBlock, 0, len(arr))
			for _, item := range arr {
				out = append(out, flattenToolCalls(item)...)
			}
			if len(out) > 0 {
				return out
			}
		}
		if call, ok := mapToToolUse(v); ok {
			return []orchestrator.AssistantBlock{call}
		}
		return nil
	default:
		return nil
	}
}

func mapToToolUse(obj map[string]any) (orchestrator.AssistantBlock, bool) {
	name := firstStringFromMap(obj, "tool", "name", "action", "tool_name")
	callID := firstStringFromMap(obj, "id", "call_id", "tool_call_id")

	var input any
	if fn, ok := obj["function"].(map[string]any); ok {
		if name == "" {
			name = firstStringFromMap(fn, "name", "tool")
		}
		if args, ok := fn["arguments"]; ok {
			input = args
		}
	}
	for _, key := range []string{"input", "arguments", "args", "parameters", "action_input"} {
		if input != nil {
			break
		}
		if v, ok := obj[key]; ok {
			input = v
		}
	}

	name = strings.TrimSpace(name)
	switch strings.ToLower(name) {
	case "", "final", "final_answer", "answer", "none":
		return orchestrator.AssistantBlock{}, false
	}

	return orchestrator.AssistantBlock{
		Type:  "tool_use",
		ID:    callID,
		Name:  name,
		Input: normalizeToolInput(input),
	}, true
}

func firstStringFromMap(m map[string]any, keys ...string) string {
	for _, key := range keys {
		v, ok := m[key]
		if !ok {
			continue
		}
		if text, ok := v.(string); ok {
			text = strings.TrimSpace(text)
			if text != "" {
				return text
			}
		}
	}
	return ""
}

func normalizeToolInput(raw any) map[string]any {
	switch v := raw.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		return v
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return map[string]any{}
		}
		var decoded any
		if err := json.Unmarshal([]byte(v), &decoded); err == nil {
			return normalizeToolInput(decoded)
		}
		return map[string]any{"value": v}
	case []any:
		return map[string]any{"items": v}
	default:
		return map[string]any{"value": v}
	}
}

func parseReActToolUseBlocks(text string) []orchestrator.AssistantBlock {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	out := make([]orchestrator.AssistantBlock, 0, 2)

	for i := 0; i < len(lines); i++ {
		name, ok := parseFieldValue(lines[i], "action")
		if !ok {
			continue
		}
		if isTerminalReActAction(name) {
			continue
		}

		inputRaw := ""
		nextIdx := i
		for j := i + 1; j < len(lines); j++ {
			if value, ok := parseFieldValue(lines[j], "action input"); ok {
				inputRaw = value
				nextIdx = j
				if strings.TrimSpace(inputRaw) == "" {
					var idx int
					inputRaw, idx = collectReActMultilineInput(lines, j+1)
					nextIdx = idx
				}
				break
			}
			if _, ok := parseFieldValue(lines[j], "action"); ok {
				nextIdx = j - 1
				break
			}
			if _, ok := parseFieldValue(lines[j], "final answer"); ok {
				nextIdx = j - 1
				break
			}
		}
		out = append(out, orchestrator.AssistantBlock{
			Type:  "tool_use",
			Name:  strings.TrimSpace(name),
			Input: normalizeToolInput(inputRaw),
		})
		i = nextIdx
	}
	return out
}

func parseFieldValue(line, field string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return "", false
	}
	idx := strings.Index(trimmed, ":")
	if idx <= 0 {
		return "", false
	}
	left := strings.ToLower(strings.TrimSpace(trimmed[:idx]))
	if left != strings.ToLower(strings.TrimSpace(field)) {
		return "", false
	}
	return strings.TrimSpace(trimmed[idx+1:]), true
}

func isTerminalReActAction(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "final", "final_answer", "answer", "none":
		return true
	default:
		return false
	}
}

func collectReActMultilineInput(lines []string, start int) (string, int) {
	if start < 0 {
		return "", 0
	}
	collected := make([]string, 0, 4)
	depth := 0
	startedJSON := false
	last := start

	for i := start; i < len(lines); i++ {
		if len(collected) > 0 {
			if _, ok := parseFieldValue(lines[i], "observation"); ok {
				last = i - 1
				break
			}
			if _, ok := parseFieldValue(lines[i], "thought"); ok {
				last = i - 1
				break
			}
			if _, ok := parseFieldValue(lines[i], "action"); ok {
				last = i - 1
				break
			}
			if _, ok := parseFieldValue(lines[i], "final answer"); ok {
				last = i - 1
				break
			}
		}
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" && !startedJSON && len(collected) == 0 {
			last = i
			continue
		}
		collected = append(collected, lines[i])
		last = i
		depth += strings.Count(lines[i], "{")
		depth -= strings.Count(lines[i], "}")
		if strings.Contains(lines[i], "{") {
			startedJSON = true
		}
		if startedJSON && depth <= 0 {
			break
		}
		if !startedJSON && len(collected) >= 1 {
			break
		}
	}
	return strings.TrimSpace(strings.Join(collected, "\n")), last
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

func (s *server) executeToolBlocks(ctx context.Context, req orchestrator.Request, calls []orchestrator.AssistantBlock, allowed map[string]struct{}) []any {
	out := make([]any, 0, len(calls))
	aliases := toolAliasesFromMetadata(req.Metadata)
	for _, call := range calls {
		originalName := strings.ToLower(strings.TrimSpace(call.Name))
		name := originalName
		callID := strings.TrimSpace(call.ID)
		if callID == "" {
			callID = "toolu_auto"
		}
		if mapped, ok := aliases[originalName]; ok {
			if _, declared := allowed[mapped]; declared {
				name = mapped
				s.appendToolAliasEvent(req, originalName, mapped, call.Input)
			}
		}
		if _, ok := allowed[name]; !ok {
			s.appendToolGapEvent(req, call.Name, call.Input, "tool_not_declared")
			out = append(out, toolResultBlock(callID, "tool is not declared in request tools", true))
			continue
		}

		result, err := s.toolExecutor.Execute(ctx, toolruntime.Call{
			ID:    callID,
			Name:  name,
			Input: call.Input,
		})
		if err != nil {
			reason := "tool_execution_error"
			if errors.Is(err, toolruntime.ErrToolNotImplemented) {
				reason = "tool_not_implemented"
			}
			s.appendToolGapEvent(req, call.Name, call.Input, reason)
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

func (s *server) appendToolEmulationEvent(req orchestrator.Request, emulationMode, parser string, calls []orchestrator.AssistantBlock) {
	if len(calls) == 0 {
		return
	}
	sessionID := ""
	mode := ""
	path := ""
	if req.Metadata != nil {
		sessionID = stringFromAny(req.Metadata["session_id"])
		mode = stringFromAny(req.Metadata["mode"])
		path = stringFromAny(req.Metadata["request_path"])
	}
	if strings.TrimSpace(path) == "" {
		path = "/v1/messages"
	}
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	s.appendEvent(ccevent.AppendInput{
		EventType: "tool.emulated_call",
		SessionID: sessionID,
		RunID:     req.RunID,
		Data: map[string]any{
			"path":           path,
			"mode":           mode,
			"emulation_mode": normalizeToolEmulationMode(emulationMode),
			"parser":         strings.TrimSpace(parser),
			"count":          len(calls),
			"tools":          names,
		},
	})
}

func (s *server) appendToolGapEvent(req orchestrator.Request, toolName string, input map[string]any, reason string) {
	sessionID := ""
	mode := ""
	path := ""
	clientModel := ""
	requestedModel := ""
	upstreamModel := ""
	emulationMode := ""
	plannerModel := ""
	if req.Metadata != nil {
		sessionID = stringFromAny(req.Metadata["session_id"])
		mode = stringFromAny(req.Metadata["mode"])
		path = stringFromAny(req.Metadata["request_path"])
		clientModel = stringFromAny(req.Metadata["client_model"])
		requestedModel = stringFromAny(req.Metadata["requested_model"])
		upstreamModel = stringFromAny(req.Metadata["upstream_model"])
		emulationMode = stringFromAny(req.Metadata["tool_emulation_mode"])
		plannerModel = stringFromAny(req.Metadata["tool_planner_model"])
	}
	if strings.TrimSpace(path) == "" {
		path = "/v1/messages"
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: sessionID,
		RunID:     req.RunID,
		Data: map[string]any{
			"path":            path,
			"mode":            mode,
			"name":            strings.TrimSpace(toolName),
			"reason":          strings.TrimSpace(reason),
			"client_model":    clientModel,
			"requested_model": requestedModel,
			"upstream_model":  upstreamModel,
			"emulation_mode":  normalizeToolEmulationMode(emulationMode),
			"planner_model":   plannerModel,
			"input":           input,
		},
	})
}

func (s *server) appendToolAliasEvent(req orchestrator.Request, fromName, toName string, input map[string]any) {
	fromName = strings.TrimSpace(fromName)
	toName = strings.TrimSpace(toName)
	if fromName == "" || toName == "" || strings.EqualFold(fromName, toName) {
		return
	}
	sessionID := ""
	mode := ""
	path := ""
	if req.Metadata != nil {
		sessionID = stringFromAny(req.Metadata["session_id"])
		mode = stringFromAny(req.Metadata["mode"])
		path = stringFromAny(req.Metadata["request_path"])
	}
	if strings.TrimSpace(path) == "" {
		path = "/v1/messages"
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: "tool.alias_applied",
		SessionID: sessionID,
		RunID:     req.RunID,
		Data: map[string]any{
			"path":  path,
			"mode":  mode,
			"from":  fromName,
			"to":    toName,
			"input": input,
		},
	})
}

func toolAliasesFromMetadata(metadata map[string]any) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["tool_aliases"]
	if !ok || raw == nil {
		return nil
	}
	out := map[string]string{}
	add := func(k, v string) {
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.ToLower(strings.TrimSpace(v))
		if k == "" || v == "" {
			return
		}
		out[k] = v
	}
	switch aliases := raw.(type) {
	case map[string]string:
		for k, v := range aliases {
			add(k, v)
		}
	case map[string]any:
		for k, v := range aliases {
			add(k, stringFromAny(v))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	default:
		return ""
	}
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
