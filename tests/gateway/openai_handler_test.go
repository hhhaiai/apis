package gateway_test

import (
	"ccgateway/internal/ccevent"
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/modelmap"
	"ccgateway/internal/policy"
	"ccgateway/internal/settings"
	"ccgateway/internal/upstream"
)

func TestOpenAIChatCompletionsNonStream(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"hello openai"}],
		"max_tokens":128
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIChatCompletionsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Object != "chat.completion" {
		t.Fatalf("unexpected object: %q", resp.Object)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected one choice")
	}
	if resp.Choices[0].Message.Role != "assistant" {
		t.Fatalf("unexpected role: %q", resp.Choices[0].Message.Role)
	}
}

func TestOpenAIChatCompletionsRejectTrailingJSON(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"hello openai"}],
		"max_tokens":128
	} {}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestOpenAIChatCompletionsAllowUnknownTopLevelFields(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"hello openai"}],
		"max_tokens":128,
		"x_extra":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestOpenAIChatCompletionsToolCalls(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"please use tool"}],
		"max_tokens":128,
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"get_weather",
					"description":"Get weather",
					"parameters":{"type":"object","properties":{"query":{"type":"string"}}}
				}
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIChatCompletionsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected finish_reason tool_calls, got %q", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) == 0 {
		t.Fatalf("expected tool calls in response")
	}
}

func TestOpenAIChatCompletionsStream(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"stream please"}],
		"max_tokens":128,
		"stream":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	bodyStr := rr.Body.String()
	if !strings.Contains(bodyStr, "chat.completion.chunk") {
		t.Fatalf("expected chunk payload in stream")
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatalf("expected [DONE] sentinel")
	}
}

func TestOpenAIResponsesNonStream(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"input":"hello responses",
		"max_output_tokens":128
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIResponsesResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Object != "response" {
		t.Fatalf("unexpected object: %q", resp.Object)
	}
	if len(resp.Output) == 0 {
		t.Fatalf("expected output items")
	}
}

func TestOpenAIResponsesRejectTrailingJSON(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"input":"hello responses",
		"max_output_tokens":128
	} {}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestOpenAIResponsesAllowUnknownTopLevelFields(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"input":"hello responses",
		"max_output_tokens":128,
		"x_extra":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestOpenAIResponsesStream(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"input":"hello stream responses",
		"max_output_tokens":128,
		"stream":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	bodyStr := rr.Body.String()
	if !strings.Contains(bodyStr, `"type":"response.created"`) {
		t.Fatalf("expected response.created event")
	}
	if !strings.Contains(bodyStr, `"type":"response.completed"`) {
		t.Fatalf("expected response.completed event")
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatalf("expected [DONE] sentinel")
	}
}

func TestOpenAIModelMappingHeaders(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper: modelmap.NewStaticMapper(map[string]string{
			"claude-test": "mapped/provider-model",
		}, true, ""),
	})

	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"hello"}],
		"max_tokens":32
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("x-cc-requested-model"); got != "claude-test" {
		t.Fatalf("unexpected requested model header: %q", got)
	}
	if got := rr.Header().Get("x-cc-upstream-model"); got != "mapped/provider-model" {
		t.Fatalf("unexpected upstream model header: %q", got)
	}
	if svc.capturedModel != "mapped/provider-model" {
		t.Fatalf("expected mapped model in orchestrator, got %q", svc.capturedModel)
	}
}

func TestOpenAIChatCompletionsServerSideToolLoop(t *testing.T) {
	svc := &toolLoopService{}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolLoop.MaxSteps = 3
	st := settings.NewStore(cfg)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
	})

	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"please use tool"}],
		"max_tokens":128,
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"get_weather",
					"description":"Get weather",
					"parameters":{"type":"object","properties":{"city":{"type":"string"}}}
				}
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIChatCompletionsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("expected finish_reason stop after server loop, got %q", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 0 {
		t.Fatalf("expected no outward tool_calls after server loop, got %d", len(resp.Choices[0].Message.ToolCalls))
	}
	if svc.calls != 2 {
		t.Fatalf("expected 2 orchestrator calls, got %d", svc.calls)
	}
}

