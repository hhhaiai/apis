package gateway_test

import (
	. "ccgateway/internal/gateway"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/settings"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	return newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})
}

func newTestRouterWithDeps(t *testing.T, deps Dependencies) http.Handler {
	t.Helper()
	if deps.Orchestrator == nil {
		deps.Orchestrator = orchestrator.NewSimpleService()
	}
	if deps.Policy == nil {
		deps.Policy = policy.NewNoopEngine()
	}
	if deps.ModelMapper == nil {
		deps.ModelMapper = modelmap.NewIdentityMapper()
	}
	return NewRouter(Dependencies{
		Orchestrator: deps.Orchestrator,
		Policy:       deps.Policy,
		ModelMapper:  deps.ModelMapper,
		Settings:     deps.Settings,
		ToolCatalog:  deps.ToolCatalog,
		SessionStore: deps.SessionStore,
		RunStore:     deps.RunStore,
		TodoStore:    deps.TodoStore,
		PlanStore:    deps.PlanStore,
		EventStore:   deps.EventStore,
		MCPRegistry:  deps.MCPRegistry,
		AdminToken:   deps.AdminToken,
	})
}

func TestMessagesNonStream(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello gateway"}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Type != "message" {
		t.Fatalf("expected type=message, got %q", resp.Type)
	}
	if resp.Role != "assistant" {
		t.Fatalf("expected role=assistant, got %q", resp.Role)
	}
	if len(resp.Content) == 0 {
		t.Fatalf("expected non-empty content blocks")
	}
}

func TestMessagesMissingAnthropicVersion(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hi"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}

	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error envelope: %v", err)
	}
	if env.Type != "error" || env.Error.Type != "invalid_request_error" {
		t.Fatalf("unexpected error envelope: %+v", env)
	}
}

func TestMessagesStreamSequence(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"stream":true,
		"messages":[{"role":"user","content":"stream this please"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", ct)
	}

	payload := rr.Body.String()
	order := []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	}
	last := -1
	for _, marker := range order {
		i := strings.Index(payload, marker)
		if i < 0 {
			t.Fatalf("missing stream marker: %s", marker)
		}
		if i < last {
			t.Fatalf("stream marker out of order: %s", marker)
		}
		last = i
	}
}

func TestMessagesToolUse(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"please call a tool for weather"}],
		"tools":[
			{
				"name":"get_weather",
				"description":"Get weather",
				"input_schema":{"type":"object","properties":{"query":{"type":"string"}}}
			}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != "tool_use" {
		t.Fatalf("expected stop_reason=tool_use, got %q", resp.StopReason)
	}
	if len(resp.Content) == 0 || resp.Content[0].Type != "tool_use" {
		t.Fatalf("expected first block tool_use, got %+v", resp.Content)
	}
}

func TestCountTokens(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[
			{"role":"user","content":"one two three"},
			{"role":"assistant","content":"four"}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var resp CountTokensResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.InputTokens < 4 {
		t.Fatalf("expected input_tokens >= 4, got %d", resp.InputTokens)
	}
}

type captureService struct {
	capturedModel string
	capturedReq   orchestrator.Request
}

type toolLoopService struct {
	calls         int
	sawToolResult bool
	alwaysToolUse bool
	toolName      string
}

func (s *captureService) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	s.capturedModel = req.Model
	s.capturedReq = req
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "ok"},
		},
		StopReason: "end_turn",
		Usage: orchestrator.Usage{
			InputTokens:  1,
			OutputTokens: 1,
		},
	}, nil
}

func (s *toolLoopService) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	s.calls++
	toolName := strings.TrimSpace(s.toolName)
	if toolName == "" {
		toolName = "get_weather"
	}
	if s.calls == 1 || s.alwaysToolUse {
		return orchestrator.Response{
			Model: req.Model,
			Blocks: []orchestrator.AssistantBlock{
				{
					Type: "tool_use",
					ID:   "toolu_1",
					Name: toolName,
					Input: map[string]any{
						"city": "Beijing",
					},
				},
			},
			StopReason: "tool_use",
			Usage: orchestrator.Usage{
				InputTokens:  1,
				OutputTokens: 1,
			},
		}, nil
	}
	s.sawToolResult = containsToolResult(req.Messages, "toolu_1")
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "server tool loop done"},
		},
		StopReason: "end_turn",
		Usage: orchestrator.Usage{
			InputTokens:  2,
			OutputTokens: 3,
		},
	}, nil
}

func (s *toolLoopService) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent)
	errs := make(chan error)
	close(events)
	close(errs)
	return events, errs
}

type passThroughService struct{}

func (s *passThroughService) Complete(_ context.Context, _ orchestrator.Request) (orchestrator.Response, error) {
	return orchestrator.Response{
		Model: "unused",
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "unused"},
		},
		StopReason: "end_turn",
		Usage: orchestrator.Usage{
			InputTokens:  1,
			OutputTokens: 1,
		},
	}, nil
}

