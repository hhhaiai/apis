package gateway_test

import (
	"ccgateway/internal/auth"
	"ccgateway/internal/channel"
	. "ccgateway/internal/gateway"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/plugin"
	"ccgateway/internal/policy"
	"ccgateway/internal/probe"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/settings"
	"ccgateway/internal/token"
	"ccgateway/internal/toolcatalog"
	"ccgateway/internal/upstream"
)

func TestAdminSettingsAuthAndUpdate(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	tc := toolcatalog.NewCatalog(nil)
	router := NewRouter(Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		ToolCatalog:  tc,
		AdminToken:   "secret-admin",
	})

	reqNoAuth := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	rrNoAuth := httptest.NewRecorder()
	router.ServeHTTP(rrNoAuth, reqNoAuth)
	if rrNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing admin token, got %d", rrNoAuth.Code)
	}

	putBody := `{
		"use_mode_model_override":true,
		"mode_models":{"plan":"planner-model","chat":"chat-model"},
		"prompt_prefixes":{"plan":"PLAN-FIRST"},
		"allow_experimental_tools":true,
		"allow_unknown_tools":false,
		"routing":{"retries":2,"reflection_passes":3,"timeout_ms":12000,"mode_routes":{"plan":["a","b"]}}
	}`
	reqPut := httptest.NewRequest(http.MethodPut, "/admin/settings", strings.NewReader(putBody))
	reqPut.Header.Set("authorization", "Bearer secret-admin")
	rrPut := httptest.NewRecorder()
	router.ServeHTTP(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin put settings, got %d; body=%s", rrPut.Code, rrPut.Body.String())
	}

	reqGet := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	reqGet.Header.Set("x-admin-token", "secret-admin")
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin get settings, got %d", rrGet.Code)
	}
	var got settings.RuntimeSettings
	if err := json.Unmarshal(rrGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode settings response: %v", err)
	}
	if !got.UseModeModelOverride {
		t.Fatalf("expected mode model override enabled")
	}
	if got.ModeModels["plan"] != "planner-model" {
		t.Fatalf("unexpected plan mode model: %q", got.ModeModels["plan"])
	}
}

func TestAdminSettingsRejectUnknownFields(t *testing.T) {
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(settings.DefaultRuntimeSettings()),
		AdminToken:   "secret-admin",
	})

	req := httptest.NewRequest(http.MethodPut, "/admin/settings", strings.NewReader(`{"unknown_field":1}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field in settings body, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminSettingsRejectTrailingJSON(t *testing.T) {
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(settings.DefaultRuntimeSettings()),
		AdminToken:   "secret-admin",
	})

	req := httptest.NewRequest(http.MethodPut, "/admin/settings", strings.NewReader(`{} {}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON in settings body, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminToolsUpdate(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	tc := toolcatalog.NewCatalog([]toolcatalog.ToolSpec{
		{Name: "get_weather", Status: toolcatalog.StatusSupported},
	})
	router := NewRouter(Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		ToolCatalog:  tc,
		AdminToken:   "secret-admin",
	})

	putBody := `{
		"tools":[
			{"name":"get_weather","status":"supported"},
			{"name":"sql_exec","status":"unsupported"}
		]
	}`
	reqPut := httptest.NewRequest(http.MethodPut, "/admin/tools", strings.NewReader(putBody))
	reqPut.Header.Set("authorization", "Bearer secret-admin")
	rrPut := httptest.NewRecorder()
	router.ServeHTTP(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin put tools, got %d; body=%s", rrPut.Code, rrPut.Body.String())
	}

	reqGet := httptest.NewRequest(http.MethodGet, "/admin/tools", nil)
	reqGet.Header.Set("authorization", "Bearer secret-admin")
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin get tools, got %d", rrGet.Code)
	}
	var payload struct {
		Tools []toolcatalog.ToolSpec `json:"tools"`
	}
	if err := json.Unmarshal(rrGet.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode tools response: %v", err)
	}
	if len(payload.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(payload.Tools))
	}
}

func TestAdminToolsScopedUpdate(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	tc := toolcatalog.NewScopedCatalog([]toolcatalog.ToolSpec{
		{Name: "global_tool", Status: toolcatalog.StatusSupported},
	})
	router := NewRouter(Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		ToolCatalog:  tc,
		AdminToken:   "secret-admin",
	})

	putBody := `{"tools":[{"name":"alpha_only","status":"supported"}]}`
	reqPut := httptest.NewRequest(http.MethodPut, "/admin/tools?scope=project&project_id=alpha", strings.NewReader(putBody))
	reqPut.Header.Set("authorization", "Bearer secret-admin")
	rrPut := httptest.NewRecorder()
	router.ServeHTTP(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for scoped tool put, got %d; body=%s", rrPut.Code, rrPut.Body.String())
	}

	reqGetAlpha := httptest.NewRequest(http.MethodGet, "/admin/tools?scope=project&project_id=alpha", nil)
	reqGetAlpha.Header.Set("authorization", "Bearer secret-admin")
	rrGetAlpha := httptest.NewRecorder()
	router.ServeHTTP(rrGetAlpha, reqGetAlpha)
	if rrGetAlpha.Code != http.StatusOK {
		t.Fatalf("expected 200 for alpha tools get, got %d; body=%s", rrGetAlpha.Code, rrGetAlpha.Body.String())
	}
	var alphaPayload struct {
		Tools []toolcatalog.ToolSpec `json:"tools"`
	}
	if err := json.Unmarshal(rrGetAlpha.Body.Bytes(), &alphaPayload); err != nil {
		t.Fatalf("decode alpha tool payload: %v", err)
	}
	if len(alphaPayload.Tools) != 1 || alphaPayload.Tools[0].Name != "alpha_only" {
		t.Fatalf("unexpected alpha tools: %+v", alphaPayload.Tools)
	}

	reqGetGlobal := httptest.NewRequest(http.MethodGet, "/admin/tools?scope=global", nil)
	reqGetGlobal.Header.Set("authorization", "Bearer secret-admin")
	rrGetGlobal := httptest.NewRecorder()
	router.ServeHTTP(rrGetGlobal, reqGetGlobal)
	if rrGetGlobal.Code != http.StatusOK {
		t.Fatalf("expected 200 for global tools get, got %d; body=%s", rrGetGlobal.Code, rrGetGlobal.Body.String())
	}
	var globalPayload struct {
		Tools []toolcatalog.ToolSpec `json:"tools"`
	}
	if err := json.Unmarshal(rrGetGlobal.Body.Bytes(), &globalPayload); err != nil {
		t.Fatalf("decode global tool payload: %v", err)
	}
	if len(globalPayload.Tools) != 1 || globalPayload.Tools[0].Name != "global_tool" {
		t.Fatalf("unexpected global tools: %+v", globalPayload.Tools)
	}
}