func TestOpenAIChatCompletionsMapsToolHistoryAndToolChoice(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"tool_choice":{"type":"function","function":{"name":"get_weather"}},
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"get_weather",
					"description":"Get weather",
					"parameters":{"type":"object","properties":{"city":{"type":"string"}}}
				}
			}
		],
		"messages":[
			{"role":"user","content":"weather for beijing"},
			{
				"role":"assistant",
				"content":null,
				"tool_calls":[
					{
						"id":"call_1",
						"type":"function",
						"function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}
					}
				]
			},
			{"role":"tool","tool_call_id":"call_1","content":"{\"temp\":25}"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	tc, ok := svc.capturedReq.Metadata["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_choice in metadata, got %#v", svc.capturedReq.Metadata["tool_choice"])
	}
	if typ, _ := tc["type"].(string); typ != "function" {
		t.Fatalf("unexpected tool_choice type: %q", typ)
	}

	if len(svc.capturedReq.Messages) != 3 {
		t.Fatalf("expected 3 canonical messages, got %d", len(svc.capturedReq.Messages))
	}
	assistant := svc.capturedReq.Messages[1]
	if assistant.Role != "assistant" {
		t.Fatalf("expected assistant role, got %q", assistant.Role)
	}
	assistantBlocks, ok := assistant.Content.([]any)
	if !ok || len(assistantBlocks) == 0 {
		t.Fatalf("expected assistant content blocks, got %#v", assistant.Content)
	}
	toolUse, ok := assistantBlocks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_use block, got %#v", assistantBlocks[0])
	}
	if typ, _ := toolUse["type"].(string); typ != "tool_use" {
		t.Fatalf("unexpected assistant block type: %q", typ)
	}
	if id, _ := toolUse["id"].(string); id != "call_1" {
		t.Fatalf("unexpected tool_use id: %q", id)
	}
	input, ok := toolUse["input"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_use input map, got %#v", toolUse["input"])
	}
	if city, _ := input["city"].(string); city != "Beijing" {
		t.Fatalf("unexpected tool_use input city: %#v", input["city"])
	}

	toolResultMsg := svc.capturedReq.Messages[2]
	if toolResultMsg.Role != "user" {
		t.Fatalf("expected tool result message role=user, got %q", toolResultMsg.Role)
	}
	toolResultBlocks, ok := toolResultMsg.Content.([]any)
	if !ok || len(toolResultBlocks) == 0 {
		t.Fatalf("expected tool_result block content, got %#v", toolResultMsg.Content)
	}
	toolResult, ok := toolResultBlocks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_result block map, got %#v", toolResultBlocks[0])
	}
	if typ, _ := toolResult["type"].(string); typ != "tool_result" {
		t.Fatalf("unexpected tool_result type: %q", typ)
	}
	if id, _ := toolResult["tool_use_id"].(string); id != "call_1" {
		t.Fatalf("unexpected tool_result tool_use_id: %q", id)
	}
}

