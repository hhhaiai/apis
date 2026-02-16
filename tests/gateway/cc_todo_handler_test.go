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
	"ccgateway/internal/todo"
)

func TestCCTodosCreateGetListUpdate(t *testing.T) {
	st := todo.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TodoStore:    st,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/todos", strings.NewReader(`{"title":"ship mcp","session_id":"sess_1","plan_id":"plan_1","status":"pending","metadata":{"priority":"p1"}}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}

	var created todo.Todo
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created todo: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected todo id")
	}
	if created.Status != todo.StatusPending {
		t.Fatalf("expected pending status, got %q", created.Status)
	}
	if created.PlanID != "plan_1" {
		t.Fatalf("expected plan_id=plan_1, got %q", created.PlanID)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/todos/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", getRR.Code, getRR.Body.String())
	}

	inProgress := httptest.NewRequest(http.MethodPut, "/v1/cc/todos/"+created.ID, strings.NewReader(`{"status":"in_progress"}`))
	inProgressRR := httptest.NewRecorder()
	router.ServeHTTP(inProgressRR, inProgress)
	if inProgressRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on in_progress update, got %d; body=%s", inProgressRR.Code, inProgressRR.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPut, "/v1/cc/todos/"+created.ID, strings.NewReader(`{"status":"completed"}`))
	completeRR := httptest.NewRecorder()
	router.ServeHTTP(completeRR, completeReq)
	if completeRR.Code != http.StatusOK {
		t.Fatalf("expected 200 on completed update, got %d; body=%s", completeRR.Code, completeRR.Body.String())
	}
	var completed todo.Todo
	if err := json.Unmarshal(completeRR.Body.Bytes(), &completed); err != nil {
		t.Fatalf("unmarshal completed todo: %v", err)
	}
	if completed.Status != todo.StatusCompleted {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatalf("expected completed_at set")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?status=completed&session_id=sess_1&plan_id=plan_1&limit=1", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []todo.Todo `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal todo list: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Data) != 1 {
		t.Fatalf("unexpected list payload: %+v", listResp)
	}
}

func TestCCTodosNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/cc/todos", strings.NewReader(`{"title":"x"}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCTodosBadLimit(t *testing.T) {
	st := todo.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TodoStore:    st,
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?limit=-1", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCTodosRejectUnknownFieldsOnCreate(t *testing.T) {
	st := todo.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TodoStore:    st,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/todos", strings.NewReader(`{"title":"task","session_id":"sess_1","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCTodosUpdateRejectTrailingJSON(t *testing.T) {
	st := todo.NewStore()
	created, err := st.Create(todo.CreateInput{
		Title:     "todo item",
		SessionID: "sess_1",
	})
	if err != nil {
		t.Fatalf("create todo: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TodoStore:    st,
	})

	req := httptest.NewRequest(http.MethodPut, "/v1/cc/todos/"+created.ID, strings.NewReader(`{"status":"completed"} {}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