func (s *passThroughService) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent, 4)
	errs := make(chan error, 1)

	events <- orchestrator.StreamEvent{
		Type:        "message_start",
		RawEvent:    "message_start",
		RawData:     []byte(`{"type":"message_start","message":{"id":"msg_passthrough","type":"message","role":"assistant","model":"mapped-model","content":[],"usage":{"input_tokens":1,"output_tokens":0}}}`),
		PassThrough: true,
	}
	events <- orchestrator.StreamEvent{
		Type:        "message_stop",
		RawEvent:    "message_stop",
		RawData:     []byte(`{"type":"message_stop"}`),
		PassThrough: true,
	}

	close(events)
	close(errs)
	return events, errs
}

func (s *captureService) Stream(_ context.Context, req orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	s.capturedModel = req.Model
	s.capturedReq = req
	events := make(chan orchestrator.StreamEvent, 2)
	errs := make(chan error, 1)
	events <- orchestrator.StreamEvent{Type: "message_start"}
	events <- orchestrator.StreamEvent{Type: "message_stop"}
	close(events)
	close(errs)
	return events, errs
}

func containsToolResult(messages []orchestrator.Message, toolUseID string) bool {
	toolUseID = strings.TrimSpace(toolUseID)
	for _, m := range messages {
		blocks, ok := m.Content.([]any)
		if !ok {
			continue
		}
		for _, item := range blocks {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			t, _ := block["type"].(string)
			if t != "tool_result" {
				continue
			}
			id, _ := block["tool_use_id"].(string)
			if id == toolUseID {
				return true
			}
		}
	}
	return false
}

func TestMessagesModelMappingUsesUpstreamModel(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper: modelmap.NewStaticMapper(map[string]string{
			"claude-test": "upstream/model-A",
		}, true, ""),
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello mapping"}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("x-cc-requested-model"); got != "claude-test" {
		t.Fatalf("unexpected x-cc-requested-model: %q", got)
	}
	if got := rr.Header().Get("x-cc-upstream-model"); got != "upstream/model-A" {
		t.Fatalf("unexpected x-cc-upstream-model: %q", got)
	}
	if svc.capturedModel != "upstream/model-A" {
		t.Fatalf("expected orchestrator model mapping applied, got %q", svc.capturedModel)
	}

	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Model != "claude-test" {
		t.Fatalf("expected outward response model to keep requested model, got %q", resp.Model)
	}
}

