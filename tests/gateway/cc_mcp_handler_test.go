package gateway_test

import (
	. "ccgateway/internal/gateway"
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
)

func TestCCMCPServersCRUDAndHealth(t *testing.T) {
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		var resp map[string]any
		switch method {
		case "tools/list":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"tools": []map[string]any{
						{"name": "remote_search", "description": "search"},
					},
				},
			}
		case "tools/call":
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result": map[string]any{
					"isError": false,
					"content": "ok-from-mcp-call",
				},
			}
		default:
			resp = map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"error": map[string]any{
					"message": "unsupported",
				},
			}
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer healthyUpstream.Close()

	registry := mcpregistry.NewStore(healthyUpstream.Client())
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		MCPRegistry:  registry,
	})

	createBody := `{"id":"mcp_http_1","name":"local-http","transport":"http","url":"` + healthyUpstream.URL + `","timeout_ms":5000}`
	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers", strings.NewReader(createBody))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}
	var created mcpregistry.Server
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created server: %v", err)
	}
	if created.ID != "mcp_http_1" {
		t.Fatalf("unexpected id: %q", created.ID)
	}

	healthReq := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers/mcp_http_1/health", nil)
	healthRR := httptest.NewRecorder()
	router.ServeHTTP(healthRR, healthReq)
	if healthRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", healthRR.Code, healthRR.Body.String())
	}
	var healthChecked mcpregistry.Server
	if err := json.Unmarshal(healthRR.Body.Bytes(), &healthChecked); err != nil {
		t.Fatalf("unmarshal health result: %v", err)
	}
	if !healthChecked.Status.Healthy {
		t.Fatalf("expected healthy=true, got %+v", healthChecked.Status)
	}

	reconnectReq := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers/mcp_http_1/reconnect", nil)
	reconnectRR := httptest.NewRecorder()
	router.ServeHTTP(reconnectRR, reconnectReq)
	if reconnectRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", reconnectRR.Code, reconnectRR.Body.String())
	}
	var reconnected mcpregistry.Server
	if err := json.Unmarshal(reconnectRR.Body.Bytes(), &reconnected); err != nil {
		t.Fatalf("unmarshal reconnect result: %v", err)
	}
	if !reconnected.Status.Healthy {
		t.Fatalf("expected reconnect healthy=true, got %+v", reconnected.Status)
	}

	toolsListReq := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers/mcp_http_1/tools/list", strings.NewReader(`{}`))
	toolsListRR := httptest.NewRecorder()
	router.ServeHTTP(toolsListRR, toolsListReq)
	if toolsListRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for tools/list, got %d; body=%s", toolsListRR.Code, toolsListRR.Body.String())
	}
	var toolsList struct {
		ServerID string             `json:"server_id"`
		Tools    []mcpregistry.Tool `json:"tools"`
		Count    int                `json:"count"`
	}
	if err := json.Unmarshal(toolsListRR.Body.Bytes(), &toolsList); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if toolsList.Count != 1 || len(toolsList.Tools) != 1 || toolsList.Tools[0].Name != "remote_search" {
		t.Fatalf("unexpected tools/list payload: %+v", toolsList)
	}

	toolsCallReq := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers/mcp_http_1/tools/call", strings.NewReader(`{"name":"remote_search","arguments":{"q":"hello"}}`))
	toolsCallRR := httptest.NewRecorder()
	router.ServeHTTP(toolsCallRR, toolsCallReq)
	if toolsCallRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for tools/call, got %d; body=%s", toolsCallRR.Code, toolsCallRR.Body.String())
	}
	var toolsCall mcpregistry.ToolCallResult
	if err := json.Unmarshal(toolsCallRR.Body.Bytes(), &toolsCall); err != nil {
		t.Fatalf("unmarshal tools/call: %v", err)
	}
	if toolsCall.ToolName != "remote_search" {
		t.Fatalf("unexpected tool name: %q", toolsCall.ToolName)
	}
	if content, ok := toolsCall.Content.(string); !ok || strings.TrimSpace(content) != "ok-from-mcp-call" {
		t.Fatalf("unexpected tools/call content: %#v", toolsCall.Content)
	}

	updateBody := `{"timeout_ms":12000}`
	updateReq := httptest.NewRequest(http.MethodPut, "/v1/cc/mcp/servers/mcp_http_1", strings.NewReader(updateBody))
	updateRR := httptest.NewRecorder()
	router.ServeHTTP(updateRR, updateReq)
	if updateRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", updateRR.Code, updateRR.Body.String())
	}
	var updated mcpregistry.Server
	if err := json.Unmarshal(updateRR.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal updated server: %v", err)
	}
	if updated.TimeoutMS != 12000 {
		t.Fatalf("expected timeout update, got %d", updated.TimeoutMS)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/mcp/servers/mcp_http_1", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", getRR.Code, getRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/mcp/servers?limit=1", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []mcpregistry.Server `json:"data"`
		Count int                  `json:"count"`
		Total int                  `json:"total"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if listResp.Count != 1 || listResp.Total != 1 {
		t.Fatalf("unexpected list counters: %+v", listResp)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/cc/mcp/servers/mcp_http_1", nil)
	deleteRR := httptest.NewRecorder()
	router.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d; body=%s", deleteRR.Code, deleteRR.Body.String())
	}

	getAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/v1/cc/mcp/servers/mcp_http_1", nil)
	getAfterDeleteRR := httptest.NewRecorder()
	router.ServeHTTP(getAfterDeleteRR, getAfterDeleteReq)
	if getAfterDeleteRR.Code != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d; body=%s", getAfterDeleteRR.Code, getAfterDeleteRR.Body.String())
	}
}

func TestCCMCPServersNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/cc/mcp/servers", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCMCPServersRejectUnknownFieldsOnCreate(t *testing.T) {
	registry := mcpregistry.NewStore(nil)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		MCPRegistry:  registry,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers", strings.NewReader(`{"name":"x","transport":"http","url":"http://127.0.0.1","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCMCPServersProjectScopeIsolation(t *testing.T) {
	healthyUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      "1",
			"result": map[string]any{
				"tools": []map[string]any{
					{"name": "remote_search"},
				},
			},
		})
	}))
	defer healthyUpstream.Close()

	registry := mcpregistry.NewStore(healthyUpstream.Client())
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		MCPRegistry:  registry,
	})

	body := `{"id":"shared-id","name":"scoped","transport":"http","url":"` + healthyUpstream.URL + `"}`
	reqAlpha := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers?scope=project&project_id=alpha", strings.NewReader(body))
	rrAlpha := httptest.NewRecorder()
	router.ServeHTTP(rrAlpha, reqAlpha)
	if rrAlpha.Code != http.StatusCreated {
		t.Fatalf("expected 201 for alpha create, got %d; body=%s", rrAlpha.Code, rrAlpha.Body.String())
	}

	reqBeta := httptest.NewRequest(http.MethodPost, "/v1/cc/mcp/servers?scope=project&project_id=beta", strings.NewReader(body))
	rrBeta := httptest.NewRecorder()
	router.ServeHTTP(rrBeta, reqBeta)
	if rrBeta.Code != http.StatusCreated {
		t.Fatalf("expected 201 for beta create, got %d; body=%s", rrBeta.Code, rrBeta.Body.String())
	}

	listAlpha := httptest.NewRequest(http.MethodGet, "/v1/cc/mcp/servers?scope=project&project_id=alpha", nil)
	rrListAlpha := httptest.NewRecorder()
	router.ServeHTTP(rrListAlpha, listAlpha)
	if rrListAlpha.Code != http.StatusOK {
		t.Fatalf("expected 200 for alpha list, got %d; body=%s", rrListAlpha.Code, rrListAlpha.Body.String())
	}
	var alphaPayload struct {
		Data []mcpregistry.Server `json:"data"`
	}
	if err := json.Unmarshal(rrListAlpha.Body.Bytes(), &alphaPayload); err != nil {
		t.Fatalf("decode alpha list: %v", err)
	}
	if len(alphaPayload.Data) != 1 || alphaPayload.Data[0].ID != "shared-id" {
		t.Fatalf("unexpected alpha servers: %+v", alphaPayload.Data)
	}

	getWrongScope := httptest.NewRequest(http.MethodGet, "/v1/cc/mcp/servers/shared-id?scope=project&project_id=gamma", nil)
	rrGetWrongScope := httptest.NewRecorder()
	router.ServeHTTP(rrGetWrongScope, getWrongScope)
	if rrGetWrongScope.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for wrong project scope, got %d; body=%s", rrGetWrongScope.Code, rrGetWrongScope.Body.String())
	}
}
