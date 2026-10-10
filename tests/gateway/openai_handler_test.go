package gateway_test

import (
	"ccgateway/internal/ccevent"
	. "ccgateway/internal/gateway"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
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

func TestOpenAIChatCompletionsGroqSDKAlias(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"hello groq sdk"}],
		"max_tokens":128
	}`
	req := httptest.NewRequest(http.MethodPost, "/openai/v1/chat/completions", strings.NewReader(body))
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
}

func TestOpenAIModelsListsConfiguredModels(t *testing.T) {
	t.Setenv("UPSTREAM_MODEL_ROUTES_JSON", `{"GLM-5":["glm-5"],"*":["fallback"]}`)
	t.Setenv("UPSTREAM_ADAPTERS_JSON", `[{"name":"glm-47","kind":"openai","base_url":"http://127.0.0.1:5022","api_key":"free","model":"GLM-4.7"}]`)

	cfg := settings.DefaultRuntimeSettings()
	cfg.ModelMappings = map[string]string{"claude-3-7-sonnet": "GLM-5"}
	cfg.ModelMapFallback = "GLM-4.7"
	cfg.ModeModels = map[string]string{"default": "GLM-5"}
	router := newTestRouterWithDeps(t, Dependencies{
		Settings: settings.NewStore(cfg),
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIModelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal models response: %v", err)
	}
	if resp.Object != "list" {
		t.Fatalf("unexpected object: %q", resp.Object)
	}
	got := map[string]bool{}
	for _, model := range resp.Data {
		got[model.ID] = true
		if model.Object != "model" {
			t.Fatalf("unexpected model object for %q: %q", model.ID, model.Object)
		}
	}
	for _, want := range []string{"GLM-5", "GLM-4.7", "claude-3-7-sonnet"} {
		if !got[want] {
			t.Fatalf("missing model %q in %#v", want, resp.Data)
		}
	}
	if got["*"] {
		t.Fatalf("wildcard model should not be exposed: %#v", resp.Data)
	}
}

func TestOpenAIModelsGroqSDKAlias(t *testing.T) {
	t.Setenv("UPSTREAM_MODEL_ROUTES_JSON", `{"GLM-5":["glm-5"]}`)
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIModelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal models response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "GLM-5" {
		t.Fatalf("expected GLM-5 model through alias, got %#v", resp.Data)
	}
}

func TestOpenAIModelRetrieve(t *testing.T) {
	router := newTestRouter(t)

	for _, path := range []string{"/v1/models/GLM-5", "/openai/v1/models/GLM-4.7", "/v1/models/qwen%2Fcoder"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s: expected status 200, got %d; body=%s", path, rr.Code, rr.Body.String())
		}
		var resp OpenAIModel
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("%s: unmarshal model response: %v", path, err)
		}
		if resp.Object != "model" {
			t.Fatalf("%s: unexpected object: %q", path, resp.Object)
		}
		if strings.Contains(path, "qwen") {
			if resp.ID != "qwen/coder" {
				t.Fatalf("%s: expected decoded model id, got %q", path, resp.ID)
			}
		} else if strings.TrimSpace(resp.ID) == "" {
			t.Fatalf("%s: expected non-empty model id", path)
		}
	}
}

func TestOpenAIModelsFallbackAndMethod(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp OpenAIModelsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal models response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "default" {
		t.Fatalf("expected default fallback model, got %#v", resp.Data)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d; body=%s", rr.Code, rr.Body.String())
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

func TestOpenAIChatCompletionsPreservesResponseFormatAndMaxCompletionTokens(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"return json"}],
		"max_completion_tokens":321,
		"response_format":{"type":"json_object"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if svc.capturedReq.MaxTokens != 321 {
		t.Fatalf("expected max_completion_tokens to drive MaxTokens, got %d", svc.capturedReq.MaxTokens)
	}
	responseFormat, ok := svc.capturedReq.Metadata["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("expected response_format metadata map, got %#v", svc.capturedReq.Metadata["response_format"])
	}
	if responseFormat["type"] != "json_object" {
		t.Fatalf("unexpected response_format: %#v", responseFormat)
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
	if !strings.Contains(bodyStr, `"type":"response.output_item.added"`) {
		t.Fatalf("expected response.output_item.added event")
	}
	if !strings.Contains(bodyStr, `"type":"response.content_part.added"`) {
		t.Fatalf("expected response.content_part.added event")
	}
	if !strings.Contains(bodyStr, `"type":"response.completed"`) {
		t.Fatalf("expected response.completed event")
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatalf("expected [DONE] sentinel")
	}
}

func TestOpenAIResponsesStreamToolCallEmitsCodexOutputItemDone(t *testing.T) {
	svc := &responsesStreamingToolCallService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})

	body := `{
		"model":"claude-test",
		"input":"please call a tool",
		"max_output_tokens":128,
		"stream":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	bodyStr := rr.Body.String()
	for _, marker := range []string{
		`"type":"response.output_item.added"`,
		`"type":"response.function_call_arguments.delta"`,
		`"type":"response.output_item.done"`,
		`"type":"function_call"`,
		`"name":"read_file"`,
		`"call_id":"call_read"`,
		`"arguments":"{\"path\":\"abc\"}"`,
	} {
		if !strings.Contains(bodyStr, marker) {
			t.Fatalf("missing marker %s in stream body: %s", marker, bodyStr)
		}
	}
}

