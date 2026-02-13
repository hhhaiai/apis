package gateway_test

import (
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/modelmap"
	"ccgateway/internal/policy"
	"ccgateway/internal/settings"
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
