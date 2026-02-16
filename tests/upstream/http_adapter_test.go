package upstream_test

import (
	"bufio"
	. "ccgateway/internal/upstream"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/orchestrator"
)

func TestHTTPAdapterOpenAI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "mapped-model" {
			t.Fatalf("expected model mapped-model, got %#v", body["model"])
		}
		msgs, ok := body["messages"].([]any)
		if !ok || len(msgs) < 2 {
			t.Fatalf("expected at least 2 messages with system injection, got %#v", body["messages"])
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"mapped-model",
			"choices":[{"finish_reason":"stop","message":{"content":"ok","tool_calls":[]}}],
			"usage":{"prompt_tokens":3,"completion_tokens":2}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "oa",
		Kind:    AdapterKindOpenAI,
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	resp, err := adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "mapped-model",
		MaxTokens: 64,
		System:    "system prompt",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.Blocks) == 0 || resp.Blocks[0].Type != "text" {
		t.Fatalf("unexpected blocks: %+v", resp.Blocks)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("unexpected stop reason: %q", resp.StopReason)
	}
}

func TestHTTPAdapterOpenAIForceStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writer := bufio.NewWriter(w)

		_, _ = fmt.Fprintln(writer, `data: {"choices":[{"delta":{"content":"hel"},"finish_reason":null}]}`)
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":7,"completion_tokens":2}}`)
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, `data: [DONE]`)
		_, _ = fmt.Fprintln(writer)
		_ = writer.Flush()
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:          "oa-stream",
		Kind:          AdapterKindOpenAI,
		BaseURL:       server.URL,
		ForceStream:   true,
		StreamOptions: map[string]any{"include_usage": true},
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	resp, err := adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "m",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.Blocks) == 0 || resp.Blocks[0].Text != "hello" {
		t.Fatalf("unexpected blocks: %+v", resp.Blocks)
	}
	if resp.Usage.InputTokens != 7 || resp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected usage: %+v", resp.Usage)
	}
}

