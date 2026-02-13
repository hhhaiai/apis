package gateway_test

import (
	. "ccgateway/internal/gateway"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/session"
)

func TestCCSessionsCreateGetList(t *testing.T) {
	st := session.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		SessionStore: st,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/sessions", strings.NewReader(`{"title":"workspace","metadata":{"mode":"plan"}}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}

	var created session.Session
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created session: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected non-empty session id")
	}
	if created.Title != "workspace" {
		t.Fatalf("unexpected title: %q", created.Title)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/sessions/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", getRR.Code, getRR.Body.String())
	}
	var got session.Session
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get session: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected same session id, got %q", got.ID)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/sessions?limit=1", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []session.Session `json:"data"`
		Count int               `json:"count"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if listResp.Count != 1 || listResp.Total != 1 {
		t.Fatalf("unexpected list counters: %+v", listResp)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != created.ID {
		t.Fatalf("unexpected list payload: %+v", listResp.Data)
	}
}

func TestCCSessionsFork(t *testing.T) {
	st := session.NewStore()
	parent, err := st.Create(session.CreateInput{
		ID:    "sess_parent",
		Title: "parent",
		Metadata: map[string]any{
			"topic": "alpha",
		},
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		SessionStore: st,
	})

	forkReq := httptest.NewRequest(http.MethodPost, "/v1/cc/sessions/"+parent.ID+"/fork", strings.NewReader(`{"title":"child"}`))
	forkRR := httptest.NewRecorder()
	router.ServeHTTP(forkRR, forkReq)
	if forkRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", forkRR.Code, forkRR.Body.String())
	}
	var child session.Session
	if err := json.Unmarshal(forkRR.Body.Bytes(), &child); err != nil {
		t.Fatalf("unmarshal child session: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Fatalf("expected parent_id %q, got %q", parent.ID, child.ParentID)
	}
	if child.Title != "child" {
		t.Fatalf("expected child title, got %q", child.Title)
	}
	if child.Metadata["topic"] != "alpha" {
		t.Fatalf("expected inherited metadata, got %#v", child.Metadata)
	}
}

func TestCCSessionsNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/cc/sessions", strings.NewReader(`{"title":"x"}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
