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
	if err := decodeJSONBodySingle(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
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
	if err := s.enforceTokenModelAccess(r.Context(), msgReq.Model); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
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
	msgReq.Metadata = s.applyChannelRoutePolicy(r.Context(), msgReq.Metadata, mappedModel)

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
	creq.Metadata["session_id"] = sessionID
	creq.Metadata["request_path"] = "/v1/chat/completions"
	creq.Metadata["client_model"] = clientModel
	creq.Metadata["requested_model"] = requestedModel
	creq.Metadata["upstream_model"] = mappedModel
	reservedQuota := estimateReservedQuota(msgReq.MaxTokens, msgReq.System, msgReq.Messages)
	if err := s.reserveQuotaFromRequestContext(r.Context(), reservedQuota); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "quota_error", err.Error())
		return
	}

	if msgReq.Stream {
		creq = s.applyVisionFallback(r.Context(), creq)
		creq = s.applyToolSupportFallback(creq)
		var usage orchestrator.Usage
		if s.shouldStreamWithToolLoop(creq) {
			generatedText, usage = s.streamOpenAIChatCompletionsWithToolLoop(w, r, creq, requestedModel)
		} else {
			generatedText, usage = s.streamOpenAIChatCompletions(w, r, creq, requestedModel)
		}
		if err := s.settleQuotaFromRequestContext(r.Context(), reservedQuota, usageToQuotaAmount(usage.InputTokens, usage.OutputTokens)); err != nil {
			statusCode = http.StatusForbidden
			errText = err.Error()
		}
		return
	}

	creq = s.applyVisionFallback(r.Context(), creq)
	creq = s.applyToolSupportFallback(creq)
	resp, err := s.completeWithToolLoop(r.Context(), creq)
	if err != nil {
		_ = s.refundQuotaFromRequestContext(r.Context(), reservedQuota)
		statusCode = http.StatusBadGateway
		errText = err.Error()
		s.writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	generatedText = collectResponseText(resp)
	if err := s.settleQuotaFromRequestContext(r.Context(), reservedQuota, usageToQuotaAmount(resp.Usage.InputTokens, resp.Usage.OutputTokens)); err != nil {
		_ = s.refundQuotaFromRequestContext(r.Context(), reservedQuota)
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "quota_error", err.Error())
		return
	}

	out := toOpenAIChatCompletionsResponse(s.nextID("chatcmpl"), clientModel, resp)
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) streamOpenAIChatCompletions(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) (string, orchestrator.Usage) {
	var generated strings.Builder
	var usage orchestrator.Usage
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return generated.String(), usage
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
				return generated.String(), usage
			}
			appendStreamText(&generated, ev)
			if ev.Usage.InputTokens > 0 || ev.Usage.OutputTokens > 0 {
				usage = ev.Usage
			}
			chunk := openAIChatChunkFromEvent(streamID, outwardModel, created, ev)
			if chunk == nil {
				continue
			}
			raw, _ := json.Marshal(chunk)
			if err := writeOpenAISSEData(w, string(raw)); err != nil {
				return generated.String(), usage
			}
			flusher.Flush()
		case err, ok := <-errs:
			if !ok || err == nil {
				continue
			}
			_ = writeOpenAISSEData(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()))
			flusher.Flush()
			return generated.String(), usage
		case <-r.Context().Done():
			return generated.String(), usage
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
	if err := decodeJSONBodySingle(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
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
	if err := s.enforceTokenModelAccess(r.Context(), msgReq.Model); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
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
	msgReq.Metadata = s.applyChannelRoutePolicy(r.Context(), msgReq.Metadata, mappedModel)

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
	creq.Metadata["session_id"] = sessionID
	creq.Metadata["request_path"] = "/v1/responses"
	creq.Metadata["client_model"] = clientModel
	creq.Metadata["requested_model"] = requestedModel
	creq.Metadata["upstream_model"] = mappedModel
	reservedQuota := estimateReservedQuota(msgReq.MaxTokens, msgReq.System, msgReq.Messages)
	if err := s.reserveQuotaFromRequestContext(r.Context(), reservedQuota); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "quota_error", err.Error())
		return
	}

	if msgReq.Stream {
		creq = s.applyVisionFallback(r.Context(), creq)
		creq = s.applyToolSupportFallback(creq)
		var usage orchestrator.Usage
		if s.shouldStreamWithToolLoop(creq) {
			generatedText, usage = s.streamOpenAIResponsesWithToolLoop(w, r, creq, requestedModel)
		} else {
			generatedText, usage = s.streamOpenAIResponses(w, r, creq, requestedModel)
		}
		if err := s.settleQuotaFromRequestContext(r.Context(), reservedQuota, usageToQuotaAmount(usage.InputTokens, usage.OutputTokens)); err != nil {
			statusCode = http.StatusForbidden
			errText = err.Error()
		}
		return
	}

	creq = s.applyVisionFallback(r.Context(), creq)
	creq = s.applyToolSupportFallback(creq)
	resp, err := s.completeWithToolLoop(r.Context(), creq)
	if err != nil {
		_ = s.refundQuotaFromRequestContext(r.Context(), reservedQuota)
		statusCode = http.StatusBadGateway
		errText = err.Error()
		s.writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	generatedText = collectResponseText(resp)
	if err := s.settleQuotaFromRequestContext(r.Context(), reservedQuota, usageToQuotaAmount(resp.Usage.InputTokens, resp.Usage.OutputTokens)); err != nil {
		_ = s.refundQuotaFromRequestContext(r.Context(), reservedQuota)
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "quota_error", err.Error())
		return
	}
	out := toOpenAIResponsesResponse(s.nextID("resp"), clientModel, resp)

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) streamOpenAIResponses(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) (string, orchestrator.Usage) {
	var generated strings.Builder
	var usage orchestrator.Usage
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return generated.String(), usage
	}

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	respID := s.nextID("resp")
	created := time.Now().Unix()
	itemID := "msg_" + respID
	textItemStarted := false
	var activeTool *openAIResponseFunctionCallState
	createdEvt := openAIResponseCreatedEvent(respID, outwardModel, created)
	rawCreated, _ := json.Marshal(createdEvt)
	_ = writeOpenAISSEData(w, string(rawCreated))
	flusher.Flush()

	events, errs := s.orchestrator.Stream(r.Context(), req)
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if activeTool != nil {
					raw, _ := json.Marshal(openAIResponseFunctionCallDoneEvent(activeTool))
					_ = writeOpenAISSEData(w, string(raw))
					activeTool = nil
				}
				if textItemStarted {
					for _, item := range openAIResponseTextDoneEvents(respID, itemID, generated.String()) {
						raw, _ := json.Marshal(item)
						_ = writeOpenAISSEData(w, string(raw))
					}
				}
				completed := openAIResponseCompletedEvent(respID, outwardModel, created, generated.String(), usage)
				raw, _ := json.Marshal(completed)
				_ = writeOpenAISSEData(w, string(raw))
				_ = writeOpenAISSEData(w, "[DONE]")
				flusher.Flush()
				return generated.String(), usage
			}
			appendStreamText(&generated, ev)
			if ev.Usage.InputTokens > 0 || ev.Usage.OutputTokens > 0 {
				usage = ev.Usage
			}
			if ev.Type == "content_block_start" && ev.Block.Type == "tool_use" {
				activeTool = newOpenAIResponseFunctionCallState(respID, ev)
				raw, _ := json.Marshal(openAIResponseFunctionCallAddedEvent(activeTool))
				if err := writeOpenAISSEData(w, string(raw)); err != nil {
					return generated.String(), usage
				}
				flusher.Flush()
				continue
			}
			if ev.Type == "content_block_delta" && ev.DeltaJSON != "" {
				if activeTool == nil {
					activeTool = newOpenAIResponseFunctionCallState(respID, ev)
					raw, _ := json.Marshal(openAIResponseFunctionCallAddedEvent(activeTool))
					if err := writeOpenAISSEData(w, string(raw)); err != nil {
						return generated.String(), usage
					}
				}
				activeTool.Arguments.WriteString(ev.DeltaJSON)
				raw, _ := json.Marshal(openAIResponseFunctionCallArgumentsDeltaEvent(respID, activeTool, ev.DeltaJSON))
				if err := writeOpenAISSEData(w, string(raw)); err != nil {
					return generated.String(), usage
				}
				flusher.Flush()
				continue
			}
			if ev.Type == "content_block_stop" && activeTool != nil {
				raw, _ := json.Marshal(openAIResponseFunctionCallDoneEvent(activeTool))
				if err := writeOpenAISSEData(w, string(raw)); err != nil {
					return generated.String(), usage
				}
				activeTool = nil
				flusher.Flush()
				continue
			}
			if ev.Type == "content_block_delta" && ev.DeltaText != "" && !textItemStarted {
				for _, startItem := range openAIResponseTextStartEvents(itemID) {
					raw, _ := json.Marshal(startItem)
					if err := writeOpenAISSEData(w, string(raw)); err != nil {
						return generated.String(), usage
					}
				}
				textItemStarted = true
			}
			item := openAIResponseStreamEvent(respID, itemID, ev)
			if item == nil {
				continue
			}
			raw, _ := json.Marshal(item)
			if err := writeOpenAISSEData(w, string(raw)); err != nil {
				return generated.String(), usage
			}
			flusher.Flush()
		case err, ok := <-errs:
			if !ok || err == nil {
				continue
			}
			failed := openAIResponseFailedEvent(respID, outwardModel, created, err)
			raw, _ := json.Marshal(failed)
			_ = writeOpenAISSEData(w, string(raw))
			_ = writeOpenAISSEData(w, fmt.Sprintf(`{"type":"error","error":{"message":%q}}`, err.Error()))
			_ = writeOpenAISSEData(w, "[DONE]")
			flusher.Flush()
			return generated.String(), usage
		case <-r.Context().Done():
			return generated.String(), usage
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
		maxTokens = req.MaxCompletionTokens
	}
	if maxTokens <= 0 {
		maxTokens = 1024
	}

	msgs := make([]MessageParam, 0, len(req.Messages))
	systemParts := make([]string, 0, 1)
	for i, m := range req.Messages {
		converted, systemText, err := openAIChatMessageToMessageParams(m)
		if err != nil {
			return MessagesRequest{}, fmt.Errorf("messages[%d]: %w", i, err)
		}
		if strings.TrimSpace(systemText) != "" {
			systemParts = append(systemParts, systemText)
		}
		msgs = append(msgs, converted...)
	}
	if len(msgs) == 0 {
		return MessagesRequest{}, fmt.Errorf("messages must include at least one non-system message")
	}
	tools, err := openAIToolsToToolDefinitions(req.Tools)
	if err != nil {
		return MessagesRequest{}, err
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
		Metadata:    mergeChatMetadata(req),
	}, nil
}

