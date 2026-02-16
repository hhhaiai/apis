package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"ccgateway/internal/orchestrator"
)

func (s *server) shouldStreamWithToolLoop(req orchestrator.Request) bool {
	if len(req.Tools) == 0 {
		return false
	}
	cfg := toolLoopConfigFromMetadata(req.Metadata)
	return cfg.enabled
}

func (s *server) streamMessagesWithToolLoop(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) (string, orchestrator.Usage) {
	var usage orchestrator.Usage
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return "", usage
	}

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	resp, err := s.completeWithToolLoop(r.Context(), req)
	if err != nil {
		_ = writeSSE(w, "error", map[string]any{
			"type": "error",
			"error": map[string]any{
				"type":    "api_error",
				"message": err.Error(),
			},
		})
		flusher.Flush()
		return "", usage
	}

	messageID := s.nextID("msg")
	writeEvent := func(event string, payload any) bool {
		if err := writeSSE(w, event, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !writeEvent("message_start", streamPayloadFromEvent(orchestrator.StreamEvent{Type: "message_start"}, outwardModel, messageID)) {
		return collectResponseText(resp), resp.Usage
	}
	for idx, block := range resp.Blocks {
		start := orchestrator.StreamEvent{
			Type:  "content_block_start",
			Index: idx,
			Block: block,
		}
		if !writeEvent("content_block_start", streamPayloadFromEvent(start, outwardModel, messageID)) {
			return collectResponseText(resp), resp.Usage
		}

		switch block.Type {
		case "text":
			delta := orchestrator.StreamEvent{
				Type:      "content_block_delta",
				Index:     idx,
				DeltaText: block.Text,
			}
			if !writeEvent("content_block_delta", streamPayloadFromEvent(delta, outwardModel, messageID)) {
				return collectResponseText(resp), resp.Usage
			}
		case "tool_use":
			inputRaw, _ := json.Marshal(block.Input)
			partialJSON := "{}"
			if len(inputRaw) > 0 {
				partialJSON = string(inputRaw)
			}
			delta := orchestrator.StreamEvent{
				Type:      "content_block_delta",
				Index:     idx,
				DeltaJSON: partialJSON,
			}
			if !writeEvent("content_block_delta", streamPayloadFromEvent(delta, outwardModel, messageID)) {
				return collectResponseText(resp), resp.Usage
			}
		}

		stop := orchestrator.StreamEvent{
			Type:  "content_block_stop",
			Index: idx,
		}
		if !writeEvent("content_block_stop", streamPayloadFromEvent(stop, outwardModel, messageID)) {
			return collectResponseText(resp), resp.Usage
		}
	}

	stopReason := strings.TrimSpace(resp.StopReason)
	if stopReason == "" {
		stopReason = "end_turn"
	}
	msgDelta := orchestrator.StreamEvent{
		Type:       "message_delta",
		StopReason: stopReason,
		Usage:      resp.Usage,
	}
	if !writeEvent("message_delta", streamPayloadFromEvent(msgDelta, outwardModel, messageID)) {
		return collectResponseText(resp), resp.Usage
	}
	_ = writeEvent("message_stop", streamPayloadFromEvent(orchestrator.StreamEvent{Type: "message_stop"}, outwardModel, messageID))

	return collectResponseText(resp), resp.Usage
}

func (s *server) streamOpenAIChatCompletionsWithToolLoop(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) (string, orchestrator.Usage) {
	var usage orchestrator.Usage
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return "", usage
	}

	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	resp, err := s.completeWithToolLoop(r.Context(), req)
	if err != nil {
		_ = writeOpenAISSEData(w, fmt.Sprintf(`{"error":{"message":%q}}`, err.Error()))
		flusher.Flush()
		return "", usage
	}

	streamID := s.nextID("chatcmpl")
	created := time.Now().Unix()
	writeChunk := func(chunk map[string]any) bool {
		raw, _ := json.Marshal(chunk)
		if err := writeOpenAISSEData(w, string(raw)); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	startChunk := map[string]any{
		"id":      streamID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   outwardModel,
		"choices": []map[string]any{
			{
				"index": 0,
				"delta": map[string]any{
					"role": "assistant",
				},
				"finish_reason": nil,
			},
		},
	}
	if !writeChunk(startChunk) {
		return collectResponseText(resp), resp.Usage
	}

	toolIndex := 0
	for _, block := range resp.Blocks {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			chunk := map[string]any{
				"id":      streamID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   outwardModel,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"content": block.Text,
						},
						"finish_reason": nil,
					},
				},
			}
			if !writeChunk(chunk) {
				return collectResponseText(resp), resp.Usage
			}
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			chunk := map[string]any{
				"id":      streamID,
				"object":  "chat.completion.chunk",
				"created": created,
				"model":   outwardModel,
				"choices": []map[string]any{
					{
						"index": 0,
						"delta": map[string]any{
							"tool_calls": []map[string]any{
								{
									"index": toolIndex,
									"id":    block.ID,
									"type":  "function",
									"function": map[string]any{
										"name":      block.Name,
										"arguments": string(args),
									},
								},
							},
						},
						"finish_reason": nil,
					},
				},
			}
			if !writeChunk(chunk) {
				return collectResponseText(resp), resp.Usage
			}
			toolIndex++
		}
	}

	finishReason := "stop"
	if strings.TrimSpace(resp.StopReason) == "tool_use" {
		finishReason = "tool_calls"
	}
	finishChunk := map[string]any{
		"id":      streamID,
		"object":  "chat.completion.chunk",
		"created": created,
		"model":   outwardModel,
		"choices": []map[string]any{
			{
				"index":         0,
				"delta":         map[string]any{},
				"finish_reason": finishReason,
			},
		},
	}
	if writeChunk(finishChunk) {
		_ = writeOpenAISSEData(w, "[DONE]")
		flusher.Flush()
	}
	return collectResponseText(resp), resp.Usage
}

func (s *server) streamOpenAIResponsesWithToolLoop(w http.ResponseWriter, r *http.Request, req orchestrator.Request, outwardModel string) (string, orchestrator.Usage) {
	var usage orchestrator.Usage
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return "", usage
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
	if err := writeOpenAISSEData(w, string(rawCreated)); err != nil {
		return "", usage
	}
	flusher.Flush()

	resp, err := s.completeWithToolLoop(r.Context(), req)
	if err != nil {
		_ = writeOpenAISSEData(w, fmt.Sprintf(`{"type":"error","error":{"message":%q}}`, err.Error()))
		flusher.Flush()
		return "", usage
	}

	for _, block := range resp.Blocks {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			item := map[string]any{
				"type":        "response.output_text.delta",
				"response_id": respID,
				"delta":       block.Text,
			}
			raw, _ := json.Marshal(item)
			if err := writeOpenAISSEData(w, string(raw)); err != nil {
				return collectResponseText(resp), resp.Usage
			}
			flusher.Flush()
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			item := map[string]any{
				"type":        "response.function_call_arguments.delta",
				"response_id": respID,
				"delta":       string(args),
			}
			raw, _ := json.Marshal(item)
			if err := writeOpenAISSEData(w, string(raw)); err != nil {
				return collectResponseText(resp), resp.Usage
			}
			flusher.Flush()
		}
	}

	completed := map[string]any{
		"type":    "response.completed",
		"id":      respID,
		"model":   outwardModel,
		"created": created,
	}
	rawCompleted, _ := json.Marshal(completed)
	_ = writeOpenAISSEData(w, string(rawCompleted))
	_ = writeOpenAISSEData(w, "[DONE]")
	flusher.Flush()

	return collectResponseText(resp), resp.Usage
}