func TestHTTPAdapterAnthropic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "ant-key" {
			t.Fatalf("unexpected x-api-key: %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("unexpected anthropic-version: %q", got)
		}
		if got := r.Header.Get("anthropic-beta"); got != "tools-2024-04-04" {
			t.Fatalf("unexpected anthropic-beta: %q", got)
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"content":[{"type":"text","text":"hi"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":4,"output_tokens":2}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "an",
		Kind:    AdapterKindAnthropic,
		BaseURL: server.URL,
		APIKey:  "ant-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	resp, err := adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "claude-test",
		MaxTokens: 128,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
			"anthropic-beta":    "tools-2024-04-04",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.Blocks) != 1 || resp.Blocks[0].Text != "hi" {
		t.Fatalf("unexpected blocks: %+v", resp.Blocks)
	}
}

func TestHTTPAdapterAnthropicToolChoiceMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		choice, ok := body["tool_choice"].(map[string]any)
		if !ok {
			t.Fatalf("expected tool_choice map, got %#v", body["tool_choice"])
		}
		if typ, _ := choice["type"].(string); typ != "any" {
			t.Fatalf("expected anthropic tool_choice any, got %#v", choice["type"])
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "an-tool-choice",
		Kind:    AdapterKindAnthropic,
		BaseURL: server.URL,
		APIKey:  "ant-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "claude-test",
		MaxTokens: 128,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
		Tools: []orchestrator.Tool{
			{Name: "get_weather", InputSchema: map[string]any{"type": "object"}},
		},
		Metadata: map[string]any{
			"tool_choice": "required",
		},
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestHTTPAdapterOpenAIToolChoiceMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		choice, ok := body["tool_choice"].(map[string]any)
		if !ok {
			t.Fatalf("expected openai tool_choice object, got %#v", body["tool_choice"])
		}
		if typ, _ := choice["type"].(string); typ != "function" {
			t.Fatalf("unexpected openai tool_choice type: %#v", choice["type"])
		}
		fn, ok := choice["function"].(map[string]any)
		if !ok {
			t.Fatalf("expected function in tool_choice, got %#v", choice["function"])
		}
		if name, _ := fn["name"].(string); name != "read_file" {
			t.Fatalf("unexpected function name: %q", name)
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"gpt-test",
			"choices":[{"finish_reason":"stop","message":{"content":"ok","tool_calls":[]}}],
			"usage":{"prompt_tokens":1,"completion_tokens":1}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "oa-tool-choice",
		Kind:    AdapterKindOpenAI,
		BaseURL: server.URL,
		APIKey:  "oa-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "gpt-test",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello"},
		},
		Tools: []orchestrator.Tool{
			{Name: "read_file", InputSchema: map[string]any{"type": "object"}},
		},
		Metadata: map[string]any{
			"tool_choice": map[string]any{
				"type": "tool",
				"name": "read_file",
			},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestHTTPAdapterAnthropicImageURLMapping(t *testing.T) {
	const imageData = "aGVsbG8="
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		msgs, ok := body["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Fatalf("expected messages array, got %#v", body["messages"])
		}
		msg0, ok := msgs[0].(map[string]any)
		if !ok {
			t.Fatalf("expected first message object, got %#v", msgs[0])
		}
		content, ok := msg0["content"].([]any)
		if !ok || len(content) < 2 {
			t.Fatalf("expected multimodal content, got %#v", msg0["content"])
		}
		imageBlock, ok := content[1].(map[string]any)
		if !ok {
			t.Fatalf("expected image block map, got %#v", content[1])
		}
		if typ, _ := imageBlock["type"].(string); typ != "image" {
			t.Fatalf("expected anthropic image block, got %#v", imageBlock["type"])
		}
		source, ok := imageBlock["source"].(map[string]any)
		if !ok {
			t.Fatalf("expected image source object, got %#v", imageBlock["source"])
		}
		if sourceType, _ := source["type"].(string); sourceType != "base64" {
			t.Fatalf("expected base64 source type, got %#v", source["type"])
		}
		if mediaType, _ := source["media_type"].(string); mediaType != "image/png" {
			t.Fatalf("unexpected media_type: %q", mediaType)
		}
		if data, _ := source["data"].(string); data != imageData {
			t.Fatalf("unexpected image data: %q", data)
		}

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "an-image",
		Kind:    AdapterKindAnthropic,
		BaseURL: server.URL,
		APIKey:  "ant-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "claude-test",
		MaxTokens: 128,
		Messages: []orchestrator.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "describe image",
					},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64," + imageData,
						},
					},
				},
			},
		},
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestHTTPAdapterAnthropicBlocksLocalImageFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		msgs, ok := body["messages"].([]any)
		if !ok || len(msgs) == 0 {
			t.Fatalf("expected messages array, got %#v", body["messages"])
		}
		msg0, ok := msgs[0].(map[string]any)
		if !ok {
			t.Fatalf("expected first message object, got %#v", msgs[0])
		}
		content, ok := msg0["content"].([]any)
		if !ok || len(content) < 2 {
			t.Fatalf("expected multimodal content, got %#v", msg0["content"])
		}
		imageBlock, ok := content[1].(map[string]any)
		if !ok {
			t.Fatalf("expected image block map, got %#v", content[1])
		}
		if typ, _ := imageBlock["type"].(string); typ != "image_url" {
			t.Fatalf("expected blocked host to remain image_url block, got %#v", imageBlock["type"])
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"model":"claude-test",
			"content":[{"type":"text","text":"ok"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":1}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "an-image-local",
		Kind:    AdapterKindAnthropic,
		BaseURL: server.URL,
		APIKey:  "ant-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	_, err = adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "claude-test",
		MaxTokens: 128,
		Messages: []orchestrator.Message{
			{
				Role: "user",
				Content: []any{
					map[string]any{
						"type": "text",
						"text": "describe image",
					},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "http://127.0.0.1:9999/private.png",
						},
					},
				},
			},
		},
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
}

func TestHTTPAdapterCanonical(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/complete" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"blocks":[{"type":"text","text":"from canonical"}],
			"stop_reason":"end_turn",
			"usage":{"input_tokens":1,"output_tokens":2}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "ca",
		Kind:    AdapterKindCanonical,
		BaseURL: server.URL,
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	resp, err := adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "custom-model",
		MaxTokens: 32,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "x"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.Blocks) != 1 || !strings.Contains(resp.Blocks[0].Text, "canonical") {
		t.Fatalf("unexpected blocks: %+v", resp.Blocks)
	}
}