func openAIChatMessageToMessageParams(m OpenAIChatMessage) ([]MessageParam, string, error) {
	role := strings.ToLower(strings.TrimSpace(m.Role))
	switch role {
	case "system":
		return nil, openAIContentToText(m.Content), nil
	case "tool":
		toolCallID := strings.TrimSpace(m.ToolCallID)
		if toolCallID == "" {
			return nil, "", fmt.Errorf("tool message missing tool_call_id")
		}
		return []MessageParam{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": toolCallID,
						"content":     openAIContentToToolResult(m.Content),
					},
				},
			},
		}, "", nil
	case "assistant":
		if len(m.ToolCalls) == 0 {
			if m.Content == nil {
				return []MessageParam{{Role: "assistant", Content: ""}}, "", nil
			}
			return []MessageParam{{Role: "assistant", Content: m.Content}}, "", nil
		}
		blocks := make([]any, 0, 1+len(m.ToolCalls))
		if text := openAIContentToText(m.Content); strings.TrimSpace(text) != "" {
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": text,
			})
		}
		for i, tc := range m.ToolCalls {
			name := strings.TrimSpace(tc.Function.Name)
			if name == "" {
				return nil, "", fmt.Errorf("assistant tool_call[%d] missing function name", i)
			}
			callID := strings.TrimSpace(tc.ID)
			if callID == "" {
				callID = fmt.Sprintf("toolu_%d", i+1)
			}
			blocks = append(blocks, map[string]any{
				"type":  "tool_use",
				"id":    callID,
				"name":  name,
				"input": parseOpenAIToolArguments(tc.Function.Arguments),
			})
		}
		if len(blocks) == 0 {
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": "",
			})
		}
		return []MessageParam{{Role: "assistant", Content: blocks}}, "", nil
	default:
		if m.Content == nil {
			return []MessageParam{{Role: m.Role, Content: ""}}, "", nil
		}
		return []MessageParam{{Role: m.Role, Content: m.Content}}, "", nil
	}
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

