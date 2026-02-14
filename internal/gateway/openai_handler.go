package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/ccrun"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/runlog"
)

func (s *server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	statusCode := http.StatusOK
	errText := ""
	runID := ""
	mode := "chat"
	clientModel := ""
	requestedModel := ""
	upstreamModel := ""
	streamMode := false
	toolCount := 0
	sessionID := ""
	generatedText := ""
	defer func() {
		recordText := buildRunRecordText("/v1/chat/completions", mode, statusCode, streamMode, generatedText, errText)
		s.logRun(runlog.Entry{
			RunID:          runID,
			Path:           "/v1/chat/completions",
			Mode:           mode,
			ClientModel:    clientModel,
			RequestedModel: requestedModel,
			UpstreamModel:  upstreamModel,
			Stream:         streamMode,
			ToolCount:      toolCount,
			Status:         statusCode,
			Error:          errText,
			RecordText:     recordText,
			DurationMS:     time.Since(started).Milliseconds(),
		})
		if runID != "" {
			s.completeRunIfConfigured(runID, statusCode, errText)
		}
		if runID != "" {
			eventType := "run.completed"
			if statusCode >= 400 {
				eventType = "run.failed"
			}
			s.appendEvent(ccevent.AppendInput{
				EventType: eventType,
				SessionID: sessionID,
				RunID:     runID,
				Data: map[string]any{
					"path":        "/v1/chat/completions",
					"mode":        mode,
					"status":      statusCode,
					"error":       errText,
					"stream":      streamMode,
					"output_text": compactOutputForEvent(generatedText),
					"record_text": recordText,
				},
			})
		}
	}()

	if r.Method != http.MethodPost {
		statusCode = http.StatusMethodNotAllowed
		errText = "method not allowed"
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	var req OpenAIChatCompletionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = http.StatusBadRequest
		errText = "invalid JSON body"
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	msgReq, err := openAIChatToMessagesRequest(req)
	if err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	mode = requestMode(r, msgReq.Metadata)
	clientModel = msgReq.Model
	streamMode = msgReq.Stream
	toolCount = len(msgReq.Tools)
	sessionID = requestSessionID(r, msgReq.Metadata)
	msgReq.System = s.applySystemPromptPrefix(mode, msgReq.System)
	msgReq.Metadata = s.applyRoutingPolicy(mode, msgReq.Metadata)

	requestedModel, mappedModel, err := s.resolveUpstreamModel(mode, clientModel)
	if err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	upstreamModel = mappedModel
	msgReq.Model = mappedModel

	action := policy.Action{
		Path:      "/v1/chat/completions",
		Model:     msgReq.Model,
		Mode:      mode,
		ToolNames: toolNames(msgReq.Tools),
	}
	if err := s.policy.Authorize(r.Context(), action); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
		return
	}

	runID = s.nextID("run")
	s.createRunIfConfigured(ccrun.CreateInput{
		ID:             runID,
		SessionID:      sessionID,
		Path:           "/v1/chat/completions",
		Mode:           mode,
		ClientModel:    clientModel,
		RequestedModel: requestedModel,
		UpstreamModel:  mappedModel,
		Stream:         streamMode,
		ToolCount:      toolCount,
	})
	s.appendEvent(ccevent.AppendInput{
		EventType: "run.created",
		SessionID: sessionID,
		RunID:     runID,
		Data: map[string]any{
			"path":            "/v1/chat/completions",
			"mode":            mode,
			"client_model":    clientModel,
			"requested_model": requestedModel,
			"upstream_model":  mappedModel,
			"stream":          streamMode,
		},
	})
	w.Header().Set("request-id", runID)
	w.Header().Set("x-cc-run-id", runID)
	w.Header().Set("x-cc-mode", mode)
	w.Header().Set("x-cc-client-model", clientModel)
	w.Header().Set("x-cc-requested-model", requestedModel)
	w.Header().Set("x-cc-upstream-model", mappedModel)

	creq := toCanonicalRequest(runID, msgReq, r)
	if creq.Metadata == nil {
		creq.Metadata = map[string]any{}
	}
	creq.Metadata["mode"] = mode
	creq.Metadata["client_model"] = clientModel
	creq.Metadata["requested_model"] = requestedModel
	creq.Metadata["upstream_model"] = mappedModel

	if msgReq.Stream {
		generatedText = s.streamOpenAIChatCompletions(w, r, creq, requestedModel)
		return
	}

	resp, err := s.completeWithToolLoop(r.Context(), creq)
	if err != nil {
		statusCode = http.StatusBadGateway
		errText = err.Error()
		s.writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	generatedText = collectResponseText(resp)

	out := toOpenAIChatCompletionsResponse(s.nextID("chatcmpl"), clientModel, resp)
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) streamOpenAIChatCompletions(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) string {
	var generated strings.Builder
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return generated.String()
	}

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	streamID := s.nextID("chatcmpl")
	created := time.Now().Unix()
	events, errs := s.orchestrator.Stream(r.Context(), req)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				_ = writeOpenAISSEData(w, "[DONE]")
				flusher.Flush()
				return generated.String()
			}
			appendStreamText(&generated, ev)
			chunk := openAIChatChunkFromEvent(streamID, outwardModel, created, ev)
			if chunk == nil {
				continue
			}
			raw, _ := json.Marshal(chunk)
			if err := writeOpenAISSEData(w, string(raw)); err != nil {
				return generated.String()
			}
			flusher.Flush()
		case err, ok := <-errs:
			if !ok || err == nil {
				continue
			}
			_ = writeOpenAISSEData(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()))
			flusher.Flush()
			return generated.String()
		case <-r.Context().Done():
			return generated.String()
		}
	}
}

