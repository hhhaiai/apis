package gateway_test

import (
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/plugin"
	"ccgateway/internal/policy"
)

func TestCCPluginsCRUDAndEvents(t *testing.T) {
	pluginStore := plugin.NewManager()
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PluginStore:  pluginStore,
		EventStore:   eventStore,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plugins", strings.NewReader(`{
		"name":"planner_pack",
		"version":"1.2.0",
		"description":"planning helpers",
		"skills":[{"name":"plan_check","template":"check {{task}}"}],
		"hooks":[{"name":"post_reflect","point":"post_response","priority":10}],
		"mcp_servers":[{"name":"local_fs","transport":"stdio","command":"npx"}]
	}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}

	var created plugin.Plugin
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created plugin: %v", err)
	}
	if created.Name != "planner_pack" {
		t.Fatalf("unexpected plugin name: %q", created.Name)
	}
	if !created.Enabled {
		t.Fatalf("new plugin should be enabled")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins?limit=1", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []plugin.Plugin `json:"data"`
		Count int             `json:"count"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if listResp.Count != 1 || listResp.Total != 1 || len(listResp.Data) != 1 {
		t.Fatalf("unexpected list payload: %+v", listResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins/planner_pack", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", getRR.Code, getRR.Body.String())
	}

	disableReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plugins/planner_pack/disable", nil)
	disableRR := httptest.NewRecorder()
	router.ServeHTTP(disableRR, disableReq)
	if disableRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for disable, got %d; body=%s", disableRR.Code, disableRR.Body.String())
	}
	var disabled plugin.Plugin
	if err := json.Unmarshal(disableRR.Body.Bytes(), &disabled); err != nil {
		t.Fatalf("unmarshal disable payload: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("expected disabled plugin")
	}

	enableReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plugins/planner_pack/enable", nil)
	enableRR := httptest.NewRecorder()
	router.ServeHTTP(enableRR, enableReq)
	if enableRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for enable, got %d; body=%s", enableRR.Code, enableRR.Body.String())
	}
	var enabled plugin.Plugin
	if err := json.Unmarshal(enableRR.Body.Bytes(), &enabled); err != nil {
		t.Fatalf("unmarshal enable payload: %v", err)
	}
	if !enabled.Enabled {
		t.Fatalf("expected enabled plugin")
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/cc/plugins/planner_pack", nil)
	deleteRR := httptest.NewRecorder()
	router.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	getAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins/planner_pack", nil)
	getAfterDeleteRR := httptest.NewRecorder()
	router.ServeHTTP(getAfterDeleteRR, getAfterDeleteReq)
	if getAfterDeleteRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d; body=%s", getAfterDeleteRR.Code, getAfterDeleteRR.Body.String())
	}

	events := eventStore.List(ccevent.ListFilter{Limit: 20})
	found := map[string]bool{}
	for _, ev := range events {
		found[ev.EventType] = true
		recordText, _ := ev.Data["record_text"].(string)
		if strings.TrimSpace(recordText) == "" {
			t.Fatalf("expected record_text for event %q", ev.EventType)
		}
	}
	for _, want := range []string{
		"plugin.installed",
		"plugin.disabled",
		"plugin.enabled",
		"plugin.uninstalled",
	} {
		if !found[want] {
			t.Fatalf("missing plugin event %q in %+v", want, found)
		}
	}
}

func TestCCPluginsNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCPluginsProjectScopeIsolation(t *testing.T) {
	pluginStore := plugin.NewManager()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PluginStore:  pluginStore,
	})

	createAlpha := httptest.NewRequest(http.MethodPost, "/v1/cc/plugins?scope=project&project_id=alpha", strings.NewReader(`{"name":"planner_pack","version":"1.0.0"}`))
	rrCreateAlpha := httptest.NewRecorder()
	router.ServeHTTP(rrCreateAlpha, createAlpha)
	if rrCreateAlpha.Code != http.StatusCreated {
		t.Fatalf("expected 201 for alpha create, got %d; body=%s", rrCreateAlpha.Code, rrCreateAlpha.Body.String())
	}

	createBeta := httptest.NewRequest(http.MethodPost, "/v1/cc/plugins?scope=project&project_id=beta", strings.NewReader(`{"name":"planner_pack","version":"1.0.0"}`))
	rrCreateBeta := httptest.NewRecorder()
	router.ServeHTTP(rrCreateBeta, createBeta)
	if rrCreateBeta.Code != http.StatusCreated {
		t.Fatalf("expected 201 for beta create, got %d; body=%s", rrCreateBeta.Code, rrCreateBeta.Body.String())
	}

	listAlpha := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins?scope=project&project_id=alpha", nil)
	rrListAlpha := httptest.NewRecorder()
	router.ServeHTTP(rrListAlpha, listAlpha)
	if rrListAlpha.Code != http.StatusOK {
		t.Fatalf("expected 200 for alpha list, got %d; body=%s", rrListAlpha.Code, rrListAlpha.Body.String())
	}
	var alphaPayload struct {
		Data []plugin.Plugin `json:"data"`
	}
	if err := json.Unmarshal(rrListAlpha.Body.Bytes(), &alphaPayload); err != nil {
		t.Fatalf("decode alpha payload: %v", err)
	}
	if len(alphaPayload.Data) != 1 || alphaPayload.Data[0].Name != "planner_pack" {
		t.Fatalf("unexpected alpha plugins: %+v", alphaPayload.Data)
	}

	listGlobal := httptest.NewRequest(http.MethodGet, "/v1/cc/plugins?scope=global", nil)
	rrListGlobal := httptest.NewRecorder()
	router.ServeHTTP(rrListGlobal, listGlobal)
	if rrListGlobal.Code != http.StatusOK {
		t.Fatalf("expected 200 for global list, got %d; body=%s", rrListGlobal.Code, rrListGlobal.Body.String())
	}
	var globalPayload struct {
		Data []plugin.Plugin `json:"data"`
	}
	if err := json.Unmarshal(rrListGlobal.Body.Bytes(), &globalPayload); err != nil {
		t.Fatalf("decode global payload: %v", err)
	}
	if len(globalPayload.Data) != 0 {
		t.Fatalf("expected no global plugins, got %+v", globalPayload.Data)
	}
}

func TestCCPluginsRejectUnknownFieldsOnCreate(t *testing.T) {
	pluginStore := plugin.NewManager()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PluginStore:  pluginStore,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/plugins", strings.NewReader(`{"name":"planner_pack","version":"1.0.0","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