func openAIContentToToolResult(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		return openAIContentToText(c)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", c)
	}
}

func parseOpenAIToolArguments(arguments string) map[string]any {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return map[string]any{}
	}
	var decoded any
	if err := json.Unmarshal([]byte(arguments), &decoded); err != nil {
		return map[string]any{
			"_raw": arguments,
		}
	}
	if obj, ok := decoded.(map[string]any); ok {
		return obj
	}
	return map[string]any{
		"value": decoded,
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

	tools, err := openAIToolsToToolDefinitions(req.Tools)
	if err != nil {
		return MessagesRequest{}, err
	}
	metadata := mergeResponsesMetadata(req)

	return MessagesRequest{
		Model:       req.Model,
		MaxTokens:   maxTokens,
		System:      req.Instructions,
		Messages:    msgs,
		Stream:      req.Stream,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Metadata:    metadata,
	}, nil
}

func openAIToolsToToolDefinitions(openAITools []OpenAIChatTool) ([]ToolDefinition, error) {
	tools := make([]ToolDefinition, 0, len(openAITools))
	for _, t := range openAITools {
		toolType := strings.ToLower(strings.TrimSpace(t.Type))
		switch toolType {
		case "", "function":
			name := strings.TrimSpace(t.Function.Name)
			description := t.Function.Description
			parameters := t.Function.Parameters
			if name == "" {
				name = t.Name
				description = t.Description
				parameters = t.Parameters
			}
			tools = append(tools, ToolDefinition{
				Name:        name,
				Description: description,
				InputSchema: ensureObjectSchema(parameters),
			})
		case "namespace":
			namespace := strings.TrimSpace(t.Name)
			for _, child := range t.Tools {
				childType := strings.ToLower(strings.TrimSpace(child.Type))
				if childType != "" && childType != "function" {
					return nil, fmt.Errorf("unsupported namespace tool type %q", child.Type)
				}
				description := strings.TrimSpace(child.Description)
				if description == "" {
					description = strings.TrimSpace(t.Description)
				} else if nsDesc := strings.TrimSpace(t.Description); nsDesc != "" {
					description = nsDesc + "\n\n" + description
				}
				tools = append(tools, ToolDefinition{
					Name:        openAINamespaceToolName(namespace, child.Name),
					Description: description,
					InputSchema: ensureObjectSchema(child.Parameters),
				})
			}
		case "web_search":
			// Codex sends Responses-native service tools such as web_search.
			// The local gateway cannot execute OpenAI service-side tools itself, so
			// do not expose them as Anthropic function tools to upstream adapters.
			continue
		default:
			return nil, fmt.Errorf("unsupported tool type %q", t.Type)
		}
	}
	return tools, nil
}