func TestAdminChannelsCRUD(t *testing.T) {
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		ChannelStore: channel.NewAbilityStore(),
		AdminToken:   "secret-admin",
	})

	createBody := `{
		"name":"primary-openai",
		"type":"openai",
		"models":"gpt-4o",
		"group":"default",
		"status":1
	}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/admin/channels", strings.NewReader(createBody))
	reqCreate.Header.Set("authorization", "Bearer secret-admin")
	rrCreate := httptest.NewRecorder()
	router.ServeHTTP(rrCreate, reqCreate)
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 for channel create, got %d; body=%s", rrCreate.Code, rrCreate.Body.String())
	}

	var created channel.Channel
	if err := json.Unmarshal(rrCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create channel response: %v", err)
	}
	if created.ID == 0 {
		t.Fatalf("expected non-zero channel id, got %+v", created)
	}

	reqList := httptest.NewRequest(http.MethodGet, "/admin/channels", nil)
	reqList.Header.Set("authorization", "Bearer secret-admin")
	rrList := httptest.NewRecorder()
	router.ServeHTTP(rrList, reqList)
	if rrList.Code != http.StatusOK {
		t.Fatalf("expected 200 for channel list, got %d; body=%s", rrList.Code, rrList.Body.String())
	}
	var payload struct {
		Data []channel.Channel `json:"data"`
	}
	if err := json.Unmarshal(rrList.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode channel list response: %v", err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("expected 1 channel, got %d; payload=%s", len(payload.Data), rrList.Body.String())
	}

	statusBody := `{"status":2}`
	reqStatus := httptest.NewRequest(http.MethodPut, "/admin/channels/"+strconv.FormatInt(created.ID, 10)+"/status", strings.NewReader(statusBody))
	reqStatus.Header.Set("authorization", "Bearer secret-admin")
	rrStatus := httptest.NewRecorder()
	router.ServeHTTP(rrStatus, reqStatus)
	if rrStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 for channel status update, got %d; body=%s", rrStatus.Code, rrStatus.Body.String())
	}

	reqGet := httptest.NewRequest(http.MethodGet, "/admin/channels/"+strconv.FormatInt(created.ID, 10), nil)
	reqGet.Header.Set("authorization", "Bearer secret-admin")
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for channel get, got %d; body=%s", rrGet.Code, rrGet.Body.String())
	}
	var updated channel.Channel
	if err := json.Unmarshal(rrGet.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode channel get response: %v", err)
	}
	if updated.Status != 2 {
		t.Fatalf("expected status=2 after subpath update, got %d", updated.Status)
	}

	reqTest := httptest.NewRequest(http.MethodPost, "/admin/channels/"+strconv.FormatInt(created.ID, 10)+"/test", strings.NewReader(`{}`))
	reqTest.Header.Set("authorization", "Bearer secret-admin")
	rrTest := httptest.NewRecorder()
	router.ServeHTTP(rrTest, reqTest)
	if rrTest.Code != http.StatusOK {
		t.Fatalf("expected 200 for channel test endpoint, got %d; body=%s", rrTest.Code, rrTest.Body.String())
	}
}

func TestAdminBootstrapApply(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	tc := toolcatalog.NewScopedCatalog(nil)
	pluginStore := plugin.NewManager()
	mcpStore := mcpregistry.NewStore(nil)
	router := NewRouter(Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		ToolCatalog:  tc,
		PluginStore:  pluginStore,
		MCPRegistry:  mcpStore,
		AdminToken:   "secret-admin",
	})

	body := `{
		"scope":"project",
		"project_id":"alpha",
		"tools":[{"name":"alpha_tool","status":"supported"}],
		"plugins":[{"name":"alpha_plugin","version":"1.0.0"}],
		"mcp_servers":[{"id":"alpha_mcp","name":"alpha-mcp","transport":"http","url":"http://127.0.0.1:18080"}]
	}`
	req := httptest.NewRequest(http.MethodPost, "/admin/bootstrap/apply", strings.NewReader(body))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for bootstrap apply, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode bootstrap response: %v", err)
	}
	if ok, _ := resp["ok"].(bool); !ok {
		t.Fatalf("expected bootstrap ok=true, got %#v", resp)
	}

	reqPluginList := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins?scope=project&project_id=alpha", nil)
	reqPluginList.Header.Set("authorization", "Bearer secret-admin")
	rrPluginList := httptest.NewRecorder()
	router.ServeHTTP(rrPluginList, reqPluginList)
	if rrPluginList.Code != http.StatusOK {
		t.Fatalf("expected 200 for scoped plugin list, got %d; body=%s", rrPluginList.Code, rrPluginList.Body.String())
	}

	reqTools := httptest.NewRequest(http.MethodGet, "/admin/tools?scope=project&project_id=alpha", nil)
	reqTools.Header.Set("authorization", "Bearer secret-admin")
	rrTools := httptest.NewRecorder()
	router.ServeHTTP(rrTools, reqTools)
	if rrTools.Code != http.StatusOK {
		t.Fatalf("expected 200 for scoped tools get, got %d; body=%s", rrTools.Code, rrTools.Body.String())
	}
}

func TestAdminToolGapsSummary(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	tc := toolcatalog.NewCatalog(nil)
	eventStore := ccevent.NewStore()
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: "sess_1",
		RunID:     "run_1",
		Data: map[string]any{
			"name":   "unknown_tool",
			"reason": "tool_not_declared",
			"path":   "/v1/messages",
		},
	})
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: "sess_1",
		RunID:     "run_2",
		Data: map[string]any{
			"name":   "web_search",
			"reason": "tool_not_implemented",
			"path":   "/v1/chat/completions",
		},
	})
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: "sess_2",
		RunID:     "run_3",
		Data: map[string]any{
			"name":   "web_search",
			"reason": "tool_not_implemented",
			"path":   "/v1/chat/completions",
		},
	})

	router := NewRouter(Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		ToolCatalog:  tc,
		EventStore:   eventStore,
		AdminToken:   "secret-admin",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/tools/gaps?reason=tool_not_implemented", nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin tool gap summary, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		EventType string         `json:"event_type"`
		Scanned   int            `json:"scanned"`
		Matched   int            `json:"matched"`
		ByTool    map[string]int `json:"by_tool"`
		ByReason  map[string]int `json:"by_reason"`
		Summaries []struct {
			Name   string `json:"name"`
			Reason string `json:"reason"`
			Count  int    `json:"count"`
		} `json:"gap_summaries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode tool gaps response: %v", err)
	}
	if resp.EventType != "tool.gap_detected" {
		t.Fatalf("unexpected event type: %q", resp.EventType)
	}
	if resp.Scanned != 3 {
		t.Fatalf("expected scanned=3, got %d", resp.Scanned)
	}
	if resp.Matched != 2 {
		t.Fatalf("expected matched=2, got %d", resp.Matched)
	}
	if resp.ByTool["web_search"] != 2 {
		t.Fatalf("expected web_search count=2, got %+v", resp.ByTool)
	}
	if resp.ByReason["tool_not_implemented"] != 2 {
		t.Fatalf("expected reason count=2, got %+v", resp.ByReason)
	}
	if len(resp.Summaries) != 1 || resp.Summaries[0].Name != "web_search" || resp.Summaries[0].Count != 2 {
		t.Fatalf("unexpected summaries: %+v", resp.Summaries)
	}
}

