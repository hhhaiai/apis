package gateway_test

import (
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/probe"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/settings"
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