func openAINamespaceToolName(namespace, name string) string {
	namespace = strings.TrimSpace(namespace)
	name = strings.TrimSpace(name)
	if namespace == "" {
		return name
	}
	if strings.HasSuffix(namespace, "_") || strings.HasPrefix(name, "_") {
		return namespace + name
	}
	return namespace + "__" + name
}

func ensureObjectSchema(schema map[string]any) map[string]any {
	if schema != nil {
		return schema
	}
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
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

func mergeChatMetadata(req OpenAIChatCompletionsRequest) map[string]any {
	out := mergeMetadata(req.Metadata, req.StreamOptions)
	if out == nil {
		out = map[string]any{}
	}
	if req.ResponseFormat != nil {
		out["response_format"] = req.ResponseFormat
	}
	if req.MaxCompletionTokens > 0 {
		out["max_completion_tokens"] = req.MaxCompletionTokens
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeResponsesMetadata(req OpenAIResponsesRequest) map[string]any {
	out := mergeMetadata(req.Metadata, req.StreamOptions)
	if out == nil {
		out = map[string]any{}
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		out["previous_response_id"] = req.PreviousResponseID
	}
	if len(req.Reasoning) > 0 {
		out["reasoning"] = req.Reasoning
	}
	if len(req.Text) > 0 {
		out["text"] = req.Text
	}
	if req.ParallelToolCalls != nil {
		out["parallel_tool_calls"] = *req.ParallelToolCalls
	}
	if req.Store != nil {
		out["store"] = *req.Store
	}
	if len(req.Include) > 0 {
		out["include"] = req.Include
	}
	if strings.TrimSpace(req.Truncation) != "" {
		out["truncation"] = req.Truncation
	}
	if strings.TrimSpace(req.ServiceTier) != "" {
		out["service_tier"] = req.ServiceTier
	}
	if strings.TrimSpace(req.PromptCacheKey) != "" {
		out["prompt_cache_key"] = req.PromptCacheKey
	}
	if strings.TrimSpace(req.SafetyIdentifier) != "" {
		out["safety_identifier"] = req.SafetyIdentifier
	}
	if req.TopLogprobs != nil {
		out["top_logprobs"] = *req.TopLogprobs
	}
	if req.Background != nil {
		out["background"] = *req.Background
	}
	if req.Conversation != nil {
		out["conversation"] = req.Conversation
	}
	if req.MaxToolCalls != nil {
		out["max_tool_calls"] = *req.MaxToolCalls
	}
	if len(req.ClientMetadata) > 0 {
		out["client_metadata"] = req.ClientMetadata
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
		for i, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			converted, err := parseResponsesInputItem(i, obj)
			if err != nil {
				return nil, err
			}
			out = append(out, converted...)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("input is required")
		}
		return out, nil
	default:
		return nil, fmt.Errorf("input must be string or array")
	}
}

func parseResponsesInputItem(index int, obj map[string]any) ([]MessageParam, error) {
	role := strings.TrimSpace(stringFromAny(obj["role"]))
	itemType := strings.ToLower(strings.TrimSpace(stringFromAny(obj["type"])))

	// Support role-style input items and preserve OpenAI tool history payloads.
	if role != "" || itemType == "message" {
		if role == "" {
			role = "user"
		}
		chatMsg := OpenAIChatMessage{
			Role:       role,
			Content:    obj["content"],
			ToolCallID: firstNonEmptyString(obj, "tool_call_id", "call_id"),
			ToolCalls:  parseOpenAIToolCallsFromAny(obj["tool_calls"]),
		}
		converted, _, err := openAIChatMessageToMessageParams(chatMsg)
		if err != nil {
			return nil, fmt.Errorf("input[%d]: %w", index, err)
		}
		return converted, nil
	}

	switch itemType {
	case "function_call", "tool_call", "custom_tool_call", "local_shell_call", "tool_search_call":
		name := strings.TrimSpace(stringFromAny(obj["name"]))
		if name == "" {
			switch itemType {
			case "local_shell_call":
				name = "local_shell"
			case "tool_search_call":
				name = "tool_search"
			}
		}
		if name == "" {
			return nil, fmt.Errorf("input[%d]: function_call name is required", index)
		}
		callID := firstNonEmptyString(obj, "call_id", "id")
		if callID == "" {
			callID = fmt.Sprintf("toolu_resp_%d", index+1)
		}
		inputObj := map[string]any{}
		if rawArgs, ok := obj["arguments"]; ok {
			inputObj = parseToolInputValue(rawArgs)
		} else if rawInput, ok := obj["input"]; ok {
			inputObj = parseToolInputValue(rawInput)
		} else if rawAction, ok := obj["action"]; ok {
			inputObj = parseToolInputValue(rawAction)
		}
		return []MessageParam{
			{
				Role: "assistant",
				Content: []any{
					map[string]any{
						"type":  "tool_use",
						"id":    callID,
						"name":  name,
						"input": inputObj,
					},
				},
			},
		}, nil
	case "function_call_output", "custom_tool_call_output", "tool_search_output", "tool_result":
		callID := firstNonEmptyString(obj, "call_id", "tool_call_id", "id")
		if callID == "" {
			return nil, fmt.Errorf("input[%d]: function_call_output call_id is required", index)
		}
		output := openAIContentToToolResult(obj["output"])
		if strings.TrimSpace(output) == "" {
			output = openAIContentToToolResult(obj["content"])
		}
		return []MessageParam{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type":        "tool_result",
						"tool_use_id": callID,
						"content":     output,
					},
				},
			},
		}, nil
	default:
		if content, ok := obj["content"]; ok {
			return []MessageParam{
				{Role: "user", Content: content},
			}, nil
		}
		if text := strings.TrimSpace(stringFromAny(obj["text"])); text != "" {
			return []MessageParam{
				{Role: "user", Content: text},
			}, nil
		}
		return nil, nil
	}
}

