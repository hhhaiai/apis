package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/ccrun"
	"ccgateway/internal/memory"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/runlog"
)

func (s *server) handleMessages(w http.ResponseWriter, r *http.Request) {
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
		recordText := buildRunRecordText("/v1/messages", mode, statusCode, streamMode, generatedText, errText)
		s.logRun(runlog.Entry{
			RunID:          runID,
			Path:           "/v1/messages",
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
					"path":        "/v1/messages",
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

	if err := requireAnthropicVersion(r); err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	var req MessagesRequest
	if err := decodeJSONBodySingle(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		statusCode = http.StatusBadRequest
		errText = "invalid JSON body"
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if err := validateMessagesRequest(req); err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	if err := s.enforceTokenModelAccess(r.Context(), req.Model); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
		return
	}
	mode = requestMode(r, req.Metadata)
	clientModel = req.Model
	streamMode = req.Stream
	toolCount = len(req.Tools)
	sessionID = requestSessionID(r, req.Metadata)
	req.System = s.applySystemPromptPrefix(mode, req.System)
	req.Metadata = s.applyRoutingPolicy(mode, req.Metadata)

	// --- Memory Integration Start ---
	if s.memoryStore != nil && sessionID != "" {
		ctx := r.Context()
		// 1. Get working memory
		wm, err := s.memoryStore.GetWorkingMemory(ctx, sessionID)
		if err != nil {
			s.appendEvent(ccevent.AppendInput{
				EventType: "memory.error",
				SessionID: sessionID,
				RunID:     runID,
				Data: map[string]any{
					"stage": "get_working_memory",
					"error": err.Error(),
				},
			})
		}

		// 2. Append current user message
		if wm != nil {
			lastUserMsg := req.Messages[len(req.Messages)-1]
			wm.Messages = append(wm.Messages, memory.Message{
				Role:      lastUserMsg.Role,
				Content:   contentToMemoryText(lastUserMsg.Content),
				Timestamp: time.Now(),
			})
			_ = s.memoryStore.UpdateWorkingMemory(ctx, wm)
		}

		// 3. Summarization Check
		if wm != nil && len(wm.Messages) > 10 && s.summarizer != nil {
			go func(sid string, msgs []memory.Message) {
				// Async summarization to avoid blocking response
				summary, err := s.summarizer.SummarizeRecent(context.Background(), msgs)
				if err == nil {
					sm, _ := s.memoryStore.GetSessionMemory(context.Background(), sid)
					if sm == nil {
						sm = &memory.SessionMemory{SessionID: sid}
					}
					sm.Summary = summary
					_ = s.memoryStore.UpdateSessionMemory(context.Background(), sm)

					// Trucate working memory (keep last 4 messages + current)
					// Verify concurrency safety in real implementation
					wmPayload, _ := s.memoryStore.GetWorkingMemory(context.Background(), sid)
					if wmPayload != nil && len(wmPayload.Messages) > 4 {
						wmPayload.Messages = wmPayload.Messages[len(wmPayload.Messages)-4:]
						_ = s.memoryStore.UpdateWorkingMemory(context.Background(), wmPayload)
					}
				}
			}(sessionID, wm.Messages)
		}

		// 4. Context Construction (Optional - for this implementation we just logging it)
		// To truly use it, we would replace req.Messages with buildContextMessages(wm, sm)
		// However, since we are a gateway, strictly replacing messages might break client expectations
		// So we will inject the summary into System prompt if available
		sm, _ := s.memoryStore.GetSessionMemory(ctx, sessionID)
		if sm != nil && sm.Summary != "" {
			summarySystemMsg := fmt.Sprintf("\n\nPrevious Conversation Summary:\n%s", sm.Summary)
			if req.System == nil {
				req.System = summarySystemMsg
			} else {
				req.System = fmt.Sprintf("%v%s", req.System, summarySystemMsg)
			}
		}
	}
	// --- Memory Integration End ---

	requestedModel, mappedModel, err := s.resolveUpstreamModel(mode, clientModel)
	if err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	upstreamModel = mappedModel
	req.Model = mappedModel
	req.Metadata = s.applyChannelRoutePolicy(r.Context(), req.Metadata, mappedModel)

	action := policy.Action{
		Path:      "/v1/messages",
		Model:     req.Model,
		Mode:      mode,
		ToolNames: toolNames(req.Tools),
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
		Path:           "/v1/messages",
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
			"path":            "/v1/messages",
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

	creq := toCanonicalRequest(runID, req, r)
	if creq.Metadata == nil {
		creq.Metadata = map[string]any{}
	}
	creq.Metadata["mode"] = mode
	creq.Metadata["session_id"] = sessionID
	creq.Metadata["request_path"] = "/v1/messages"
	creq.Metadata["client_model"] = clientModel
	creq.Metadata["requested_model"] = requestedModel
	creq.Metadata["upstream_model"] = mappedModel
	reservedQuota := estimateReservedQuota(req.MaxTokens, req.System, req.Messages)
	if err := s.reserveQuotaFromRequestContext(r.Context(), reservedQuota); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "quota_error", err.Error())
		return
	}
	if req.Stream {
		if _, ok := creq.Metadata["strict_stream_passthrough"]; !ok {
			creq.Metadata["strict_stream_passthrough"] = true
		}
		if _, ok := creq.Metadata["strict_stream_passthrough_soft"]; !ok {
			creq.Metadata["strict_stream_passthrough_soft"] = true
		}
		creq = s.applyVisionFallback(r.Context(), creq)
		creq = s.applyToolSupportFallback(creq)
		var usage orchestrator.Usage
		if s.shouldStreamWithToolLoop(creq) {
			generatedText, usage = s.streamMessagesWithToolLoop(w, r, creq, requestedModel)
		} else {
			generatedText, usage = s.streamMessages(w, r, creq, requestedModel)
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

	msg := fromCanonicalResponse(s.nextID("msg"), resp)
	msg.Model = clientModel
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(msg)
}

func (s *server) streamMessages(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) (string, orchestrator.Usage) {
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

	events, errs := s.orchestrator.Stream(r.Context(), req)

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return generated.String(), usage
			}
			appendStreamText(&generated, ev)
			if ev.Usage.InputTokens > 0 || ev.Usage.OutputTokens > 0 {
				usage = ev.Usage
			}
			if ev.PassThrough && len(ev.RawData) > 0 {
				raw := ev.RawData
				if rewritten, ok := rewriteAnthropicStreamModel(ev.Type, ev.RawData, outwardModel); ok {
					raw = rewritten
				}
				eventName := ev.Type
				if strings.TrimSpace(ev.RawEvent) != "" {
					eventName = ev.RawEvent
				}
				if err := writeSSERaw(w, eventName, raw); err != nil {
					return generated.String(), usage
				}
				flusher.Flush()
				continue
			}
			payload := streamPayloadFromEvent(ev, outwardModel, s.nextID("msg"))
			if err := writeSSE(w, ev.Type, payload); err != nil {
				return generated.String(), usage
			}
			flusher.Flush()
		case err, ok := <-errs:
			if !ok || err == nil {
				continue
			}
			_ = writeSSE(w, "error", map[string]any{
				"type": "error",
				"error": map[string]any{
					"type":    "api_error",
					"message": err.Error(),
				},
			})
			flusher.Flush()
			return generated.String(), usage
		case <-r.Context().Done():
			return generated.String(), usage
		}
	}
}