func TestAdminToolGapsWithSuggestions(t *testing.T) {
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
						{"name": "image_recognition"},
					},
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
		ID:        "mcp_suggest_1",
		Name:      "mcp-suggest",
		Transport: mcpregistry.TransportHTTP,
		URL:       mcpRPC.URL,
		TimeoutMS: 2000,
		Retries:   1,
	})
	if err != nil {
		t.Fatalf("register mcp server: %v", err)
	}

	eventStore := ccevent.NewStore()
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: "sess_suggest_1",
		RunID:     "run_suggest_1",
		Data: map[string]any{
			"name":   "remote_search",
			"reason": "tool_not_implemented",
			"path":   "/v1/messages",
		},
	})
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: "sess_suggest_1",
		RunID:     "run_suggest_2",
		Data: map[string]any{
			"name":   "read_file",
			"reason": "tool_not_declared",
			"path":   "/v1/messages",
		},
	})
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType: "tool.gap_detected",
		SessionID: "sess_suggest_1",
		RunID:     "run_suggest_3",
		Data: map[string]any{
			"name":   "totally_unknown",
			"reason": "tool_not_declared",
			"path":   "/v1/messages",
		},
	})

	cfg := settings.DefaultRuntimeSettings()
	cfg.ToolAliases = map[string]string{"read_file": "file_read"}
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(cfg),
		EventStore:   eventStore,
		MCPRegistry:  registry,
		AdminToken:   "secret-admin",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/tools/gaps?include_suggestions=true", nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin tool gap suggestions, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode tool gap suggestions: %v", err)
	}

	replacements, ok := payload["replacement_candidates"].(map[string]any)
	if !ok {
		t.Fatalf("expected replacement_candidates map, got %#v", payload["replacement_candidates"])
	}
	remote, ok := replacements["remote_search"].([]any)
	if !ok || len(remote) == 0 {
		t.Fatalf("expected remote_search replacement suggestions, got %#v", replacements["remote_search"])
	}
	readFile, ok := replacements["read_file"].([]any)
	if !ok || len(readFile) == 0 {
		t.Fatalf("expected read_file replacement suggestions, got %#v", replacements["read_file"])
	}

	unresolved, ok := payload["unresolved_tools"].([]any)
	if !ok {
		t.Fatalf("expected unresolved_tools array, got %#v", payload["unresolved_tools"])
	}
	foundUnknown := false
	for _, item := range unresolved {
		if name, _ := item.(string); name == "totally_unknown" {
			foundUnknown = true
			break
		}
	}
	if !foundUnknown {
		t.Fatalf("expected totally_unknown in unresolved_tools, got %#v", unresolved)
	}
}

func TestAdminMarketplaceCloudListRejectsPrivateHost(t *testing.T) {
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		AdminToken:   "secret-admin",
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/marketplace/cloud/list", strings.NewReader(`{"url":"http://127.0.0.1:8080/manifests.json"}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for private host cloud list, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode cloud list error: %v", err)
	}
	if !strings.Contains(env.Error.Message, "private network hosts are not allowed") {
		t.Fatalf("expected private-host rejection message, got %q", env.Error.Message)
	}
}