func TestMessagesModelMappingStrictRejectsUnknownModel(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper: modelmap.NewStaticMapper(map[string]string{
			"known-model": "upstream/model",
		}, true, ""),
	})

	body := `{
		"model":"unknown-model",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}

	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error envelope: %v", err)
	}
	if env.Error.Type != "invalid_request_error" {
		t.Fatalf("unexpected error type: %s", env.Error.Type)
	}
}

func TestMessagesStrictPassThroughStreamRewritesOutwardModel(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: &passThroughService{},
		Policy:       policy.NewNoopEngine(),
		ModelMapper: modelmap.NewStaticMapper(map[string]string{
			"claude-test": "mapped-model",
		}, true, ""),
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"stream":true,
		"messages":[{"role":"user","content":"hello strict stream"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	payload := rr.Body.String()
	if !strings.Contains(payload, `event: message_start`) {
		t.Fatalf("expected passthrough message_start event")
	}
	if !strings.Contains(payload, `"model":"claude-test"`) {
		t.Fatalf("expected outward model rewrite to requested model, payload=%s", payload)
	}
	if strings.Contains(payload, `"model":"mapped-model"`) {
		t.Fatalf("did not expect mapped upstream model in outward stream payload")
	}
}

func TestMessagesModeSettingsApplied(t *testing.T) {
	svc := &captureService{}
	st := settings.NewStore(settings.RuntimeSettings{
		UseModeModelOverride: true,
		ModeModels: map[string]string{
			"plan": "planner-model",
		},
		PromptPrefixes: map[string]string{
			"plan": "PLAN FIRST",
		},
		AllowExperimentalTools: false,
		AllowUnknownTools:      true,
		Routing: settings.RoutingSettings{
			Retries:             2,
			ReflectionPasses:    4,
			TimeoutMS:           15000,
			ParallelCandidates:  2,
			EnableResponseJudge: true,
			ModeRoutes: map[string][]string{
				"plan": []string{"planner-upstream", "fallback-upstream"},
			},
		},
		ToolLoop: settings.ToolLoopSettings{
			Mode:     "server_loop",
			MaxSteps: 7,
		},
	})
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
	})

	body := `{
		"model":"client-model",
		"max_tokens":128,
		"system":"existing system",
		"messages":[{"role":"user","content":"plan this"}]
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-cc-mode", "plan")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("x-cc-mode"); got != "plan" {
		t.Fatalf("unexpected mode header: %q", got)
	}
	if got := rr.Header().Get("x-cc-client-model"); got != "client-model" {
		t.Fatalf("unexpected client model header: %q", got)
	}
	if got := rr.Header().Get("x-cc-requested-model"); got != "planner-model" {
		t.Fatalf("unexpected requested model header: %q", got)
	}
	if svc.capturedModel != "planner-model" {
		t.Fatalf("expected model overridden by mode settings, got %q", svc.capturedModel)
	}
	if sys, _ := svc.capturedReq.System.(string); !strings.Contains(sys, "PLAN FIRST") {
		t.Fatalf("expected prompt prefix injected into system, got %q", sys)
	}
	if retries, ok := svc.capturedReq.Metadata["routing_retries"].(int); !ok || retries != 2 {
		t.Fatalf("expected routing retries=2 in metadata, got %#v", svc.capturedReq.Metadata["routing_retries"])
	}
	route, ok := svc.capturedReq.Metadata["routing_adapter_route"].([]string)
	if !ok || len(route) != 2 || route[0] != "planner-upstream" {
		t.Fatalf("unexpected routing_adapter_route metadata: %#v", svc.capturedReq.Metadata["routing_adapter_route"])
	}
	if pc, ok := svc.capturedReq.Metadata["parallel_candidates"].(int); !ok || pc != 2 {
		t.Fatalf("expected parallel_candidates=2 in metadata, got %#v", svc.capturedReq.Metadata["parallel_candidates"])
	}
	if ej, ok := svc.capturedReq.Metadata["enable_response_judge"].(bool); !ok || !ej {
		t.Fatalf("expected enable_response_judge=true in metadata, got %#v", svc.capturedReq.Metadata["enable_response_judge"])
	}
	if tl, ok := svc.capturedReq.Metadata["tool_loop_mode"].(string); !ok || tl != "server_loop" {
		t.Fatalf("expected tool_loop_mode=server_loop, got %#v", svc.capturedReq.Metadata["tool_loop_mode"])
	}
	if ts, ok := svc.capturedReq.Metadata["tool_loop_max_steps"].(int); !ok || ts != 7 {
		t.Fatalf("expected tool_loop_max_steps=7, got %#v", svc.capturedReq.Metadata["tool_loop_max_steps"])
	}
}

func TestMessagesServerSideToolLoop(t *testing.T) {
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
		"max_tokens":128,
		"messages":[{"role":"user","content":"please use tool"}],
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("expected end_turn after loop, got %q", resp.StopReason)
	}
	if svc.calls != 2 {
		t.Fatalf("expected 2 complete calls, got %d", svc.calls)
	}
	if !svc.sawToolResult {
		t.Fatalf("expected tool_result injected into second round request")
	}
}

func TestMessagesServerSideToolLoopMaxTurns(t *testing.T) {
	svc := &toolLoopService{alwaysToolUse: true}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolLoop.MaxSteps = 2
	st := settings.NewStore(cfg)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"keep calling tool"}],
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != "max_turns" {
		t.Fatalf("expected max_turns stop, got %q", resp.StopReason)
	}
	if svc.calls != 2 {
		t.Fatalf("expected calls capped at 2, got %d", svc.calls)
	}
}

func TestMessagesServerSideToolLoopWithMCPFallback(t *testing.T) {
	mcpRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		id := req["id"]
		method, _ := req["method"].(string)
		switch method {
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "remote_search"},
					},
				},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"isError": false,
					"content": "ok-from-mcp",
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error":   map[string]any{"message": "unsupported"},
			})
		}
	}))
	defer mcpRPC.Close()

	registry := mcpregistry.NewStore(mcpRPC.Client())
	_, err := registry.Register(mcpregistry.RegisterInput{
		ID:        "mcp_remote_1",
		Name:      "remote-1",
		Transport: mcpregistry.TransportHTTP,
		URL:       mcpRPC.URL,
		TimeoutMS: 2000,
		Retries:   1,
	})
	if err != nil {
		t.Fatalf("register mcp server: %v", err)
	}

	svc := &toolLoopService{toolName: "remote_search"}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolLoop.MaxSteps = 3
	st := settings.NewStore(cfg)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		MCPRegistry:  registry,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"please use remote tool"}],
		"tools":[{"name":"remote_search","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.StopReason != "end_turn" {
		t.Fatalf("expected end_turn, got %q", resp.StopReason)
	}
	if svc.calls != 2 {
		t.Fatalf("expected two turns, got %d", svc.calls)
	}
	if !svc.sawToolResult {
		t.Fatalf("expected MCP tool_result injected")
	}
}