func parseOpenAIToolCallsFromAny(raw any) []OpenAIToolCall {
	switch v := raw.(type) {
	case []OpenAIToolCall:
		if len(v) == 0 {
			return nil
		}
		out := make([]OpenAIToolCall, len(v))
		copy(out, v)
		return out
	case []any:
		out := make([]OpenAIToolCall, 0, len(v))
		for _, item := range v {
			obj, ok := item.(map[string]any)
			if !ok {
				continue
			}
			call := OpenAIToolCall{
				ID:   strings.TrimSpace(stringFromAny(obj["id"])),
				Type: strings.TrimSpace(stringFromAny(obj["type"])),
			}
			if call.Type == "" {
				call.Type = "function"
			}
			if fn, ok := obj["function"].(map[string]any); ok {
				call.Function = OpenAIToolFunction{
					Name:      strings.TrimSpace(stringFromAny(fn["name"])),
					Arguments: strings.TrimSpace(stringFromAny(fn["arguments"])),
				}
			}
			out = append(out, call)
		}
		return out
	default:
		return nil
	}
}

func parseToolInputValue(raw any) map[string]any {
	switch v := raw.(type) {
	case map[string]any:
		return v
	case string:
		return parseOpenAIToolArguments(v)
	case nil:
		return map[string]any{}
	default:
		return map[string]any{"value": v}
	}
}