func TestOpenAIResponsesStreamErrorEmitsResponseFailedAndDone(t *testing.T) {
	svc := &responsesStreamingErrorService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})

	body := `{
		"model":"claude-test",
		"input":"please stream fail",
		"stream":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	bodyStr := rr.Body.String()
	for _, marker := range []string{
		`"type":"response.created"`,
		`"type":"response.failed"`,
		`"status":"failed"`,
		`"code":"stream_error"`,
		"upstream stream broke",
		"data: [DONE]",
	} {
		if !strings.Contains(bodyStr, marker) {
			t.Fatalf("missing marker %s in stream body: %s", marker, bodyStr)
		}
	}
}

type responsesStreamingToolCallService struct{}

func (s *responsesStreamingToolCallService) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "unused"},
		},
		StopReason: "end_turn",
	}, nil
}

func (s *responsesStreamingToolCallService) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent, 8)
	errs := make(chan error, 1)
	events <- orchestrator.StreamEvent{Type: "message_start"}
	events <- orchestrator.StreamEvent{
		Type:  "content_block_start",
		Index: 0,
		Block: orchestrator.AssistantBlock{
			Type: "tool_use",
			ID:   "call_read",
			Name: "read_file",
		},
	}
	events <- orchestrator.StreamEvent{Type: "content_block_delta", Index: 0, DeltaJSON: `{"path":"a`}
	events <- orchestrator.StreamEvent{Type: "content_block_delta", Index: 0, DeltaJSON: `bc"}`}
	events <- orchestrator.StreamEvent{Type: "content_block_stop", Index: 0}
	events <- orchestrator.StreamEvent{
		Type:       "message_delta",
		StopReason: "tool_use",
		Usage:      orchestrator.Usage{InputTokens: 2, OutputTokens: 3},
	}
	events <- orchestrator.StreamEvent{Type: "message_stop"}
	close(events)
	close(errs)
	return events, errs
}

type responsesStreamingErrorService struct{}

func (s *responsesStreamingErrorService) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	return orchestrator.Response{Model: req.Model}, nil
}

func (s *responsesStreamingErrorService) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent)
	errs := make(chan error, 1)
	errs <- errors.New("upstream stream broke")
	close(errs)
	return events, errs
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