func (s *server) handleCountTokens(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	statusCode := http.StatusOK
	errText := ""
	defer func() {
		s.logRun(runlog.Entry{
			Path:       "/v1/messages/count_tokens",
			Mode:       "chat",
			Stream:     false,
			Status:     statusCode,
			Error:      errText,
			DurationMS: time.Since(started).Milliseconds(),
		})
	}()

	if r.Method != http.MethodPost {
		statusCode = http.StatusMethodNotAllowed
		errText = "method not allowed"
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}
	if err := requireAnthropicVersion(r); err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	var req CountTokensRequest
	if err := decodeJSONBodySingle(r, &req, false); err != nil {
		s.reportRequestDecodeIssue(r, err)
		statusCode = http.StatusBadRequest
		errText = "invalid JSON body"
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		statusCode = http.StatusBadRequest
		errText = "model is required"
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if err := s.enforceTokenModelAccess(r.Context(), req.Model); err != nil {
		statusCode = http.StatusForbidden
		errText = err.Error()
		s.writeError(w, http.StatusForbidden, "permission_error", err.Error())
		return
	}
	if len(req.Messages) == 0 {
		statusCode = http.StatusBadRequest
		errText = "messages is required"
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}
	mode := requestMode(r, nil)
	clientModel := req.Model
	requestedModel, mappedModel, err := s.resolveUpstreamModel(mode, clientModel)
	if err != nil {
		statusCode = http.StatusBadRequest
		errText = err.Error()
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	req.Model = mappedModel
	w.Header().Set("x-cc-mode", mode)
	w.Header().Set("x-cc-client-model", clientModel)
	w.Header().Set("x-cc-requested-model", requestedModel)
	w.Header().Set("x-cc-upstream-model", mappedModel)

	tokens := 0
	for _, m := range req.Messages {
		tokens += estimateContentTokens(m.Content)
	}
	if req.System != nil {
		tokens += estimateContentTokens(req.System)
	}

	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(CountTokensResponse{InputTokens: max(tokens, 1)})
}

func validateMessagesRequest(req MessagesRequest) error {
	if strings.TrimSpace(req.Model) == "" {
		return fmt.Errorf("model is required")
	}
	if req.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be > 0")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages is required")
	}
	for _, t := range req.Tools {
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("tool name is required")
		}
		if t.InputSchema == nil {
			return fmt.Errorf("tool %q input_schema is required", t.Name)
		}
	}
	return nil
}