func firstNonEmptyString(obj map[string]any, keys ...string) string {
	for _, key := range keys {
		if text := strings.TrimSpace(stringFromAny(obj[key])); text != "" {
			return text
		}
	}
	return ""
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

func openAIResponseStreamEvent(respID, itemID string, ev orchestrator.StreamEvent) map[string]any {
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
			"type":          "response.output_text.delta",
			"response_id":   respID,
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"delta":         ev.DeltaText,
		}
	default:
		return nil
	}
}

type openAIResponseFunctionCallState struct {
	ItemID      string
	CallID      string
	Name        string
	OutputIndex int
	Arguments   strings.Builder
}

func newOpenAIResponseFunctionCallState(respID string, ev orchestrator.StreamEvent) *openAIResponseFunctionCallState {
	block := ev.Block
	callID := strings.TrimSpace(block.ID)
	if callID == "" {
		callID = fmt.Sprintf("call_%s_%d", respID, ev.Index)
	}
	name := strings.TrimSpace(block.Name)
	if name == "" {
		name = "tool"
	}
	outputIndex := ev.Index
	if outputIndex < 0 {
		outputIndex = 0
	}
	return &openAIResponseFunctionCallState{
		ItemID:      callID,
		CallID:      callID,
		Name:        name,
		OutputIndex: outputIndex,
	}
}