func TestAdminSchedulerAndProbeStatus(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	tc := toolcatalog.NewCatalog(nil)
	sched := scheduler.NewEngine(scheduler.Config{
		FailureThreshold: 2,
		Cooldown:         5 * time.Second,
	}, []string{"a1"})
	probeRunner := probe.NewRunner(probe.Config{
		Enabled:       true,
		Interval:      30 * time.Second,
		Timeout:       5 * time.Second,
		DefaultModels: []string{"m1"},
	}, []upstream.Adapter{upstream.NewMockAdapter("a1", false)}, sched)
	router := NewRouter(Dependencies{
		Orchestrator:    svc,
		Policy:          policy.NewNoopEngine(),
		ModelMapper:     modelmap.NewIdentityMapper(),
		Settings:        st,
		ToolCatalog:     tc,
		SchedulerStatus: sched,
		ProbeStatus:     probeRunner,
		AdminToken:      "secret-admin",
	})

	reqScheduler := httptest.NewRequest(http.MethodGet, "/admin/scheduler", nil)
	reqScheduler.Header.Set("authorization", "Bearer secret-admin")
	rrScheduler := httptest.NewRecorder()
	router.ServeHTTP(rrScheduler, reqScheduler)
	if rrScheduler.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin scheduler, got %d; body=%s", rrScheduler.Code, rrScheduler.Body.String())
	}

	reqProbe := httptest.NewRequest(http.MethodGet, "/admin/probe", nil)
	reqProbe.Header.Set("authorization", "Bearer secret-admin")
	rrProbe := httptest.NewRecorder()
	router.ServeHTTP(rrProbe, reqProbe)
	if rrProbe.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin probe, got %d; body=%s", rrProbe.Code, rrProbe.Body.String())
	}

	putSchedulerBody := `{"failure_threshold":4,"cooldown_ms":12000,"strict_probe_gate":true}`
	reqPutScheduler := httptest.NewRequest(http.MethodPut, "/admin/scheduler", strings.NewReader(putSchedulerBody))
	reqPutScheduler.Header.Set("authorization", "Bearer secret-admin")
	rrPutScheduler := httptest.NewRecorder()
	router.ServeHTTP(rrPutScheduler, reqPutScheduler)
	if rrPutScheduler.Code != http.StatusOK {
		t.Fatalf("expected 200 for put admin scheduler, got %d; body=%s", rrPutScheduler.Code, rrPutScheduler.Body.String())
	}

	putProbeBody := `{"enabled":false,"interval_ms":45000,"timeout_ms":7000,"default_models":["x1","x2"],"stream_smoke":true}`
	reqPutProbe := httptest.NewRequest(http.MethodPut, "/admin/probe", strings.NewReader(putProbeBody))
	reqPutProbe.Header.Set("authorization", "Bearer secret-admin")
	rrPutProbe := httptest.NewRecorder()
	router.ServeHTTP(rrPutProbe, reqPutProbe)
	if rrPutProbe.Code != http.StatusOK {
		t.Fatalf("expected 200 for put admin probe, got %d; body=%s", rrPutProbe.Code, rrPutProbe.Body.String())
	}
}

func TestAdminModelMappingUpdate(t *testing.T) {
	svc := orchestrator.NewSimpleService()
	st := settings.NewStore(settings.DefaultRuntimeSettings())
	router := NewRouter(Dependencies{
		Orchestrator: svc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     st,
		AdminToken:   "secret-admin",
	})

	putBody := `{
		"model_mappings":{"claude-test":"upstream-claude"},
		"model_map_strict":true,
		"model_map_fallback":"fallback-model"
	}`
	reqPut := httptest.NewRequest(http.MethodPut, "/admin/model-mapping", strings.NewReader(putBody))
	reqPut.Header.Set("authorization", "Bearer secret-admin")
	rrPut := httptest.NewRecorder()
	router.ServeHTTP(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for put admin model mapping, got %d; body=%s", rrPut.Code, rrPut.Body.String())
	}

	reqGet := httptest.NewRequest(http.MethodGet, "/admin/model-mapping", nil)
	reqGet.Header.Set("x-admin-token", "secret-admin")
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get admin model mapping, got %d; body=%s", rrGet.Code, rrGet.Body.String())
	}

	cfg := st.Get()
	if cfg.ModelMappings["claude-test"] != "upstream-claude" {
		t.Fatalf("unexpected mapping: %#v", cfg.ModelMappings)
	}
	if !cfg.ModelMapStrict {
		t.Fatalf("expected strict mode enabled")
	}
	if cfg.ModelMapFallback != "fallback-model" {
		t.Fatalf("unexpected fallback model: %q", cfg.ModelMapFallback)
	}
}

func TestAdminUpstreamUpdate(t *testing.T) {
	routerSvc := upstream.NewRouterService(upstream.RouterConfig{
		DefaultRoute: []string{"mock-a"},
	}, []upstream.Adapter{
		upstream.NewMockAdapter("mock-a", false),
	})
	router := NewRouter(Dependencies{
		Orchestrator: routerSvc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		AdminToken:   "secret-admin",
	})

	reqGet := httptest.NewRequest(http.MethodGet, "/admin/upstream", nil)
	reqGet.Header.Set("authorization", "Bearer secret-admin")
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for get admin upstream, got %d; body=%s", rrGet.Code, rrGet.Body.String())
	}

	putBody := `{
		"adapters":[
			{
				"name":"script-a1",
				"kind":"script",
				"command":"bash",
				"args":["-lc","cat >/dev/null; echo '{\"text\":\"ok\"}'"],
				"model":"custom-script-model"
			}
		],
		"default_route":["script-a1"],
		"model_routes":{"*":["script-a1"]}
	}`
	reqPut := httptest.NewRequest(http.MethodPut, "/admin/upstream", strings.NewReader(putBody))
	reqPut.Header.Set("authorization", "Bearer secret-admin")
	rrPut := httptest.NewRecorder()
	router.ServeHTTP(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for put admin upstream, got %d; body=%s", rrPut.Code, rrPut.Body.String())
	}

	cfg := routerSvc.GetUpstreamConfig()
	if len(cfg.Adapters) != 1 || cfg.Adapters[0].Name != "script-a1" {
		t.Fatalf("unexpected adapters after update: %+v", cfg.Adapters)
	}
	if len(cfg.DefaultRoute) != 1 || cfg.DefaultRoute[0] != "script-a1" {
		t.Fatalf("unexpected default route after update: %+v", cfg.DefaultRoute)
	}
}