func TestOpenAIChatCompletionsToolMessageMissingToolCallID(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[
			{"role":"user","content":"hello"},
			{"role":"tool","content":"missing id"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid tool message, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestOpenAIResponsesMapsToolHistoryAndToolChoice(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})

	body := `{
		"model":"claude-test",
		"max_output_tokens":128,
		"tool_choice":{"type":"function","function":{"name":"get_weather"}},
		"temperature":0.2,
		"top_p":0.9,
		"tools":[
			{
				"type":"function",
				"function":{
					"name":"get_weather",
					"description":"Get weather",
					"parameters":{"type":"object","properties":{"city":{"type":"string"}}}
				}
			}
		],
		"input":[
			{"role":"user","content":"weather for beijing"},
			{
				"role":"assistant",
				"content":[{"type":"output_text","text":"calling tool"}],
				"tool_calls":[
					{
						"id":"call_1",
						"type":"function",
						"function":{"name":"get_weather","arguments":"{\"city\":\"Beijing\"}"}
					}
				]
			},
			{"role":"tool","tool_call_id":"call_1","content":"{\"temp\":25}"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	tc, ok := svc.capturedReq.Metadata["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_choice in metadata, got %#v", svc.capturedReq.Metadata["tool_choice"])
	}
	if typ, _ := tc["type"].(string); typ != "function" {
		t.Fatalf("unexpected tool_choice type: %q", typ)
	}
	if temp, _ := svc.capturedReq.Metadata["temperature"].(float64); temp != 0.2 {
		t.Fatalf("unexpected temperature in metadata: %#v", svc.capturedReq.Metadata["temperature"])
	}
	if topP, _ := svc.capturedReq.Metadata["top_p"].(float64); topP != 0.9 {
		t.Fatalf("unexpected top_p in metadata: %#v", svc.capturedReq.Metadata["top_p"])
	}

	if len(svc.capturedReq.Messages) != 3 {
		t.Fatalf("expected 3 canonical messages, got %d", len(svc.capturedReq.Messages))
	}
	assistant := svc.capturedReq.Messages[1]
	assistantBlocks, ok := assistant.Content.([]any)
	if !ok || len(assistantBlocks) == 0 {
		t.Fatalf("expected assistant tool blocks, got %#v", assistant.Content)
	}
	lastBlock, ok := assistantBlocks[len(assistantBlocks)-1].(map[string]any)
	if !ok || lastBlock["type"] != "tool_use" {
		t.Fatalf("expected tool_use block in assistant message, got %#v", assistantBlocks)
	}

	toolResultMsg := svc.capturedReq.Messages[2]
	toolResultBlocks, ok := toolResultMsg.Content.([]any)
	if !ok || len(toolResultBlocks) == 0 {
		t.Fatalf("expected tool_result block content, got %#v", toolResultMsg.Content)
	}
	toolResult, ok := toolResultBlocks[0].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_result block map, got %#v", toolResultBlocks[0])
	}
	if id, _ := toolResult["tool_use_id"].(string); id != "call_1" {
		t.Fatalf("unexpected tool_result tool_use_id: %q", id)
	}
}

func TestOpenAIResponsesInputFunctionCallOutputMapping(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})

	body := `{
		"model":"claude-test",
		"max_output_tokens":128,
		"input":[
			{"type":"function_call","id":"call_2","name":"lookup","arguments":"{\"q\":\"weather\"}"},
			{"type":"function_call_output","call_id":"call_2","output":"{\"ok\":true}"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	if len(svc.capturedReq.Messages) != 2 {
		t.Fatalf("expected 2 canonical messages, got %d", len(svc.capturedReq.Messages))
	}
	assistant := svc.capturedReq.Messages[0]
	assistantBlocks, ok := assistant.Content.([]any)
	if !ok || len(assistantBlocks) == 0 {
		t.Fatalf("expected assistant tool_use content, got %#v", assistant.Content)
	}
	first, _ := assistantBlocks[0].(map[string]any)
	if typ, _ := first["type"].(string); typ != "tool_use" {
		t.Fatalf("unexpected first block type: %q", typ)
	}

	user := svc.capturedReq.Messages[1]
	userBlocks, ok := user.Content.([]any)
	if !ok || len(userBlocks) == 0 {
		t.Fatalf("expected user tool_result content, got %#v", user.Content)
	}
	result, _ := userBlocks[0].(map[string]any)
	if typ, _ := result["type"].(string); typ != "tool_result" {
		t.Fatalf("unexpected result block type: %q", typ)
	}
	if id, _ := result["tool_use_id"].(string); id != "call_2" {
		t.Fatalf("unexpected result block call id: %q", id)
	}
}

func TestOpenAIChatCompletionsVisionFallbackByHint(t *testing.T) {
	svc := &captureService{}
	eventStore := ccevent.NewStore()
	st := settings.NewStore(settings.RuntimeSettings{
		VisionSupportHints: map[string]bool{
			"gpt-3.5-*": false,
		},
	})
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		EventStore:   eventStore,
	})

	body := `{
		"model":"gpt-3.5-turbo",
		"max_tokens":128,
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"请结合图片回答"},
					{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}
				]
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if len(svc.capturedReq.Messages) == 0 {
		t.Fatalf("expected captured request messages")
	}
	user := svc.capturedReq.Messages[0]
	blocks, ok := user.Content.([]any)
	if !ok {
		t.Fatalf("expected user content blocks, got %#v", user.Content)
	}
	hasImage := false
	hasVisionText := false
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := block["type"].(string)
		if typ == "image_url" {
			hasImage = true
		}
		if typ == "text" {
			if text, _ := block["text"].(string); strings.Contains(text, "Vision fallback context") {
				hasVisionText = true
			}
		}
	}
	if hasImage {
		t.Fatalf("expected image block removed after fallback, got %#v", blocks)
	}
	if !hasVisionText {
		t.Fatalf("expected injected vision fallback text, got %#v", blocks)
	}

	events := eventStore.List(ccevent.ListFilter{EventType: "vision.fallback_applied"})
	if len(events) == 0 {
		t.Fatalf("expected vision.fallback_applied event")
	}
}

func TestOpenAIChatCompletionsVisionFallbackCanBeOverridden(t *testing.T) {
	svc := &captureService{}
	st := settings.NewStore(settings.RuntimeSettings{
		VisionSupportHints: map[string]bool{
			"gpt-3.5-*": false,
		},
	})
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
	})

	body := `{
		"model":"gpt-3.5-turbo",
		"max_tokens":128,
		"metadata":{"upstream_supports_vision":true},
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"请结合图片回答"},
					{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}
				]
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if len(svc.capturedReq.Messages) == 0 {
		t.Fatalf("expected captured request messages")
	}
	user := svc.capturedReq.Messages[0]
	blocks, ok := user.Content.([]any)
	if !ok {
		t.Fatalf("expected user content blocks, got %#v", user.Content)
	}
	hasImage := false
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := block["type"].(string)
		if typ == "image_url" {
			hasImage = true
			break
		}
	}
	if !hasImage {
		t.Fatalf("expected image block kept when upstream_supports_vision=true, got %#v", blocks)
	}
}

