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
	"ccgateway/internal/plan"
	"ccgateway/internal/policy"
	"ccgateway/internal/todo"
)

func TestCCPlansCreateGetListApproveExecute(t *testing.T) {
	planStore := plan.NewStore()
	todoStore := todo.NewStore()
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PlanStore:    planStore,
		TodoStore:    todoStore,
		EventStore:   eventStore,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans", strings.NewReader(`{"title":"release plan","summary":"ship features","session_id":"sess_1","steps":[{"title":"analyze"},{"title":"implement"}]}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}
	var created plan.Plan
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created plan: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected plan id")
	}
	if created.Status != plan.StatusDraft {
		t.Fatalf("expected draft status, got %q", created.Status)
	}
	todoListReq := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?plan_id="+created.ID, nil)
	todoListRR := httptest.NewRecorder()
	router.ServeHTTP(todoListRR, todoListReq)
	if todoListRR.Code != http.StatusOK {
		t.Fatalf("expected 200 todo list, got %d; body=%s", todoListRR.Code, todoListRR.Body.String())
	}
	var createdTodos struct {
		Data  []todo.Todo `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(todoListRR.Body.Bytes(), &createdTodos); err != nil {
		t.Fatalf("unmarshal plan todos: %v", err)
	}
	if createdTodos.Count != 2 || len(createdTodos.Data) != 2 {
		t.Fatalf("expected 2 auto todos, got %+v", createdTodos)
	}
	for _, td := range createdTodos.Data {
		if td.Status != todo.StatusPending {
			t.Fatalf("expected pending after plan create, got %q", td.Status)
		}
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/plans/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", getRR.Code, getRR.Body.String())
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/approve", strings.NewReader(`{}`))
	approveRR := httptest.NewRecorder()
	router.ServeHTTP(approveRR, approveReq)
	if approveRR.Code != http.StatusOK {
		t.Fatalf("expected 200 approve, got %d; body=%s", approveRR.Code, approveRR.Body.String())
	}

	execReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/execute", strings.NewReader(`{}`))
	execRR := httptest.NewRecorder()
	router.ServeHTTP(execRR, execReq)
	if execRR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute start, got %d; body=%s", execRR.Code, execRR.Body.String())
	}
	inProgressReq := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?plan_id="+created.ID+"&status=in_progress", nil)
	inProgressRR := httptest.NewRecorder()
	router.ServeHTTP(inProgressRR, inProgressReq)
	if inProgressRR.Code != http.StatusOK {
		t.Fatalf("expected 200 in_progress list, got %d; body=%s", inProgressRR.Code, inProgressRR.Body.String())
	}
	var inProgressTodos struct {
		Data  []todo.Todo `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(inProgressRR.Body.Bytes(), &inProgressTodos); err != nil {
		t.Fatalf("unmarshal in_progress todos: %v", err)
	}
	if inProgressTodos.Count != 1 || len(inProgressTodos.Data) != 1 {
		t.Fatalf("expected 1 in_progress todo after single step execute, got %+v", inProgressTodos)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/execute", strings.NewReader(`{"complete":true}`))
	completeRR := httptest.NewRecorder()
	router.ServeHTTP(completeRR, completeReq)
	if completeRR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute complete, got %d; body=%s", completeRR.Code, completeRR.Body.String())
	}
	var completed plan.Plan
	if err := json.Unmarshal(completeRR.Body.Bytes(), &completed); err != nil {
		t.Fatalf("unmarshal completed plan: %v", err)
	}
	if completed.Status != plan.StatusCompleted {
		t.Fatalf("expected completed status, got %q", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Fatalf("expected completed_at")
	}
	completedTodoReq := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?plan_id="+created.ID+"&status=completed", nil)
	completedTodoRR := httptest.NewRecorder()
	router.ServeHTTP(completedTodoRR, completedTodoReq)
	if completedTodoRR.Code != http.StatusOK {
		t.Fatalf("expected 200 completed todo list, got %d; body=%s", completedTodoRR.Code, completedTodoRR.Body.String())
	}
	var completedTodos struct {
		Data  []todo.Todo `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(completedTodoRR.Body.Bytes(), &completedTodos); err != nil {
		t.Fatalf("unmarshal completed todos: %v", err)
	}
	if completedTodos.Count != 2 || len(completedTodos.Data) != 2 {
		t.Fatalf("expected 2 completed todos, got %+v", completedTodos)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/plans?status=completed&session_id=sess_1", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []plan.Plan `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Data) != 1 {
		t.Fatalf("unexpected list payload: %+v", listResp)
	}
}

func TestCCPlansNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/cc/plans", strings.NewReader(`{"title":"x"}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCPlansStepAdvanceAutoComplete(t *testing.T) {
	planStore := plan.NewStore()
	todoStore := todo.NewStore()
	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PlanStore:    planStore,
		TodoStore:    todoStore,
		EventStore:   eventStore,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans", strings.NewReader(`{"title":"step-plan","session_id":"sess_s","steps":[{"title":"s1"},{"title":"s2"}]}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}
	var created plan.Plan
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/approve", strings.NewReader(`{}`))
	approveRR := httptest.NewRecorder()
	router.ServeHTTP(approveRR, approveReq)
	if approveRR.Code != http.StatusOK {
		t.Fatalf("expected 200 approve, got %d; body=%s", approveRR.Code, approveRR.Body.String())
	}

	exec1 := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/execute", strings.NewReader(`{}`))
	exec1RR := httptest.NewRecorder()
	router.ServeHTTP(exec1RR, exec1)
	if exec1RR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute#1, got %d; body=%s", exec1RR.Code, exec1RR.Body.String())
	}
	var planAfter1 plan.Plan
	if err := json.Unmarshal(exec1RR.Body.Bytes(), &planAfter1); err != nil {
		t.Fatalf("unmarshal plan after execute#1: %v", err)
	}
	if planAfter1.Status != plan.StatusExecuting {
		t.Fatalf("expected executing after execute#1, got %q", planAfter1.Status)
	}

	exec2 := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/execute", strings.NewReader(`{}`))
	exec2RR := httptest.NewRecorder()
	router.ServeHTTP(exec2RR, exec2)
	if exec2RR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute#2, got %d; body=%s", exec2RR.Code, exec2RR.Body.String())
	}
	var planAfter2 plan.Plan
	if err := json.Unmarshal(exec2RR.Body.Bytes(), &planAfter2); err != nil {
		t.Fatalf("unmarshal plan after execute#2: %v", err)
	}
	if planAfter2.Status != plan.StatusExecuting {
		t.Fatalf("expected executing after execute#2, got %q", planAfter2.Status)
	}
	listCompletedAfter2Req := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?plan_id="+created.ID+"&status=completed", nil)
	listCompletedAfter2RR := httptest.NewRecorder()
	router.ServeHTTP(listCompletedAfter2RR, listCompletedAfter2Req)
	var completedAfter2 struct {
		Data  []todo.Todo `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(listCompletedAfter2RR.Body.Bytes(), &completedAfter2); err != nil {
		t.Fatalf("unmarshal completed after execute#2: %v", err)
	}
	if completedAfter2.Count != 1 {
		t.Fatalf("expected 1 completed todo after execute#2, got %d", completedAfter2.Count)
	}

	exec3 := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/execute", strings.NewReader(`{}`))
	exec3RR := httptest.NewRecorder()
	router.ServeHTTP(exec3RR, exec3)
	if exec3RR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute#3, got %d; body=%s", exec3RR.Code, exec3RR.Body.String())
	}
	var planAfter3 plan.Plan
	if err := json.Unmarshal(exec3RR.Body.Bytes(), &planAfter3); err != nil {
		t.Fatalf("unmarshal plan after execute#3: %v", err)
	}
	if planAfter3.Status != plan.StatusCompleted {
		t.Fatalf("expected auto completed after execute#3, got %q", planAfter3.Status)
	}

	listCompletedReq := httptest.NewRequest(http.MethodGet, "/v1/cc/todos?plan_id="+created.ID+"&status=completed", nil)
	listCompletedRR := httptest.NewRecorder()
	router.ServeHTTP(listCompletedRR, listCompletedReq)
	var completedTodos struct {
		Data  []todo.Todo `json:"data"`
		Count int         `json:"count"`
	}
	if err := json.Unmarshal(listCompletedRR.Body.Bytes(), &completedTodos); err != nil {
		t.Fatalf("unmarshal completed todos final: %v", err)
	}
	if completedTodos.Count != 2 {
		t.Fatalf("expected 2 completed todos after execute#3, got %d", completedTodos.Count)
	}
}

func TestCCPlansRejectUnknownFieldsOnCreate(t *testing.T) {
	planStore := plan.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PlanStore:    planStore,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/plans", strings.NewReader(`{"title":"release","session_id":"sess_1","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCPlanApproveAllowsEmptyBodyAndRejectsTrailingJSON(t *testing.T) {
	planStore := plan.NewStore()
	created, err := planStore.Create(plan.CreateInput{
		Title:     "plan",
		SessionID: "sess_1",
	})
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PlanStore:    planStore,
	})

	emptyReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created.ID+"/approve", nil)
	emptyRR := httptest.NewRecorder()
	router.ServeHTTP(emptyRR, emptyReq)
	if emptyRR.Code != http.StatusOK {
		t.Fatalf("expected 200 for empty approve body, got %d; body=%s", emptyRR.Code, emptyRR.Body.String())
	}

	created2, err := planStore.Create(plan.CreateInput{
		Title:     "plan-2",
		SessionID: "sess_2",
	})
	if err != nil {
		t.Fatalf("create second plan: %v", err)
	}
	trailingReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+created2.ID+"/approve", strings.NewReader(`{} {}`))
	trailingRR := httptest.NewRecorder()
	router.ServeHTTP(trailingRR, trailingReq)
	if trailingRR.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON in approve body, got %d; body=%s", trailingRR.Code, trailingRR.Body.String())
	}
}