func TestOpenAIResponsesAcceptsCodexNamespaceTools(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})

	body := `{
		"model":"claude-test",
		"instructions":"You are a coding agent.",
		"max_output_tokens":128,
		"parallel_tool_calls":true,
		"previous_response_id":"resp_prev",
		"reasoning":{"effort":"low"},
		"text":{"format":{"type":"text"}},
		"service_tier":"priority",
		"prompt_cache_key":"cache-key",
		"safety_identifier":"safe-user",
		"top_logprobs":2,
		"background":false,
		"conversation":{"id":"conv_1"},
		"max_tool_calls":4,
		"client_metadata":{"trace":"abc"},
		"tools":[
			{
				"type":"function",
				"name":"exec_command",
				"description":"Run a command",
				"strict":false,
				"parameters":{"type":"object","properties":{"cmd":{"type":"string"}}}
			},
			{
				"type":"namespace",
				"name":"mcp__demo__",
				"description":"Demo tools",
				"tools":[
					{
						"type":"function",
						"name":"lookup_order",
						"description":"Look up an order",
						"strict":false,
						"parameters":{"type":"object","properties":{"order_id":{"type":"string"}}}
					}
				]
			},
			{
				"type":"web_search",
				"external_web_access":false
			}
		],
		"input":[{"role":"user","content":"lookup order 42"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if len(svc.capturedReq.Tools) != 2 {
		t.Fatalf("expected two canonical tools, got %d", len(svc.capturedReq.Tools))
	}
	rootTool := svc.capturedReq.Tools[0]
	if rootTool.Name != "exec_command" {
		t.Fatalf("unexpected root function tool name: %q", rootTool.Name)
	}
	tool := svc.capturedReq.Tools[1]
	if tool.Name != "mcp__demo__lookup_order" {
		t.Fatalf("unexpected namespaced tool name: %q", tool.Name)
	}
	if !strings.Contains(tool.Description, "Demo tools") || !strings.Contains(tool.Description, "Look up an order") {
		t.Fatalf("expected namespace and child descriptions, got %q", tool.Description)
	}
	if system, _ := svc.capturedReq.System.(string); system != "You are a coding agent." {
		t.Fatalf("expected instructions as system prompt, got %#v", svc.capturedReq.System)
	}
	if prev, _ := svc.capturedReq.Metadata["previous_response_id"].(string); prev != "resp_prev" {
		t.Fatalf("expected previous_response_id metadata, got %#v", svc.capturedReq.Metadata["previous_response_id"])
	}
	if parallel, _ := svc.capturedReq.Metadata["parallel_tool_calls"].(bool); !parallel {
		t.Fatalf("expected parallel_tool_calls metadata, got %#v", svc.capturedReq.Metadata["parallel_tool_calls"])
	}
	if _, ok := svc.capturedReq.Metadata["reasoning"].(map[string]any); !ok {
		t.Fatalf("expected reasoning metadata, got %#v", svc.capturedReq.Metadata["reasoning"])
	}
	if _, ok := svc.capturedReq.Metadata["text"].(map[string]any); !ok {
		t.Fatalf("expected text metadata, got %#v", svc.capturedReq.Metadata["text"])
	}
	if tier, _ := svc.capturedReq.Metadata["service_tier"].(string); tier != "priority" {
		t.Fatalf("expected service_tier metadata, got %#v", svc.capturedReq.Metadata["service_tier"])
	}
	if cacheKey, _ := svc.capturedReq.Metadata["prompt_cache_key"].(string); cacheKey != "cache-key" {
		t.Fatalf("expected prompt_cache_key metadata, got %#v", svc.capturedReq.Metadata["prompt_cache_key"])
	}
	if safetyID, _ := svc.capturedReq.Metadata["safety_identifier"].(string); safetyID != "safe-user" {
		t.Fatalf("expected safety_identifier metadata, got %#v", svc.capturedReq.Metadata["safety_identifier"])
	}
	if topLogprobs, ok := metadataNumberAsInt(svc.capturedReq.Metadata["top_logprobs"]); !ok || topLogprobs != 2 {
		t.Fatalf("expected top_logprobs metadata, got %#v", svc.capturedReq.Metadata["top_logprobs"])
	}
	if background, ok := svc.capturedReq.Metadata["background"].(bool); !ok || background {
		t.Fatalf("expected background=false metadata, got %#v", svc.capturedReq.Metadata["background"])
	}
	if _, ok := svc.capturedReq.Metadata["conversation"].(map[string]any); !ok {
		t.Fatalf("expected conversation metadata, got %#v", svc.capturedReq.Metadata["conversation"])
	}
	if maxToolCalls, ok := metadataNumberAsInt(svc.capturedReq.Metadata["max_tool_calls"]); !ok || maxToolCalls != 4 {
		t.Fatalf("expected max_tool_calls metadata, got %#v", svc.capturedReq.Metadata["max_tool_calls"])
	}
	if _, ok := svc.capturedReq.Metadata["client_metadata"].(map[string]any); !ok {
		t.Fatalf("expected client_metadata, got %#v", svc.capturedReq.Metadata["client_metadata"])
	}
}

func metadataNumberAsInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), n == float64(int(n))
	default:
		return 0, false
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

func TestOpenAIResponsesInputCodexCustomToolHistoryMapping(t *testing.T) {
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
			{"type":"custom_tool_call","call_id":"call_exec","name":"exec","input":"echo hi"},
			{"type":"custom_tool_call_output","call_id":"call_exec","output":[{"type":"input_text","text":"hi\n"}]},
			{"type":"local_shell_call","call_id":"call_shell","action":{"type":"exec","command":["pwd"]}},
			{"type":"function_call_output","call_id":"call_shell","output":"ok"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	if len(svc.capturedReq.Messages) != 4 {
		t.Fatalf("expected 4 canonical messages, got %d", len(svc.capturedReq.Messages))
	}
	firstBlocks, ok := svc.capturedReq.Messages[0].Content.([]any)
	if !ok || len(firstBlocks) == 0 {
		t.Fatalf("expected custom tool_use content, got %#v", svc.capturedReq.Messages[0].Content)
	}
	customTool, _ := firstBlocks[0].(map[string]any)
	if customTool["type"] != "tool_use" || customTool["id"] != "call_exec" || customTool["name"] != "exec" {
		t.Fatalf("unexpected custom tool mapping: %#v", customTool)
	}
	customInput, ok := customTool["input"].(map[string]any)
	if !ok || customInput["_raw"] != "echo hi" {
		t.Fatalf("expected custom raw input preserved, got %#v", customTool["input"])
	}

	customOutputBlocks, ok := svc.capturedReq.Messages[1].Content.([]any)
	if !ok || len(customOutputBlocks) == 0 {
		t.Fatalf("expected custom tool_result content, got %#v", svc.capturedReq.Messages[1].Content)
	}
	customOutput, _ := customOutputBlocks[0].(map[string]any)
	if customOutput["type"] != "tool_result" || customOutput["tool_use_id"] != "call_exec" {
		t.Fatalf("unexpected custom output mapping: %#v", customOutput)
	}
	if content, _ := customOutput["content"].(string); !strings.Contains(content, "hi") {
		t.Fatalf("expected custom output text preserved, got %#v", customOutput["content"])
	}

	shellBlocks, ok := svc.capturedReq.Messages[2].Content.([]any)
	if !ok || len(shellBlocks) == 0 {
		t.Fatalf("expected local shell tool_use content, got %#v", svc.capturedReq.Messages[2].Content)
	}
	shellTool, _ := shellBlocks[0].(map[string]any)
	if shellTool["type"] != "tool_use" || shellTool["id"] != "call_shell" || shellTool["name"] != "local_shell" {
		t.Fatalf("unexpected local shell mapping: %#v", shellTool)
	}
	shellInput, ok := shellTool["input"].(map[string]any)
	if !ok || shellInput["type"] != "exec" {
		t.Fatalf("expected shell action input preserved, got %#v", shellTool["input"])
	}

	shellOutputBlocks, ok := svc.capturedReq.Messages[3].Content.([]any)
	if !ok || len(shellOutputBlocks) == 0 {
		t.Fatalf("expected shell tool_result content, got %#v", svc.capturedReq.Messages[3].Content)
	}
	shellOutput, _ := shellOutputBlocks[0].(map[string]any)
	if shellOutput["type"] != "tool_result" || shellOutput["tool_use_id"] != "call_shell" {
		t.Fatalf("unexpected shell output mapping: %#v", shellOutput)
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
