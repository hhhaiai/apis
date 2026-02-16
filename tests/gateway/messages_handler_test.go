package gateway_test

import (
	"ccgateway/internal/auth"
	"ccgateway/internal/ccevent"
	"ccgateway/internal/channel"
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
	"ccgateway/internal/token"
	"ccgateway/internal/upstream"
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
	return NewRouter(deps)
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

func TestMessagesRejectTrailingJSON(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hi"}]
	} {}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestMessagesAllowUnknownTopLevelFields(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hi"}],
		"x_extra":true
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
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

func TestMessagesToolChoicePropagatedToCanonicalMetadata(t *testing.T) {
	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
	})
	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"hello"}],
		"tools":[
			{
				"name":"get_weather",
				"input_schema":{"type":"object","properties":{"city":{"type":"string"}}}
			}
		],
		"tool_choice":{"type":"tool","name":"get_weather"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	tc, ok := svc.capturedReq.Metadata["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool_choice in metadata, got %#v", svc.capturedReq.Metadata["tool_choice"])
	}
	if typ, _ := tc["type"].(string); typ != "tool" {
		t.Fatalf("unexpected tool_choice type: %q", typ)
	}
	if name, _ := tc["name"].(string); name != "get_weather" {
		t.Fatalf("unexpected tool_choice name: %q", name)
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

func TestCountTokensRejectTrailingJSON(t *testing.T) {
	router := newTestRouter(t)
	body := `{
		"model":"claude-test",
		"messages":[{"role":"user","content":"one two three"}]
	} {}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestMessagesTokenModelRestriction(t *testing.T) {
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate("user-model-restrict", 100)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	allowedModels := "claude-allowed"
	tk.Models = &allowedModels
	if err := tokenSvc.Update(tk); err != nil {
		t.Fatalf("update token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TokenService: tokenSvc,
		AdminToken:   "secret-admin",
	})

	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("authorization", "Bearer "+tk.Value)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if env.Error.Type != "permission_error" {
		t.Fatalf("expected permission_error, got %q", env.Error.Type)
	}
}

func TestMessagesRequireTokenWhenTokenServiceConfiguredWithoutAdminToken(t *testing.T) {
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate("user-auth-required", 100)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TokenService: tokenSvc,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	}`

	reqNoAuth := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	reqNoAuth.Header.Set("anthropic-version", "2023-06-01")
	rrNoAuth := httptest.NewRecorder()
	router.ServeHTTP(rrNoAuth, reqNoAuth)
	if rrNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 without token, got %d; body=%s", rrNoAuth.Code, rrNoAuth.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rrNoAuth.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal unauthorized response: %v", err)
	}
	if env.Error.Type != "auth_error" {
		t.Fatalf("expected auth_error, got %q", env.Error.Type)
	}

	reqAuth := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	reqAuth.Header.Set("anthropic-version", "2023-06-01")
	reqAuth.Header.Set("authorization", "Bearer "+tk.Value)
	rrAuth := httptest.NewRecorder()
	router.ServeHTTP(rrAuth, reqAuth)
	if rrAuth.Code != http.StatusOK {
		t.Fatalf("expected status 200 with token, got %d; body=%s", rrAuth.Code, rrAuth.Body.String())
	}
}

func TestMessagesTokenIPRestriction(t *testing.T) {
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate("user-ip-restrict", 100)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	allowedIP := "203.0.113.7"
	tk.Subnet = &allowedIP
	if err := tokenSvc.Update(tk); err != nil {
		t.Fatalf("update token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TokenService: tokenSvc,
		AdminToken:   "secret-admin",
	})

	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("authorization", "Bearer "+tk.Value)
	req.RemoteAddr = "198.51.100.2:12345"
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if env.Error.Type != "permission_error" {
		t.Fatalf("expected permission_error, got %q", env.Error.Type)
	}
}

func TestMessagesTokenQuotaExceededByReservation(t *testing.T) {
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate("user-quota-restrict", 5)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TokenService: tokenSvc,
		AdminToken:   "secret-admin",
	})

	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("authorization", "Bearer "+tk.Value)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	if env.Error.Type != "quota_error" {
		t.Fatalf("expected quota_error, got %q", env.Error.Type)
	}
}