func TestAdminCapabilitiesMatrixByModelAndModeRoute(t *testing.T) {
	supportsToolsFalse := false
	supportsVisionFalse := false
	supportsToolsTrue := true
	supportsVisionTrue := true

	cheapAdapter, err := upstream.NewHTTPAdapter(upstream.HTTPAdapterConfig{
		Name:           "cheap-text",
		Kind:           upstream.AdapterKindOpenAI,
		BaseURL:        "https://example.com",
		Model:          "gpt-3.5-turbo",
		SupportsTools:  &supportsToolsFalse,
		SupportsVision: &supportsVisionFalse,
	}, nil)
	if err != nil {
		t.Fatalf("new cheap adapter: %v", err)
	}
	visionAdapter, err := upstream.NewHTTPAdapter(upstream.HTTPAdapterConfig{
		Name:           "vision-pro",
		Kind:           upstream.AdapterKindOpenAI,
		BaseURL:        "https://example.com",
		Model:          "gpt-4o",
		SupportsTools:  &supportsToolsTrue,
		SupportsVision: &supportsVisionTrue,
	}, nil)
	if err != nil {
		t.Fatalf("new vision adapter: %v", err)
	}

	routerSvc := upstream.NewRouterService(upstream.RouterConfig{
		DefaultRoute: []string{"cheap-text"},
		Routes: map[string][]string{
			"vision-*": []string{"vision-pro"},
		},
	}, []upstream.Adapter{cheapAdapter, visionAdapter})
	cfg := settings.DefaultRuntimeSettings()
	cfg.Routing.ModeRoutes = map[string][]string{
		"plan": []string{"cheap-text"},
	}
	router := NewRouter(Dependencies{
		Orchestrator: routerSvc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(cfg),
		AdminToken:   "secret-admin",
	})

	reqModelRoute := httptest.NewRequest(http.MethodGet, "/admin/capabilities?model=vision-alpha", nil)
	reqModelRoute.Header.Set("authorization", "Bearer secret-admin")
	rrModelRoute := httptest.NewRecorder()
	router.ServeHTTP(rrModelRoute, reqModelRoute)
	if rrModelRoute.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin capabilities model route, got %d; body=%s", rrModelRoute.Code, rrModelRoute.Body.String())
	}

	var modelPayload struct {
		Routes struct {
			Source   string   `json:"source"`
			Resolved []string `json:"resolved"`
		} `json:"routes"`
		Effective struct {
			SupportsTools       bool `json:"supports_tools"`
			SupportsToolsKnown  bool `json:"supports_tools_known"`
			SupportsVision      bool `json:"supports_vision"`
			SupportsVisionKnown bool `json:"supports_vision_known"`
		} `json:"effective"`
		Adapters []struct {
			Name            string `json:"name"`
			OnResolvedRoute bool   `json:"on_resolved_route"`
		} `json:"adapters"`
	}
	if err := json.Unmarshal(rrModelRoute.Body.Bytes(), &modelPayload); err != nil {
		t.Fatalf("decode capabilities model payload: %v", err)
	}
	if !strings.Contains(modelPayload.Routes.Source, "upstream.model_routes.pattern:vision-*") {
		t.Fatalf("expected model route source, got %q", modelPayload.Routes.Source)
	}
	if len(modelPayload.Routes.Resolved) != 1 || modelPayload.Routes.Resolved[0] != "vision-pro" {
		t.Fatalf("unexpected resolved route: %+v", modelPayload.Routes.Resolved)
	}
	if !modelPayload.Effective.SupportsTools || !modelPayload.Effective.SupportsToolsKnown {
		t.Fatalf("expected supports_tools=true known=true, got %+v", modelPayload.Effective)
	}
	if !modelPayload.Effective.SupportsVision || !modelPayload.Effective.SupportsVisionKnown {
		t.Fatalf("expected supports_vision=true known=true, got %+v", modelPayload.Effective)
	}
	onRoute := false
	for _, row := range modelPayload.Adapters {
		if row.Name == "vision-pro" && row.OnResolvedRoute {
			onRoute = true
			break
		}
	}
	if !onRoute {
		t.Fatalf("expected vision-pro marked on_resolved_route")
	}

	reqModeRoute := httptest.NewRequest(http.MethodGet, "/admin/capabilities?mode=plan&model=vision-alpha", nil)
	reqModeRoute.Header.Set("authorization", "Bearer secret-admin")
	rrModeRoute := httptest.NewRecorder()
	router.ServeHTTP(rrModeRoute, reqModeRoute)
	if rrModeRoute.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin capabilities mode route, got %d; body=%s", rrModeRoute.Code, rrModeRoute.Body.String())
	}

	var modePayload struct {
		Routes struct {
			Source   string   `json:"source"`
			Resolved []string `json:"resolved"`
		} `json:"routes"`
		Effective struct {
			SupportsTools        bool `json:"supports_tools"`
			SupportsToolsKnown   bool `json:"supports_tools_known"`
			SupportsVision       bool `json:"supports_vision"`
			SupportsVisionKnown  bool `json:"supports_vision_known"`
			ToolFallbackNeeded   bool `json:"tool_fallback_needed"`
			VisionFallbackNeeded bool `json:"vision_fallback_needed"`
		} `json:"effective"`
	}
	if err := json.Unmarshal(rrModeRoute.Body.Bytes(), &modePayload); err != nil {
		t.Fatalf("decode capabilities mode payload: %v", err)
	}
	if modePayload.Routes.Source != "runtime.mode_routes:plan" {
		t.Fatalf("expected runtime mode route source, got %q", modePayload.Routes.Source)
	}
	if len(modePayload.Routes.Resolved) != 1 || modePayload.Routes.Resolved[0] != "cheap-text" {
		t.Fatalf("unexpected resolved route for mode plan: %+v", modePayload.Routes.Resolved)
	}
	if modePayload.Effective.SupportsTools || !modePayload.Effective.SupportsToolsKnown {
		t.Fatalf("expected supports_tools=false known=true, got %+v", modePayload.Effective)
	}
	if !modePayload.Effective.ToolFallbackNeeded {
		t.Fatalf("expected tool_fallback_needed=true, got %+v", modePayload.Effective)
	}
	if modePayload.Effective.SupportsVision || !modePayload.Effective.SupportsVisionKnown {
		t.Fatalf("expected supports_vision=false known=true, got %+v", modePayload.Effective)
	}
	if !modePayload.Effective.VisionFallbackNeeded {
		t.Fatalf("expected vision_fallback_needed=true, got %+v", modePayload.Effective)
	}
}

func TestAdminStatusIncludesCapabilitiesOverview(t *testing.T) {
	supportsTools := true
	adapter, err := upstream.NewHTTPAdapter(upstream.HTTPAdapterConfig{
		Name:           "a1",
		Kind:           upstream.AdapterKindOpenAI,
		BaseURL:        "https://example.com",
		SupportsTools:  &supportsTools,
		SupportsVision: nil,
	}, nil)
	if err != nil {
		t.Fatalf("new adapter: %v", err)
	}
	routerSvc := upstream.NewRouterService(upstream.RouterConfig{
		DefaultRoute: []string{"a1"},
	}, []upstream.Adapter{adapter})
	router := NewRouter(Dependencies{
		Orchestrator: routerSvc,
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		Settings:     settings.NewStore(settings.DefaultRuntimeSettings()),
		AdminToken:   "secret-admin",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin status, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode admin status: %v", err)
	}
	if _, ok := payload["capabilities_overview"].(map[string]any); !ok {
		t.Fatalf("expected capabilities_overview in status payload, got %#v", payload["capabilities_overview"])
	}
}

func TestAdminDashboardFallbackLegacyHTML(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "CC Gateway") {
		t.Fatalf("expected dashboard html body, got %q", body)
	}
	if !strings.Contains(body, "Admin Access") {
		t.Fatalf("expected dashboard auth bar, got %q", body)
	}
	if !strings.Contains(body, "x-admin-token") {
		t.Fatalf("expected x-admin-token header usage in dashboard, got %q", body)
	}
	if !strings.Contains(body, "Authorization") {
		t.Fatalf("expected authorization header usage in dashboard, got %q", body)
	}
	if !strings.Contains(body, "q.set('token',adminToken)") {
		t.Fatalf("expected event stream token passthrough in dashboard, got %q", body)
	}
}