func openAIResponseFunctionCallAddedEvent(call *openAIResponseFunctionCallState) map[string]any {
	return map[string]any{
		"type":         "response.output_item.added",
		"output_index": call.OutputIndex,
		"item": map[string]any{
			"id":        call.ItemID,
			"type":      "function_call",
			"status":    "in_progress",
			"name":      call.Name,
			"arguments": "",
			"call_id":   call.CallID,
		},
	}
}

func openAIResponseFunctionCallArgumentsDeltaEvent(respID string, call *openAIResponseFunctionCallState, delta string) map[string]any {
	return map[string]any{
		"type":         "response.function_call_arguments.delta",
		"response_id":  respID,
		"item_id":      call.ItemID,
		"call_id":      call.CallID,
		"output_index": call.OutputIndex,
		"delta":        delta,
	}
}

func openAIResponseFunctionCallDoneEvent(call *openAIResponseFunctionCallState) map[string]any {
	return map[string]any{
		"type":         "response.output_item.done",
		"output_index": call.OutputIndex,
		"item": map[string]any{
			"id":        call.ItemID,
			"type":      "function_call",
			"name":      call.Name,
			"arguments": call.Arguments.String(),
			"call_id":   call.CallID,
		},
	}
}

func openAIResponseCreatedEvent(respID, outwardModel string, created int64) map[string]any {
	return map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":      respID,
			"object":  "response",
			"created": created,
			"status":  "in_progress",
			"model":   outwardModel,
			"output":  []any{},
		},
	}
}

func openAIResponseTextStartEvents(itemID string) []map[string]any {
	return []map[string]any{
		{
			"type":         "response.output_item.added",
			"output_index": 0,
			"item": map[string]any{
				"id":      itemID,
				"type":    "message",
				"status":  "in_progress",
				"role":    "assistant",
				"content": []any{},
			},
		},
		{
			"type":          "response.content_part.added",
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"part": map[string]any{
				"type": "output_text",
				"text": "",
			},
		},
	}
}

func openAIResponseTextDoneEvents(respID, itemID, text string) []map[string]any {
	content := []any{
		map[string]any{
			"type": "output_text",
			"text": text,
		},
	}
	return []map[string]any{
		{
			"type":          "response.output_text.done",
			"response_id":   respID,
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"text":          text,
		},
		{
			"type":          "response.content_part.done",
			"item_id":       itemID,
			"output_index":  0,
			"content_index": 0,
			"part": map[string]any{
				"type": "output_text",
				"text": text,
			},
		},
		{
			"type":         "response.output_item.done",
			"output_index": 0,
			"item": map[string]any{
				"id":      itemID,
				"type":    "message",
				"status":  "completed",
				"role":    "assistant",
				"content": content,
			},
		},
	}
}

func openAIResponseCompletedEvent(respID, outwardModel string, created int64, text string, usage orchestrator.Usage) map[string]any {
	itemID := "msg_" + respID
	output := []any{}
	if text != "" {
		output = append(output, map[string]any{
			"type":   "message",
			"id":     itemID,
			"status": "completed",
			"role":   "assistant",
			"content": []any{
				map[string]any{
					"type": "output_text",
					"text": text,
				},
			},
		})
	}
	return map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":      respID,
			"object":  "response",
			"created": created,
			"status":  "completed",
			"model":   outwardModel,
			"output":  output,
			"usage": map[string]any{
				"input_tokens":  usage.InputTokens,
				"output_tokens": usage.OutputTokens,
				"total_tokens":  usage.InputTokens + usage.OutputTokens,
			},
		},
	}
}

func openAIResponseFailedEvent(respID, outwardModel string, created int64, err error) map[string]any {
	message := "stream failed"
	if err != nil && strings.TrimSpace(err.Error()) != "" {
		message = err.Error()
	}
	return map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":      respID,
			"object":  "response",
			"created": created,
			"status":  "failed",
			"model":   outwardModel,
			"output":  []any{},
			"error": map[string]any{
				"type":    "server_error",
				"code":    "stream_error",
				"message": message,
			},
		},
	}
}

func writeOpenAISSEData(w http.ResponseWriter, data string) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}