type captureServiceWithUpstream struct {
	captureService
	upstreamCfg upstream.UpstreamAdminConfig
}

func (s *captureServiceWithUpstream) GetUpstreamConfig() upstream.UpstreamAdminConfig {
	return s.upstreamCfg
}

func TestOpenAIChatCompletionsVisionFallbackByAdapterCapability(t *testing.T) {
	supportsVision := false
	svc := &captureServiceWithUpstream{
		upstreamCfg: upstream.UpstreamAdminConfig{
			Adapters: []upstream.AdapterSpec{
				{Name: "cheap-text", SupportsVision: &supportsVision},
			},
			DefaultRoute: []string{"cheap-text"},
		},
	}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(settings.DefaultRuntimeSettings()),
	})

	body := `{
		"model":"custom-plain-model",
		"max_tokens":128,
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"看图回答"},
					{"type":"image_url","image_url":{"url":"https://example.com/b.png"}}
				]
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	user := svc.capturedReq.Messages[0]
	blocks, ok := user.Content.([]any)
	if !ok {
		t.Fatalf("expected user content blocks, got %#v", user.Content)
	}
	hasImage := false
	hasFallbackText := false
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := block["type"].(string)
		if typ == "image_url" {
			hasImage = true
		}
		if typ == "text" {
			if text, _ := block["text"].(string); strings.Contains(text, "Vision fallback context") {
				hasFallbackText = true
			}
		}
	}
	if hasImage {
		t.Fatalf("expected image removed by adapter capability fallback, got %#v", blocks)
	}
	if !hasFallbackText {
		t.Fatalf("expected fallback text injected, got %#v", blocks)
	}
}

func TestOpenAIChatCompletionsVisionKeepWhenAdapterSupports(t *testing.T) {
	supportsVision := true
	svc := &captureServiceWithUpstream{
		upstreamCfg: upstream.UpstreamAdminConfig{
			Adapters: []upstream.AdapterSpec{
				{Name: "vision-model", SupportsVision: &supportsVision},
			},
			DefaultRoute: []string{"vision-model"},
		},
	}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(settings.DefaultRuntimeSettings()),
	})

	body := `{
		"model":"custom-vision-model",
		"max_tokens":128,
		"messages":[
			{
				"role":"user",
				"content":[
					{"type":"text","text":"看图回答"},
					{"type":"image_url","image_url":{"url":"https://example.com/c.png"}}
				]
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	user := svc.capturedReq.Messages[0]
	blocks, ok := user.Content.([]any)
	if !ok {
		t.Fatalf("expected user content blocks, got %#v", user.Content)
	}
	hasImage := false
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := block["type"].(string)
		if typ == "image_url" {
			hasImage = true
			break
		}
	}
	if !hasImage {
		t.Fatalf("expected image block kept when adapter supports vision, got %#v", blocks)
	}
}

func TestOpenAIChatCompletionsStreamAutoToolFallbackWhenUpstreamUnsupported(t *testing.T) {
	supportsTools := false
	svc := &toolLoopServiceWithUpstream{
		upstreamCfg: upstream.UpstreamAdminConfig{
			Adapters: []upstream.AdapterSpec{
				{Name: "plain-adapter", SupportsTools: &supportsTools},
			},
			DefaultRoute: []string{"plain-adapter"},
		},
	}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "client_loop"
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(cfg),
		EventStore:   eventStore,
	})

	body := `{
		"model":"custom-no-tools-model",
		"messages":[{"role":"user","content":"please use tool"}],
		"max_tokens":128,
		"stream":true,
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	if svc.calls != 2 {
		t.Fatalf("expected server-side tool loop auto enabled for stream (2 calls), got %d", svc.calls)
	}
	bodyStr := rr.Body.String()
	if !strings.Contains(bodyStr, "chat.completion.chunk") {
		t.Fatalf("expected chat.completion.chunk in stream, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "server tool loop done") {
		t.Fatalf("expected tool-loop final text in stream payload, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatalf("expected [DONE] sentinel")
	}
	fallbackEvents := eventStore.List(ccevent.ListFilter{EventType: "tool.fallback_applied"})
	if len(fallbackEvents) == 0 {
		t.Fatalf("expected tool.fallback_applied event")
	}
}

func TestOpenAIResponsesStreamAutoToolFallbackWhenUpstreamUnsupported(t *testing.T) {
	supportsTools := false
	svc := &toolLoopServiceWithUpstream{
		upstreamCfg: upstream.UpstreamAdminConfig{
			Adapters: []upstream.AdapterSpec{
				{Name: "plain-adapter", SupportsTools: &supportsTools},
			},
			DefaultRoute: []string{"plain-adapter"},
		},
	}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "client_loop"
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(cfg),
		EventStore:   eventStore,
	})

	body := `{
		"model":"custom-no-tools-model",
		"input":"hello stream responses",
		"max_output_tokens":128,
		"stream":true,
		"tools":[{"type":"function","function":{"name":"get_weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	if svc.calls != 2 {
		t.Fatalf("expected server-side tool loop auto enabled for stream (2 calls), got %d", svc.calls)
	}
	bodyStr := rr.Body.String()
	if !strings.Contains(bodyStr, `"type":"response.created"`) {
		t.Fatalf("expected response.created event")
	}
	if !strings.Contains(bodyStr, `"type":"response.completed"`) {
		t.Fatalf("expected response.completed event")
	}
	if !strings.Contains(bodyStr, "server tool loop done") {
		t.Fatalf("expected tool-loop final text in stream payload, got %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatalf("expected [DONE] sentinel")
	}
	fallbackEvents := eventStore.List(ccevent.ListFilter{EventType: "tool.fallback_applied"})
	if len(fallbackEvents) == 0 {
		t.Fatalf("expected tool.fallback_applied event")
	}
}