func TestAdminDashboardServeBuiltDist(t *testing.T) {
	distDir := t.TempDir()
	indexHTML := "<!doctype html><html><body>vue-admin-dist</body></html>"
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	assetsDir := filepath.Join(distDir, "assets")
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		t.Fatalf("mkdir assets: %v", err)
	}
	if err := os.WriteFile(filepath.Join(assetsDir, "app.js"), []byte("console.log('dist');"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}

	t.Setenv("ADMIN_UI_DIST_DIR", distDir)
	router := newTestRouter(t)

	reqIndex := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	rrIndex := httptest.NewRecorder()
	router.ServeHTTP(rrIndex, reqIndex)
	if rrIndex.Code != http.StatusOK {
		t.Fatalf("expected 200 for dist index, got %d", rrIndex.Code)
	}
	if !strings.Contains(rrIndex.Body.String(), "vue-admin-dist") {
		t.Fatalf("expected dist index body, got %q", rrIndex.Body.String())
	}

	reqAsset := httptest.NewRequest(http.MethodGet, "/admin/assets/app.js", nil)
	rrAsset := httptest.NewRecorder()
	router.ServeHTTP(rrAsset, reqAsset)
	if rrAsset.Code != http.StatusOK {
		t.Fatalf("expected 200 for dist asset, got %d", rrAsset.Code)
	}
	if !strings.Contains(rrAsset.Body.String(), "console.log('dist');") {
		t.Fatalf("expected dist asset body, got %q", rrAsset.Body.String())
	}

	reqSPA := httptest.NewRequest(http.MethodGet, "/admin/workbench/plans", nil)
	rrSPA := httptest.NewRecorder()
	router.ServeHTTP(rrSPA, reqSPA)
	if rrSPA.Code != http.StatusOK {
		t.Fatalf("expected 200 for spa fallback, got %d", rrSPA.Code)
	}
	if !strings.Contains(rrSPA.Body.String(), "vue-admin-dist") {
		t.Fatalf("expected spa fallback index body, got %q", rrSPA.Body.String())
	}
}

func TestRootRedirectsToAdminDashboard(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/admin/" {
		t.Fatalf("expected redirect to /admin/, got %q", got)
	}
}

func TestAdminPathRedirectsToAdminDashboard(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusMovedPermanently && rr.Code != http.StatusFound {
		t.Fatalf("expected 301/302, got %d", rr.Code)
	}
	if got := rr.Header().Get("Location"); got != "/admin/" {
		t.Fatalf("expected redirect to /admin/, got %q", got)
	}
}

func TestLegacyHomePageOnHomePath(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/home", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "CC Gateway") {
		t.Fatalf("expected home page title, got %q", body)
	}
	if !strings.Contains(body, "/admin/") {
		t.Fatalf("expected admin entry link, got %q", body)
	}
	if !strings.Contains(body, DefaultAdminToken) {
		t.Fatalf("expected default admin token hint, got %q", body)
	}
}

func TestAdminAuthStatus(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken: "secret-admin",
	})
	req := httptest.NewRequest(http.MethodGet, "/admin/auth/status", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode auth status: %v", err)
	}
	if got, _ := payload["auth_required"].(bool); !got {
		t.Fatalf("expected auth_required=true, got %#v", payload["auth_required"])
	}
	if got, _ := payload["default_token_enabled"].(bool); got {
		t.Fatalf("expected default_token_enabled=false, got %#v", payload["default_token_enabled"])
	}
	if got, _ := payload["token_provided"].(bool); got {
		t.Fatalf("expected token_provided=false, got %#v", payload["token_provided"])
	}
	if got, _ := payload["token_valid"].(bool); got {
		t.Fatalf("expected token_valid=false without token, got %#v", payload["token_valid"])
	}
}

func TestAdminAuthStatusWithDefaultTokenWarning(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken: DefaultAdminToken,
	})
	req := httptest.NewRequest(http.MethodGet, "/admin/auth/status", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode auth status: %v", err)
	}
	if got, _ := payload["default_token_enabled"].(bool); !got {
		t.Fatalf("expected default_token_enabled=true, got %#v", payload["default_token_enabled"])
	}
	if got, _ := payload["default_token_warning"].(string); strings.TrimSpace(got) == "" {
		t.Fatalf("expected warning text, got %#v", payload["default_token_warning"])
	}
}

func TestAdminAuthStatusWithProvidedToken(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken: "secret-admin",
	})
	req := httptest.NewRequest(http.MethodGet, "/admin/auth/status", nil)
	req.Header.Set("x-admin-token", "secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode auth status: %v", err)
	}
	if got, _ := payload["token_provided"].(bool); !got {
		t.Fatalf("expected token_provided=true, got %#v", payload["token_provided"])
	}
	if got, _ := payload["token_valid"].(bool); !got {
		t.Fatalf("expected token_valid=true, got %#v", payload["token_valid"])
	}
}

func TestAdminUsersRequireAdminAuth(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: auth.NewInMemoryService(),
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/users", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d; body=%s", rr.Code, rr.Body.String())
	}

	reqAuth := httptest.NewRequest(http.MethodGet, "/admin/auth/users", nil)
	reqAuth.Header.Set("authorization", "Bearer secret-admin")
	rrAuth := httptest.NewRecorder()
	router.ServeHTTP(rrAuth, reqAuth)
	if rrAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 with admin token, got %d; body=%s", rrAuth.Code, rrAuth.Body.String())
	}
}

