package gateway_test

import (
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/ccrun"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
)

func TestCCRunsListGet(t *testing.T) {
	runStore := ccrun.NewStore()
	_, _ = runStore.Create(ccrun.CreateInput{
		ID:        "run_a",
		SessionID: "sess_1",
		Path:      "/v1/messages",
		Mode:      "chat",
	})
	_, _ = runStore.Create(ccrun.CreateInput{
		ID:        "run_b",
		SessionID: "sess_1",
		Path:      "/v1/chat/completions",
		Mode:      "plan",
	})
	_, _ = runStore.Complete("run_a", ccrun.CompleteInput{StatusCode: 200})
	_, _ = runStore.Complete("run_b", ccrun.CompleteInput{StatusCode: 500, Error: "boom"})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		RunStore:     runStore,
	})

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/runs?session_id=sess_1&status=failed", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []ccrun.Run `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal runs list: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Data) != 1 || listResp.Data[0].ID != "run_b" {
		t.Fatalf("unexpected runs list payload: %+v", listResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/runs/run_a", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 get, got %d; body=%s", getRR.Code, getRR.Body.String())
	}
	var got ccrun.Run
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	if got.ID != "run_a" {
		t.Fatalf("unexpected run id: %q", got.ID)
	}
}

func TestCCRunsTrackedByRequests(t *testing.T) {
	runStore := ccrun.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		RunStore:     runStore,
	})

	msgReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-test","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`))
	msgReq.Header.Set("anthropic-version", "2023-06-01")
	msgReq.Header.Set("x-cc-session-id", "sess_req")
	msgRR := httptest.NewRecorder()
	router.ServeHTTP(msgRR, msgReq)
	if msgRR.Code != http.StatusOK {
		t.Fatalf("expected 200 messages, got %d; body=%s", msgRR.Code, msgRR.Body.String())
	}

	chatReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"stream":false,"metadata":{"session_id":"sess_req"}}`))
	chatRR := httptest.NewRecorder()
	router.ServeHTTP(chatRR, chatReq)
	if chatRR.Code != http.StatusOK {
		t.Fatalf("expected 200 chat, got %d; body=%s", chatRR.Code, chatRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/runs?session_id=sess_req", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 runs list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []ccrun.Run `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal runs list: %v", err)
	}
	if listResp.Count < 2 || len(listResp.Data) < 2 {
		t.Fatalf("expected at least 2 runs, got %+v", listResp)
	}
	for _, run := range listResp.Data {
		if run.Status != ccrun.StatusCompleted {
			t.Fatalf("expected completed run status, got %q", run.Status)
		}
	}
}

func TestCCRunsNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/cc/runs", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCRunsRequireAuthWhenAdminTokenConfigured(t *testing.T) {
	runStore := ccrun.NewStore()
	_, _ = runStore.Create(ccrun.CreateInput{
		ID:   "run_sec",
		Path: "/v1/messages",
	})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		RunStore:     runStore,
		AdminToken:   "secret-admin",
	})

	reqNoAuth := httptest.NewRequest(http.MethodGet, "/v1/cc/runs", nil)
	rrNoAuth := httptest.NewRecorder()
	router.ServeHTTP(rrNoAuth, reqNoAuth)
	if rrNoAuth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d; body=%s", rrNoAuth.Code, rrNoAuth.Body.String())
	}

	reqAuth := httptest.NewRequest(http.MethodGet, "/v1/cc/runs", nil)
	reqAuth.Header.Set("authorization", "Bearer secret-admin")
	rrAuth := httptest.NewRecorder()
	router.ServeHTTP(rrAuth, reqAuth)
	if rrAuth.Code != http.StatusOK {
		t.Fatalf("expected 200 with auth, got %d; body=%s", rrAuth.Code, rrAuth.Body.String())
	}
}