func (s *server) handleOpenAIResponses(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	statusCode := http.StatusOK
	errText := ""
	runID := ""
	mode := "chat"
	clientModel := ""
	requestedModel := ""
	upstreamModel := ""
	streamMode := false
	toolCount := 0
	sessionID := ""
	generatedText := ""
	defer func() {
		recordText := buildRunRecordText("/v1/responses", mode, statusCode, streamMode, generatedText, errText)
		s.logRun(runlog.Entry{
			RunID:          runID,
			Path:           "/v1/responses",
			Mode:           mode,
			ClientModel:    clientModel,
			RequestedModel: requestedModel,
			UpstreamModel:  upstreamModel,
			Stream:         streamMode,
			ToolCount:      toolCount,
			Status:         statusCode,
			Error:          errText,
			RecordText:     recordText,
			DurationMS:     time.Since(started).Milliseconds(),
		})
		if runID != "" {
			s.completeRunIfConfigured(runID, statusCode, errText)
		}
		if runID != "" {
			eventType := "run.completed"
			if statusCode >= 400 {
				eventType = "run.failed"
			}
			s.appendEvent(ccevent.AppendInput{
				EventType: eventType,
				SessionID: sessionID,
				RunID:     runID,
				Data: map[string]any{
					"path":        "/v1/responses",
					"mode":        mode,
					"status":      statusCode,
					"error":       errText,
					"stream":      streamMode,
					"output_text": compactOutputForEvent(generatedText),
					"record_text": recordText,
				},
			})
		}
	}()

	if r.Method != http.MethodPost {
		statusCode = http.StatusMethodNotAllowed
		errText = "method not allowed"
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	var req OpenAIResponsesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		statusCode = http.StatusBadRequest
		errText = "invalid JSON body"
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	msgReq, err := openAIResponsesToMessagesRequest(req)
	if err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	mode = requestMode(r, msgReq.Metadata)
	clientModel = msgReq.Model
	streamMode = msgReq.Stream
	toolCount = len(msgReq.Tools)
	sessionID = requestSessionID(r, msgReq.Metadata)
	msgReq.System = s.applySystemPromptPrefix(mode, msgReq.System)
	msgReq.Metadata = s.applyRoutingPolicy(mode, msgReq.Metadata)

	requestedModel, mappedModel, err := s.resolveUpstreamModel(mode, clientModel)
	if err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	upstreamModel = mappedModel
	msgReq.Model = mappedModel

	action := policy.Action{
		Path:      "/v1/responses",
		Model:     msgReq.Model,
		Mode:      mode,
		ToolNames: toolNames(msgReq.Tools),
	}
	if err := s.policy.Authorize(r.Context(), action); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
		return
	}

	runID = s.nextID("run")
	s.createRunIfConfigured(ccrun.CreateInput{
		ID:             runID,
		SessionID:      sessionID,
		Path:           "/v1/responses",
		Mode:           mode,
		ClientModel:    clientModel,
		RequestedModel: requestedModel,
		UpstreamModel:  mappedModel,
		Stream:         streamMode,
		ToolCount:      toolCount,
	})
	s.appendEvent(ccevent.AppendInput{
		EventType: "run.created",
		SessionID: sessionID,
		RunID:     runID,
		Data: map[string]any{
			"path":            "/v1/responses",
			"mode":            mode,
			"client_model":    clientModel,
			"requested_model": requestedModel,
			"upstream_model":  mappedModel,
			"stream":          streamMode,
		},
	})
	w.Header().Set("request-id", runID)
	w.Header().Set("x-cc-run-id", runID)
	w.Header().Set("x-cc-mode", mode)
	w.Header().Set("x-cc-client-model", clientModel)
	w.Header().Set("x-cc-requested-model", requestedModel)
	w.Header().Set("x-cc-upstream-model", mappedModel)

	creq := toCanonicalRequest(runID, msgReq, r)
	if creq.Metadata == nil {
		creq.Metadata = map[string]any{}
	}
	creq.Metadata["mode"] = mode
	creq.Metadata["client_model"] = clientModel
	creq.Metadata["requested_model"] = requestedModel
	creq.Metadata["upstream_model"] = mappedModel

	if msgReq.Stream {
		generatedText = s.streamOpenAIResponses(w, r, creq, requestedModel)
		return
	}

	resp, err := s.completeWithToolLoop(r.Context(), creq)
	if err != nil {
		statusCode = http.StatusBadGateway
		errText = err.Error()
		s.writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	generatedText = collectResponseText(resp)
	out := toOpenAIResponsesResponse(s.nextID("resp"), clientModel, resp)

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) streamOpenAIResponses(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) string {
	var generated strings.Builder
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return generated.String()
	}

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	respID := s.nextID("resp")
	created := time.Now().Unix()
	createdEvt := map[string]any{
		"type":    "response.created",
		"id":      respID,
		"model":   outwardModel,
		"created": created,
	}
	rawCreated, _ := json.Marshal(createdEvt)
	_ = writeOpenAISSEData(w, string(rawCreated))
	flusher.Flush()

	events, errs := s.orchestrator.Stream(r.Context(), req)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				completed := map[string]any{
					"type":    "response.completed",
					"id":      respID,
					"model":   outwardModel,
					"created": created,
				}
				raw, _ := json.Marshal(completed)
				_ = writeOpenAISSEData(w, string(raw))
				_ = writeOpenAISSEData(w, "[DONE]")
				flusher.Flush()
				return generated.String()
			}
			appendStreamText(&generated, ev)
			item := openAIResponseStreamEvent(respID, ev)
			if item == nil {
				continue
			}
			raw, _ := json.Marshal(item)
			if err := writeOpenAISSEData(w, string(raw)); err != nil {
				return generated.String()
			}
			flusher.Flush()
		case err, ok := <-errs:
			if !ok || err == nil {
				continue
			}
			_ = writeOpenAISSEData(w, fmt.Sprintf(`{"type":"error","error":{"message":%q}}`, err.Error()))
			flusher.Flush()
			return generated.String()
		case <-r.Context().Done():
			return generated.String()
		}
	}
}