func TestAdminUserTokenByPathRequiresAdminAuth(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("alice", "secret", "user")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate(user.ID, 100)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:   "secret-admin",
		AuthService:  authSvc,
		TokenService: tokenSvc,
	})
	path := "/admin/auth/users/" + user.ID + "/tokens/" + strconv.FormatInt(tk.ID, 10)

	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without admin token, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminUserTokenByPathCRUD(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("bob", "secret", "user")
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate(user.ID, 100)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:   "secret-admin",
		AuthService:  authSvc,
		TokenService: tokenSvc,
	})
	tokenPath := "/admin/auth/users/" + user.ID + "/tokens/" + strconv.FormatInt(tk.ID, 10)

	reqGet := httptest.NewRequest(http.MethodGet, tokenPath, nil)
	reqGet.Header.Set("authorization", "Bearer secret-admin")
	rrGet := httptest.NewRecorder()
	router.ServeHTTP(rrGet, reqGet)
	if rrGet.Code != http.StatusOK {
		t.Fatalf("expected 200 for token get, got %d; body=%s", rrGet.Code, rrGet.Body.String())
	}
	var got token.Token
	if err := json.Unmarshal(rrGet.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if got.ID != tk.ID {
		t.Fatalf("unexpected token id: got=%d want=%d", got.ID, tk.ID)
	}

	reqPut := httptest.NewRequest(http.MethodPut, tokenPath, strings.NewReader(`{"name":"rotated"}`))
	reqPut.Header.Set("authorization", "Bearer secret-admin")
	rrPut := httptest.NewRecorder()
	router.ServeHTTP(rrPut, reqPut)
	if rrPut.Code != http.StatusOK {
		t.Fatalf("expected 200 for token put, got %d; body=%s", rrPut.Code, rrPut.Body.String())
	}
	var updated token.Token
	if err := json.Unmarshal(rrPut.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode token put response: %v", err)
	}
	if updated.Name != "rotated" {
		t.Fatalf("unexpected token name after update: %q", updated.Name)
	}

	reqDelete := httptest.NewRequest(http.MethodDelete, tokenPath, nil)
	reqDelete.Header.Set("authorization", "Bearer secret-admin")
	rrDelete := httptest.NewRecorder()
	router.ServeHTTP(rrDelete, reqDelete)
	if rrDelete.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for token delete, got %d; body=%s", rrDelete.Code, rrDelete.Body.String())
	}
}

func TestAdminUserTokensCreateAllowsEmptyBody(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("empty-body-token-user", "secret", auth.RoleUser)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	tokenSvc := token.NewInMemoryService()
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:   "secret-admin",
		AuthService:  authSvc,
		TokenService: tokenSvc,
	})

	path := "/admin/auth/users/" + user.ID + "/tokens"
	req := httptest.NewRequest(http.MethodPost, path, http.NoBody)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 for empty-body token create, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var created token.Token
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created token: %v", err)
	}
	if strings.TrimSpace(created.Value) == "" {
		t.Fatalf("expected created token value, got %+v", created)
	}
}

func TestAdminUsersCreateReturnsErrorWhenGroupUpdateFails(t *testing.T) {
	base := auth.NewInMemoryService()
	authSvc := &failingAuthService{
		Service:   base,
		updateErr: errors.New("forced update error"),
	}
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/auth/users", strings.NewReader(`{
		"username":"create-group-fail",
		"password":"secret",
		"role":"user",
		"group":"vip"
	}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for group update failure, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode create user error: %v", err)
	}
	if !strings.Contains(env.Error.Message, "forced update error") {
		t.Fatalf("expected forced update error, got %q", env.Error.Message)
	}
	created, err := base.Get("create-group-fail")
	if err != nil {
		t.Fatalf("expected created user record for rollback check: %v", err)
	}
	if created.Status != auth.StatusDeleted {
		t.Fatalf("expected rolled back user status=%d, got %d", auth.StatusDeleted, created.Status)
	}
}

func TestAdminUsersCreateReturnsErrorWhenQuotaAddFails(t *testing.T) {
	base := auth.NewInMemoryService()
	authSvc := &failingAuthService{
		Service:     base,
		addQuotaErr: errors.New("forced add quota error"),
	}
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/auth/users", strings.NewReader(`{
		"username":"create-quota-fail",
		"password":"secret",
		"role":"user",
		"quota":100
	}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for quota add failure, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode create user quota error: %v", err)
	}
	if !strings.Contains(env.Error.Message, "forced add quota error") {
		t.Fatalf("expected forced add quota error, got %q", env.Error.Message)
	}
	created, err := base.Get("create-quota-fail")
	if err != nil {
		t.Fatalf("expected created user record for rollback check: %v", err)
	}
	if created.Status != auth.StatusDeleted {
		t.Fatalf("expected rolled back user status=%d, got %d", auth.StatusDeleted, created.Status)
	}
}

func TestAdminUsersCreateRejectUnknownFields(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/auth/users", strings.NewReader(`{
		"username":"unknown-field-user",
		"password":"secret",
		"role":"user",
		"oops":"unexpected"
	}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown create-user field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestAdminUsersListOffsetBeyondTotalReturnsEmpty(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	for _, username := range []string{"offset-u1", "offset-u2", "offset-u3"} {
		if _, err := authSvc.Register(username, "secret", auth.RoleUser); err != nil {
			t.Fatalf("register %s: %v", username, err)
		}
	}
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/users?offset=100", nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for users list with large offset, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data  []auth.User `json:"data"`
		Total int         `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode users payload: %v", err)
	}
	if payload.Total != 3 {
		t.Fatalf("expected total=3, got %d", payload.Total)
	}
	if len(payload.Data) != 0 {
		t.Fatalf("expected empty data for offset beyond total, got %d entries", len(payload.Data))
	}
}

func TestAdminUsersListSortedByUsername(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	for _, username := range []string{"zeta-user", "beta-user", "alpha-user"} {
		if _, err := authSvc.Register(username, "secret", auth.RoleUser); err != nil {
			t.Fatalf("register %s: %v", username, err)
		}
	}
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/users", nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for users list, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var payload struct {
		Data []auth.User `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode users payload: %v", err)
	}
	if len(payload.Data) != 3 {
		t.Fatalf("expected 3 users, got %d", len(payload.Data))
	}
	got := []string{payload.Data[0].Username, payload.Data[1].Username, payload.Data[2].Username}
	want := []string{"alpha-user", "beta-user", "zeta-user"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected sorted usernames %v, got %v", want, got)
		}
	}
}