func TestMessagesApplyChannelRoutePolicyByUserGroup(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("vip-user", "secret", auth.RoleUser)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	user.Group = "vip"
	if err := authSvc.Update(user); err != nil {
		t.Fatalf("update user group: %v", err)
	}

	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate(user.ID, 200)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	channelStore := channel.NewAbilityStore()
	if err := channelStore.AddChannel(&channel.Channel{
		Name:   "vip-adapter",
		Type:   "openai",
		Models: "claude-test",
		Group:  "vip",
		Status: channel.StatusEnabled,
		Weight: 100,
	}); err != nil {
		t.Fatalf("add channel: %v", err)
	}

	svc := &captureService{}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		AuthService:  authSvc,
		TokenService: tokenSvc,
		ChannelStore: channelStore,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":64,
		"messages":[{"role":"user","content":"hello"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("authorization", "Bearer "+tk.Value)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d; body=%s", rr.Code, rr.Body.String())
	}

	routeRaw := svc.capturedReq.Metadata["routing_adapter_route"]
	var route []string
	switch v := routeRaw.(type) {
	case []string:
		route = append(route, v...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				route = append(route, s)
			}
		}
	}
	if len(route) != 1 || route[0] != "vip-adapter" {
		t.Fatalf("expected routing_adapter_route=[vip-adapter], got %#v", routeRaw)
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

type emulationToolLoopService struct {
	calls         int
	sawToolResult bool
	capturedModel []string
}

type plannerEmulationService struct {
	calls         int
	sawToolResult bool
	capturedModel []string
}

type toolLoopServiceWithUpstream struct {
	toolLoopService
	upstreamCfg upstream.UpstreamAdminConfig
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

func (s *emulationToolLoopService) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	s.calls++
	s.capturedModel = append(s.capturedModel, req.Model)
	if s.calls == 1 {
		return orchestrator.Response{
			Model: req.Model,
			Blocks: []orchestrator.AssistantBlock{
				{Type: "text", Text: `{"tool":"get_weather","input":{"city":"Beijing"}}`},
			},
			StopReason: "end_turn",
			Usage: orchestrator.Usage{
				InputTokens:  1,
				OutputTokens: 1,
			},
		}, nil
	}
	s.sawToolResult = containsToolResult(req.Messages, "toolu_emu_1")
	return orchestrator.Response{
		Model: req.Model,
		Blocks: []orchestrator.AssistantBlock{
			{Type: "text", Text: "server tool loop done"},
		},
		StopReason: "end_turn",
		Usage: orchestrator.Usage{
			InputTokens:  1,
			OutputTokens: 1,
		},
	}, nil
}

func (s *emulationToolLoopService) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent)
	errs := make(chan error)
	close(events)
	close(errs)
	return events, errs
}

func (s *plannerEmulationService) Complete(_ context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	s.calls++
	s.capturedModel = append(s.capturedModel, req.Model)
	switch s.calls {
	case 1:
		return orchestrator.Response{
			Model: req.Model,
			Blocks: []orchestrator.AssistantBlock{
				{Type: "text", Text: `{"tool":"get_weather","input":{"city":"Beijing"}}`},
			},
			StopReason: "end_turn",
			Usage: orchestrator.Usage{
				InputTokens:  1,
				OutputTokens: 1,
			},
		}, nil
	case 2:
		s.sawToolResult = containsToolResult(req.Messages, "toolu_emu_1")
		return orchestrator.Response{
			Model: req.Model,
			Blocks: []orchestrator.AssistantBlock{
				{Type: "text", Text: "planner phase done"},
			},
			StopReason: "end_turn",
			Usage: orchestrator.Usage{
				InputTokens:  1,
				OutputTokens: 1,
			},
		}, nil
	default:
		return orchestrator.Response{
			Model: req.Model,
			Blocks: []orchestrator.AssistantBlock{
				{Type: "text", Text: "primary model final"},
			},
			StopReason: "end_turn",
			Usage: orchestrator.Usage{
				InputTokens:  1,
				OutputTokens: 1,
			},
		}, nil
	}
}

func (s *plannerEmulationService) Stream(_ context.Context, _ orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent)
	errs := make(chan error)
	close(events)
	close(errs)
	return events, errs
}