func toolNames(tools []ToolDefinition) []string {
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	return names
}

func toCanonicalRequest(runID string, req MessagesRequest, r *http.Request) orchestrator.Request {
	msgs := make([]orchestrator.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, orchestrator.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}
	tools := make([]orchestrator.Tool, 0, len(req.Tools))
	for _, t := range req.Tools {
		tools = append(tools, orchestrator.Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	headers := map[string]string{
		"anthropic-version": r.Header.Get("anthropic-version"),
		"anthropic-beta":    r.Header.Get("anthropic-beta"),
		"x-api-key":         r.Header.Get("x-api-key"),
		"authorization":     r.Header.Get("authorization"),
	}

	metadata := map[string]any{}
	for k, v := range req.Metadata {
		metadata[k] = v
	}
	if req.ToolChoice != nil {
		metadata["tool_choice"] = req.ToolChoice
	}
	if req.Temperature != nil {
		metadata["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		metadata["top_p"] = *req.TopP
	}
	if len(metadata) == 0 {
		metadata = nil
	}

	return orchestrator.Request{
		RunID:     runID,
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  msgs,
		Tools:     tools,
		Metadata:  metadata,
		Headers:   headers,
	}
}

func fromCanonicalResponse(messageID string, resp orchestrator.Response) MessageResponse {
	blocks := make([]ContentBlock, 0, len(resp.Blocks))
	for _, b := range resp.Blocks {
		cb := ContentBlock{
			Type:  b.Type,
			Text:  b.Text,
			ID:    b.ID,
			Name:  b.Name,
			Input: b.Input,
		}
		blocks = append(blocks, cb)
	}

	return MessageResponse{
		ID:           messageID,
		Type:         "message",
		Role:         "assistant",
		Model:        resp.Model,
		Content:      blocks,
		StopReason:   resp.StopReason,
		StopSequence: nil,
		Usage: UsageResponse{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
		},
	}
}

func streamPayloadFromEvent(ev orchestrator.StreamEvent, model, messageID string) map[string]any {
	switch ev.Type {
	case "message_start":
		return map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":    messageID,
				"type":  "message",
				"role":  "assistant",
				"model": model,
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		}
	case "content_block_start":
		block := map[string]any{
			"type": ev.Block.Type,
		}
		if ev.Block.Type == "tool_use" {
			block["id"] = ev.Block.ID
			block["name"] = ev.Block.Name
			block["input"] = map[string]any{}
		} else if ev.Block.Type == "text" {
			block["text"] = ""
		}
		return map[string]any{
			"type":          "content_block_start",
			"index":         ev.Index,
			"content_block": block,
		}
	case "content_block_delta":
		delta := map[string]any{}
		if ev.DeltaJSON != "" {
			delta["type"] = "input_json_delta"
			delta["partial_json"] = ev.DeltaJSON
		} else {
			delta["type"] = "text_delta"
			delta["text"] = ev.DeltaText
		}
		return map[string]any{
			"type":  "content_block_delta",
			"index": ev.Index,
			"delta": delta,
		}
	case "content_block_stop":
		return map[string]any{
			"type":  "content_block_stop",
			"index": ev.Index,
		}
	case "message_delta":
		return map[string]any{
			"type": "message_delta",
			"delta": map[string]any{
				"stop_reason":   ev.StopReason,
				"stop_sequence": nil,
			},
			"usage": map[string]any{
				"output_tokens": ev.Usage.OutputTokens,
			},
		}
	default:
		return map[string]any{
			"type": "message_stop",
		}
	}
}

func rewriteAnthropicStreamModel(eventType string, raw []byte, outwardModel string) ([]byte, bool) {
	if strings.TrimSpace(outwardModel) == "" {
		return nil, false
	}
	if eventType != "message_start" {
		return nil, false
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, false
	}
	msg, ok := payload["message"].(map[string]any)
	if !ok {
		return nil, false
	}
	msg["model"] = outwardModel
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func estimateContentTokens(content any) int {
	switch c := content.(type) {
	case string:
		return tokenCount(c)
	case []any:
		total := 0
		for _, item := range c {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := block["text"].(string); ok {
				total += tokenCount(text)
			}
		}
		return total
	default:
		return 1
	}
}

func tokenCount(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 1
	}
	return len(strings.Fields(text))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func estimateReservedQuota(maxTokens int, system any, messages []MessageParam) int64 {
	reserved := int64(max(maxTokens, 1))
	for _, m := range messages {
		reserved += int64(max(estimateContentTokens(m.Content), 0))
	}
	if system != nil {
		reserved += int64(max(estimateContentTokens(system), 0))
	}
	if reserved <= 0 {
		return 1
	}
	return reserved
}

func buildContextMessages(wm *memory.WorkingMemory, sm *memory.SessionMemory) []orchestrator.Message {
	messages := []orchestrator.Message{}

	// Add session summary as system message if it exists
	if sm != nil && sm.Summary != "" {
		messages = append(messages, orchestrator.Message{
			Role: "system",
			Content: []any{
				map[string]any{
					"type": "text",
					"text": fmt.Sprintf("Conversation Summary:\n%s", sm.Summary),
				},
			},
		})
	}

	// Add working memory messages
	if wm != nil {
		for _, msg := range wm.Messages {
			messages = append(messages, orchestrator.Message{
				Role: msg.Role,
				Content: []any{
					map[string]any{
						"type": "text",
						"text": msg.Content,
					},
				},
			})
		}
	}

	return messages
}

func contentToMemoryText(content any) string {
	switch c := content.(type) {
	case string:
		return strings.TrimSpace(c)
	case []any:
		parts := make([]string, 0, len(c))
		for _, item := range c {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text, _ := block["text"].(string)
			text = strings.TrimSpace(text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		text, _ := c["text"].(string)
		return strings.TrimSpace(text)
	default:
		return strings.TrimSpace(fmt.Sprint(content))
	}
}
