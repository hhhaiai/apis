package gateway_test

import (
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestRootHomePage(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
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