func TestHTTPAdapterGemini(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gem-model:generateContent" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "gem-key" {
			t.Fatalf("unexpected gemini key header: %q", got)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"candidates":[
				{
					"finishReason":"STOP",
					"content":{"parts":[{"text":"hello gemini"}]}
				}
			],
			"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3}
		}`))
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "gem",
		Kind:    AdapterKindGemini,
		BaseURL: server.URL,
		Model:   "gem-model",
		APIKey:  "gem-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	resp, err := adapter.Complete(context.Background(), orchestrator.Request{
		Model:     "ignored-client-model",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hello gemini"},
		},
	})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if len(resp.Blocks) == 0 || !strings.Contains(resp.Blocks[0].Text, "gemini") {
		t.Fatalf("unexpected blocks: %+v", resp.Blocks)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("unexpected stop reason: %q", resp.StopReason)
	}
}

func TestHTTPAdapterAnthropicStreamPassThrough(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte(`data: {"type":"message_start","message":{"model":"claude-upstream"}}` + "\n\n"))
		_, _ = w.Write([]byte("event: message_stop\n"))
		_, _ = w.Write([]byte(`data: {"type":"message_stop"}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "an-stream",
		Kind:    AdapterKindAnthropic,
		BaseURL: server.URL,
		APIKey:  "ant-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	events, errs := adapter.Stream(context.Background(), orchestrator.Request{
		Model:     "claude-upstream",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	})

	got := []orchestrator.StreamEvent{}
	for ev := range events {
		got = append(got, ev)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("unexpected stream error: %v", err)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 stream events, got %d", len(got))
	}
	if !got[0].PassThrough || got[0].RawEvent != "message_start" {
		t.Fatalf("unexpected first stream event: %+v", got[0])
	}
	if !strings.Contains(string(got[0].RawData), `"message_start"`) {
		t.Fatalf("unexpected raw data: %s", string(got[0].RawData))
	}
}

func TestHTTPAdapterOpenAIStreamToAnthropicEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		writer := bufio.NewWriter(w)

		_, _ = fmt.Fprintln(writer, `data: {"choices":[{"delta":{"content":"Hel"},"finish_reason":null}]}`)
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, `data: {"choices":[{"delta":{"content":"lo"},"finish_reason":null}]}`)
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a"}}]},"finish_reason":null}]}`)
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"bc\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":11,"completion_tokens":5}}`)
		_, _ = fmt.Fprintln(writer)
		_, _ = fmt.Fprintln(writer, `data: [DONE]`)
		_, _ = fmt.Fprintln(writer)
		_ = writer.Flush()
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	adapter, err := NewHTTPAdapter(HTTPAdapterConfig{
		Name:    "oa-stream",
		Kind:    AdapterKindOpenAI,
		BaseURL: server.URL,
		APIKey:  "test-key",
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}

	events, errs := adapter.Stream(context.Background(), orchestrator.Request{
		Model:     "gpt-test",
		MaxTokens: 64,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "hi"},
		},
		Metadata: map[string]any{
			"strict_stream_passthrough": true,
		},
	})

	var got []orchestrator.StreamEvent
	for ev := range events {
		got = append(got, ev)
	}
	for err := range errs {
		if err != nil {
			t.Fatalf("stream failed: %v", err)
		}
	}

	if len(got) < 8 {
		t.Fatalf("expected at least 8 events, got %d", len(got))
	}
	if got[0].Type != "message_start" {
		t.Fatalf("unexpected first event: %+v", got[0])
	}
	foundTextDelta := false
	foundToolStart := false
	foundToolDeltaA := false
	foundToolDeltaB := false
	foundMessageDelta := false
	for _, ev := range got {
		if ev.Type == "content_block_delta" && ev.DeltaText == "Hel" {
			foundTextDelta = true
		}
		if ev.Type == "content_block_start" && ev.Block.Type == "tool_use" && ev.Block.Name == "read_file" {
			foundToolStart = true
		}
		if ev.Type == "content_block_delta" && strings.Contains(ev.DeltaJSON, `{"path":"a`) {
			foundToolDeltaA = true
		}
		if ev.Type == "content_block_delta" && strings.Contains(ev.DeltaJSON, `bc"}`) {
			foundToolDeltaB = true
		}
		if ev.Type == "message_delta" && ev.StopReason == "tool_use" && ev.Usage.InputTokens == 11 && ev.Usage.OutputTokens == 5 {
			foundMessageDelta = true
		}
	}
	if !foundTextDelta {
		t.Fatalf("missing text delta event: %+v", got)
	}
	if !foundToolStart {
		t.Fatalf("missing tool_use start event: %+v", got)
	}
	if !foundToolDeltaA || !foundToolDeltaB {
		t.Fatalf("missing tool_use delta json event: %+v", got)
	}
	if !foundMessageDelta {
		t.Fatalf("missing message_delta with usage: %+v", got)
	}
	if got[len(got)-1].Type != "message_stop" {
		t.Fatalf("unexpected last event: %+v", got[len(got)-1])
	}
}