func openAIChatToMessagesRequest(req OpenAIChatCompletionsRequest) (MessagesRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		return MessagesRequest{}, fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return MessagesRequest{}, fmt.Errorf("messages is required")
	}

	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	msgs := make([]MessageParam, 0, len(req.Messages))
	systemParts := make([]string, 0, 1)
	for _, m := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(m.Role), "system") {
			if text := openAIContentToText(m.Content); strings.TrimSpace(text) != "" {
				systemParts = append(systemParts, text)
			}
			continue
		}
		msgs = append(msgs, MessageParam{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	if len(msgs) == 0 {
		return MessagesRequest{}, fmt.Errorf("messages must include at least one non-system message")
	}
	tools := make([]ToolDefinition, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t.Type != "" && t.Type != "function" {
			return MessagesRequest{}, fmt.Errorf("unsupported tool type %q", t.Type)
		}
		tools = append(tools, ToolDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	var system any
	if len(systemParts) > 0 {
		system = strings.Join(systemParts, "\n")
	}

	return MessagesRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		System:      system,
		Messages:    msgs,
		Stream:      req.Stream,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Metadata:    mergeMetadata(req.Metadata, req.StreamOptions),
	}, nil
}

func openAIContentToText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		parts := make([]string, 0, len(c))
		for _, item := range c {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func openAIResponsesToMessagesRequest(req OpenAIResponsesRequest) (MessagesRequest, error) {
	if strings.TrimSpace(req.Model) == "" {
		return MessagesRequest{}, fmt.Errorf("model is required")
	}
	msgs, err := parseResponsesInput(req.Input)
	if err != nil {
		return MessagesRequest{}, err
	}

	maxTokens := req.MaxOutputTokens
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	tools := make([]ToolDefinition, 0, len(req.Tools))
	for _, t := range req.Tools {
		if t.Type != "" && t.Type != "function" {
			return MessagesRequest{}, fmt.Errorf("unsupported tool type %q", t.Type)
		}
		tools = append(tools, ToolDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}

	return MessagesRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Messages:  msgs,
		Stream:    req.Stream,
		Tools:     tools,
		Metadata:  req.Metadata,
	}, nil
}

func mergeMetadata(metadata map[string]any, streamOptions map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range metadata {
		out[k] = v
	}
	if len(streamOptions) > 0 {
		out["stream_options"] = streamOptions
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseResponsesInput(input any) ([]MessageParam, error) {
	switch v := input.(type) {
	case string:
		return []MessageParam{
			{Role: "user", Content: v},
		}, nil
	case []any:
		out := make([]MessageParam, 0, len(v))
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := obj["role"].(string)
			content := obj["content"]
			if strings.TrimSpace(role) == "" {
				role = "user"
			}
			out = append(out, MessageParam{
				Role:    role,
				Content: content,
			})
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("input is required")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("input must be string or array")
	}
}

func toOpenAIChatCompletionsResponse(id, outwardModel string, resp orchestrator.Response) OpenAIChatCompletionsResponse {
	content := ""
	toolCalls := make([]OpenAIToolCall, 0)
	for _, b := range resp.Blocks {
		switch b.Type {
		case "text":
			content += b.Text
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			toolCalls = append(toolCalls, OpenAIToolCall{
				ID:   b.ID,
				Type: "function",
				Function: OpenAIToolFunction{
					Name:      b.Name,
					Arguments: string(args),
				},
			})
		}
	}

	finish := "stop"
	if resp.StopReason == "tool_use" || len(toolCalls) > 0 {
		finish = "tool_calls"
	}

	return OpenAIChatCompletionsResponse{
		ID:      id,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   outwardModel,
		Choices: []OpenAIChatCompletionChoice{
			{
				Index: 0,
				Message: OpenAIChatResponseMessage{
					Role:      "assistant",
					Content:   content,
					ToolCalls: toolCalls,
				},
				FinishReason: finish,
			},
		},
		Usage: OpenAIUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func toOpenAIResponsesResponse(id, outwardModel string, resp orchestrator.Response) OpenAIResponsesResponse {
	output := make([]OpenAIResponseOutput, 0, len(resp.Blocks))
	for _, b := range resp.Blocks {
		switch b.Type {
		case "text":
			output = append(output, OpenAIResponseOutput{
				Type: "message",
				ID:   "msg_" + id,
				Role: "assistant",
				Content: []OpenAIResponseContent{
					{Type: "output_text", Text: b.Text},
				},
			})
		case "tool_use":
			args, _ := json.Marshal(b.Input)
			output = append(output, OpenAIResponseOutput{
				Type:   "function_call",
				ID:     b.ID,
				Name:   b.Name,
				CallID: b.ID,
				Args:   string(args),
			})
		}
	}
	if len(output) == 0 {
		output = append(output, OpenAIResponseOutput{
			Type: "message",
			ID:   "msg_" + id,
			Role: "assistant",
			Content: []OpenAIResponseContent{
				{Type: "output_text", Text: ""},
			},
		})
	}

	return OpenAIResponsesResponse{
		ID:      id,
		Object:  "response",
		Created: time.Now().Unix(),
		Model:   outwardModel,
		Status:  "completed",
		Output:  output,
		Usage: OpenAIUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}

func openAIChatChunkFromEvent(streamID, outwardModel string, created int64, ev orchestrator.StreamEvent) map[string]any {
	base := map[string]any{
		"id":      streamID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   outwardModel,
	}

	switch ev.Type {
	case "message_start":
		base["choices"] = []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
				},
				"finish_reason": nil,
			},
		}
		return base
	case "content_block_delta":
		delta := map[string]any{}
		if ev.DeltaJSON != "" {
			delta["tool_calls"] = []map[string]any{
				{
					"index": 0,
					"function": map[string]any{
						"arguments": ev.DeltaJSON,
					},
					"type": "function",
				},
			}
		} else {
			delta["content"] = ev.DeltaText
		}
		base["choices"] = []map[string]any{
			{
				"index":         0,
				"delta":         delta,
				"finish_reason": nil,
			},
		}
		return base
	case "message_delta":
		finish := "stop"
		if ev.StopReason == "tool_use" {
			finish = "tool_calls"
		}
		base["choices"] = []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": finish,
			},
		}
		return base
	default:
		return nil
	}
}

func openAIResponseStreamEvent(respID string, ev orchestrator.StreamEvent) map[string]any {
	switch ev.Type {
	case "content_block_delta":
		if ev.DeltaJSON != "" {
			return map[string]any{
				"type":        "response.function_call_arguments.delta",
				"response_id": respID,
				"delta":       ev.DeltaJSON,
			}
		}
		return map[string]any{
			"type":        "response.output_text.delta",
			"response_id": respID,
			"delta":       ev.DeltaText,
		}
	default:
		return nil
	}
}

func writeOpenAISSEData(w http.ResponseWriter, data string) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}