func (s *toolLoopServiceWithUpstream) GetUpstreamConfig() upstream.UpstreamAdminConfig {
	return s.upstreamCfg
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

func TestMessagesRuntimeModelMappingsApplied(t *testing.T) {
	svc := &captureService{}
	st := settings.NewStore(settings.RuntimeSettings{
		ModelMappings: map[string]string{
			"claude-test": "runtime-upstream-model",
		},
	})
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
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
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("x-cc-requested-model"); got != "claude-test" {
		t.Fatalf("unexpected requested model: %q", got)
	}
	if got := rr.Header().Get("x-cc-upstream-model"); got != "runtime-upstream-model" {
		t.Fatalf("unexpected upstream model: %q", got)
	}
	if svc.capturedModel != "runtime-upstream-model" {
		t.Fatalf("expected mapped upstream model, got %q", svc.capturedModel)
	}
}

func TestMessagesRuntimeModelMappingsStrictRejectsUnknown(t *testing.T) {
	st := settings.NewStore(settings.RuntimeSettings{
		ModelMappings:  map[string]string{"known": "m1"},
		ModelMapStrict: true,
	})
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
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
		t.Fatalf("expected 400, got %d; body=%s", rr.Code, rr.Body.String())
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
		ToolAliases: map[string]string{
			"read_file": "file_read",
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
			Mode:          "server_loop",
			MaxSteps:      7,
			EmulationMode: "hybrid",
			PlannerModel:  "planner-tool-model",
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
	if em, ok := svc.capturedReq.Metadata["tool_emulation_mode"].(string); !ok || em != "hybrid" {
		t.Fatalf("expected tool_emulation_mode=hybrid, got %#v", svc.capturedReq.Metadata["tool_emulation_mode"])
	}
	if pm, ok := svc.capturedReq.Metadata["tool_planner_model"].(string); !ok || pm != "planner-tool-model" {
		t.Fatalf("expected tool_planner_model=planner-tool-model, got %#v", svc.capturedReq.Metadata["tool_planner_model"])
	}
	aliases, ok := svc.capturedReq.Metadata["tool_aliases"].(map[string]string)
	if !ok || aliases["read_file"] != "file_read" {
		t.Fatalf("expected tool_aliases metadata propagated, got %#v", svc.capturedReq.Metadata["tool_aliases"])
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

func TestMessagesServerSideToolLoopJSONEmulation(t *testing.T) {
	svc := &emulationToolLoopService{}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolLoop.MaxSteps = 3
	cfg.ToolLoop.EmulationMode = "json"
	events := ccevent.NewStore()
	st := settings.NewStore(cfg)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		EventStore:   events,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"请调用工具查询天气"}],
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if svc.calls != 2 {
		t.Fatalf("expected 2 complete calls, got %d", svc.calls)
	}
	if !svc.sawToolResult {
		t.Fatalf("expected tool_result injected for emulated tool call")
	}
	emulated := events.List(ccevent.ListFilter{EventType: "tool.emulated_call"})
	if len(emulated) == 0 {
		t.Fatalf("expected tool.emulated_call event")
	}
	if mode, _ := emulated[0].Data["emulation_mode"].(string); mode != "json" {
		t.Fatalf("unexpected emulation mode in event: %#v", emulated[0].Data["emulation_mode"])
	}
}

func TestMessagesServerSideToolLoopPlannerModelFinalize(t *testing.T) {
	svc := &plannerEmulationService{}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolLoop.MaxSteps = 4
	cfg.ToolLoop.EmulationMode = "json"
	cfg.ToolLoop.PlannerModel = "planner-model"
	st := settings.NewStore(cfg)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
	})

	body := `{
		"model":"primary-model",
		"max_tokens":128,
		"messages":[{"role":"user","content":"use tools then summarize"}],
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if svc.calls != 3 {
		t.Fatalf("expected 3 complete calls (planner x2 + primary x1), got %d", svc.calls)
	}
	if len(svc.capturedModel) < 3 {
		t.Fatalf("expected captured model sequence, got %+v", svc.capturedModel)
	}
	if svc.capturedModel[0] != "planner-model" || svc.capturedModel[1] != "planner-model" || svc.capturedModel[2] != "primary-model" {
		t.Fatalf("unexpected model sequence: %+v", svc.capturedModel)
	}
	if !svc.sawToolResult {
		t.Fatalf("expected planner call to receive tool_result")
	}

	var resp MessageResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(resp.Content) == 0 || !strings.Contains(resp.Content[0].Text, "primary model final") {
		t.Fatalf("expected final answer from primary model, got %+v", resp.Content)
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

func TestMessagesServerSideToolLoopEmitsGapEventForUnknownTool(t *testing.T) {
	svc := &toolLoopService{toolName: "totally_unknown_tool"}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolLoop.MaxSteps = 3
	st := settings.NewStore(cfg)
	events := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		EventStore:   events,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"please use unknown tool"}],
		"metadata":{"session_id":"sess_gap_1"},
		"tools":[{"name":"declared_tool","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	gaps := events.List(ccevent.ListFilter{EventType: "tool.gap_detected"})
	if len(gaps) == 0 {
		t.Fatalf("expected at least one tool.gap_detected event")
	}
	gap := gaps[0]
	if gap.SessionID != "sess_gap_1" {
		t.Fatalf("unexpected session id: %q", gap.SessionID)
	}
	if gap.RunID == "" {
		t.Fatalf("expected run id on gap event")
	}
	if name, _ := gap.Data["name"].(string); name != "totally_unknown_tool" {
		t.Fatalf("unexpected gap tool name: %#v", gap.Data["name"])
	}
	if reason, _ := gap.Data["reason"].(string); reason != "tool_not_declared" {
		t.Fatalf("unexpected gap reason: %#v", gap.Data["reason"])
	}
}

func TestMessagesToolAliasResolvesUnknownToolName(t *testing.T) {
	svc := &toolLoopService{toolName: "weather_lookup"}
	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolLoop.Mode = "server_loop"
	cfg.ToolAliases = map[string]string{
		"weather_lookup": "get_weather",
	}
	st := settings.NewStore(cfg)
	events := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		EventStore:   events,
	})

	body := `{
		"model":"claude-test",
		"max_tokens":128,
		"messages":[{"role":"user","content":"please lookup weather"}],
		"metadata":{"session_id":"sess_alias_1"},
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if svc.calls != 2 {
		t.Fatalf("expected two turns with server loop, got %d", svc.calls)
	}

	aliasEvents := events.List(ccevent.ListFilter{EventType: "tool.alias_applied"})
	if len(aliasEvents) == 0 {
		t.Fatalf("expected tool.alias_applied event")
	}
	gapEvents := events.List(ccevent.ListFilter{EventType: "tool.gap_detected"})
	for _, ev := range gapEvents {
		if name, _ := ev.Data["name"].(string); name == "weather_lookup" {
			t.Fatalf("did not expect gap event for aliased tool weather_lookup")
		}
	}
}

func TestMessagesAutoEnableToolFallbackWhenUpstreamUnsupported(t *testing.T) {
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
	st := settings.NewStore(cfg)
	events := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		EventStore:   events,
	})

	body := `{
		"model":"custom-no-tools-model",
		"max_tokens":128,
		"messages":[{"role":"user","content":"please use tool"}],
		"metadata":{"session_id":"sess_tool_fallback_1"},
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("anthropic-version", "2023-06-01")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if svc.calls != 2 {
		t.Fatalf("expected server-side tool loop auto enabled (2 calls), got %d", svc.calls)
	}
	fallbackEvents := events.List(ccevent.ListFilter{EventType: "tool.fallback_applied"})
	if len(fallbackEvents) == 0 {
		t.Fatalf("expected tool.fallback_applied event")
	}
}

func TestMessagesStreamAutoEnableToolFallbackWhenUpstreamUnsupported(t *testing.T) {
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
	st := settings.NewStore(cfg)
	events := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		EventStore:   events,
	})

	body := `{
		"model":"custom-no-tools-model",
		"max_tokens":128,
		"stream":true,
		"messages":[{"role":"user","content":"please use tool"}],
		"metadata":{"session_id":"sess_tool_fallback_stream_1"},
		"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"city":{"type":"string"}}}}]
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
	if svc.calls != 2 {
		t.Fatalf("expected server-side tool loop auto enabled for stream (2 calls), got %d", svc.calls)
	}
	payload := rr.Body.String()
	if !strings.Contains(payload, "event: message_start") || !strings.Contains(payload, "event: message_stop") {
		t.Fatalf("expected anthropic stream markers, got payload=%s", payload)
	}
	if !strings.Contains(payload, "server tool loop done") {
		t.Fatalf("expected tool-loop final text in stream payload, got %s", payload)
	}
	fallbackEvents := events.List(ccevent.ListFilter{EventType: "tool.fallback_applied"})
	if len(fallbackEvents) == 0 {
		t.Fatalf("expected tool.fallback_applied event")
	}
}