func TestAdminChannelStatusRejectUnknownFields(t *testing.T) {
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		ChannelStore: channel.NewAbilityStore(),
		AdminToken:   "secret-admin",
	})

	createBody := `{
		"name":"status-field-check",
		"type":"openai",
		"models":"gpt-4o",
		"group":"default",
		"status":1
	}`
	reqCreate := httptest.NewRequest(http.MethodPost, "/admin/channels", strings.NewReader(createBody))
	reqCreate.Header.Set("authorization", "Bearer secret-admin")
	rrCreate := httptest.NewRecorder()
	router.ServeHTTP(rrCreate, reqCreate)
	if rrCreate.Code != http.StatusCreated {
		t.Fatalf("expected 201 for channel create, got %d; body=%s", rrCreate.Code, rrCreate.Body.String())
	}
	var created channel.Channel
	if err := json.Unmarshal(rrCreate.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created channel: %v", err)
	}

	reqStatus := httptest.NewRequest(http.MethodPut, "/admin/channels/"+strconv.FormatInt(created.ID, 10)+"/status", strings.NewReader(`{"status":2,"extra":true}`))
	reqStatus.Header.Set("authorization", "Bearer secret-admin")
	rrStatus := httptest.NewRecorder()
	router.ServeHTTP(rrStatus, reqStatus)
	if rrStatus.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown status payload field, got %d; body=%s", rrStatus.Code, rrStatus.Body.String())
	}
}

func TestAdminUserPathNotImplementedWithoutAuthService(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken: "secret-admin",
	})
	for _, path := range []string{
		"/admin/auth/users/u-1",
		"/admin/auth/users/u-1/quota",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("authorization", "Bearer secret-admin")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotImplemented {
			t.Fatalf("expected 501 for %s without auth service, got %d; body=%s", path, rr.Code, rr.Body.String())
		}
		var env ErrorEnvelope
		if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode error envelope for %s: %v", path, err)
		}
		if !strings.Contains(env.Error.Message, "auth service not configured") {
			t.Fatalf("expected auth service not configured for %s, got %q", path, env.Error.Message)
		}
	}
}

func TestAdminTokenPathNotImplementedWithoutTokenService(t *testing.T) {
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken: "secret-admin",
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/auth/tokens/u-1/1", nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for token path without token service, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode token path error: %v", err)
	}
	if !strings.Contains(env.Error.Message, "token service not configured") {
		t.Fatalf("expected token service not configured message, got %q", env.Error.Message)
	}
}

func TestAdminUserTokenByIDNotImplementedWithoutTokenService(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("user-no-token-svc", "secret", auth.RoleUser)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	path := "/admin/auth/users/" + user.ID + "/tokens/1"
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 for user token path without token service, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode user token path error: %v", err)
	}
	if !strings.Contains(env.Error.Message, "token service not configured") {
		t.Fatalf("expected token service not configured message, got %q", env.Error.Message)
	}
}

func TestAdminUserStatusZeroNormalizesToDisabled(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("status-zero-user", "secret", auth.RoleUser)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:  "secret-admin",
		AuthService: authSvc,
	})

	path := "/admin/auth/users/" + user.ID
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"status":0}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for user status update, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var updated auth.User
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated user: %v", err)
	}
	if updated.Status != auth.StatusDisabled {
		t.Fatalf("expected normalized status=%d, got %d", auth.StatusDisabled, updated.Status)
	}
	stored, err := authSvc.Get(user.ID)
	if err != nil {
		t.Fatalf("get stored user: %v", err)
	}
	if stored.Status != auth.StatusDisabled {
		t.Fatalf("expected stored status=%d, got %d", auth.StatusDisabled, stored.Status)
	}
}

func TestAdminUserTokenStatusZeroNormalizesToDisabled(t *testing.T) {
	authSvc := auth.NewInMemoryService()
	user, err := authSvc.Register("status-zero-token-user", "secret", auth.RoleUser)
	if err != nil {
		t.Fatalf("register user: %v", err)
	}
	tokenSvc := token.NewInMemoryService()
	tk, err := tokenSvc.Generate(user.ID, 100)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		AdminToken:   "secret-admin",
		AuthService:  authSvc,
		TokenService: tokenSvc,
	})
	path := "/admin/auth/users/" + user.ID + "/tokens/" + strconv.FormatInt(tk.ID, 10)
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(`{"status":0}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for token status update, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var updated token.Token
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated token: %v", err)
	}
	if updated.Status != token.StatusDisabled {
		t.Fatalf("expected normalized status=%d, got %d", token.StatusDisabled, updated.Status)
	}
	if _, err := tokenSvc.Validate(tk.Value); err != token.ErrTokenDisabled {
		t.Fatalf("expected token disabled validation error, got %v", err)
	}
}

func TestAdminMarketplaceCloudListRejectsInvalidManifest(t *testing.T) {
	router := NewRouter(Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		AdminToken:   "secret-admin",
	})

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(`[
				{
					"name":"bad_plugin",
					"version":"1.0.0",
					"description":"bad manifest",
					"author":"tester",
					"homepage":"https://example.com/plugin"
				}
			]`)),
		}, nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/marketplace/cloud/list", strings.NewReader(`{"url":"http://8.8.8.8/manifests.json"}`))
	req.Header.Set("authorization", "Bearer secret-admin")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cloud manifest, got %d; body=%s", rr.Code, rr.Body.String())
	}

	var env ErrorEnvelope
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode cloud list invalid-manifest error: %v", err)
	}
	if !strings.Contains(env.Error.Message, "invalid cloud manifest") {
		t.Fatalf("expected invalid cloud manifest error, got %q", env.Error.Message)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type failingAuthService struct {
	auth.Service
	updateErr   error
	addQuotaErr error
}

func (s *failingAuthService) Update(user *auth.User) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.Service.Update(user)
}

func (s *failingAuthService) AddQuota(userID string, quota int64) error {
	if s.addQuotaErr != nil {
		return s.addQuotaErr
	}
	return s.Service.AddQuota(userID, quota)
}
