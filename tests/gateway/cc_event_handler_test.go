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
	"ccgateway/internal/session"
	"ccgateway/internal/todo"
)

func TestCCEventsList(t *testing.T) {
	eventStore := ccevent.NewStore()
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "run.created", SessionID: "sess_1", RunID: "run_1"})
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "todo.created", SessionID: "sess_1", TodoID: "todo_1"})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		EventStore:   eventStore,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cc/events?session_id=sess_1&event_type=todo.created", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data  []ccevent.Event `json:"data"`
		Count int             `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal events response: %v", err)
	}
	if resp.Count != 1 || len(resp.Data) != 1 {
		t.Fatalf("unexpected events payload: %+v", resp)
	}
	if resp.Data[0].EventType != "todo.created" {
		t.Fatalf("unexpected event type: %q", resp.Data[0].EventType)
	}
}

func TestCCEventsListBySubagentID(t *testing.T) {
	eventStore := ccevent.NewStore()
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "subagent.terminated", SubagentID: "sub_a"})
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "subagent.deleted", SubagentID: "sub_b"})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		EventStore:   eventStore,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cc/events?subagent_id=sub_a", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data  []ccevent.Event `json:"data"`
		Count int             `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal events response: %v", err)
	}
	if resp.Count != 1 || len(resp.Data) != 1 {
		t.Fatalf("unexpected events payload: %+v", resp)
	}
	if resp.Data[0].SubagentID != "sub_a" {
		t.Fatalf("unexpected subagent id: %q", resp.Data[0].SubagentID)
	}
}

func TestCCEventsListByTeamID(t *testing.T) {
	eventStore := ccevent.NewStore()
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "team.task.completed", TeamID: "team_a"})
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "team.task.failed", TeamID: "team_b"})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		EventStore:   eventStore,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cc/events?team_id=team_a", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data  []ccevent.Event `json:"data"`
		Count int             `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal events response: %v", err)
	}
	if resp.Count != 1 || len(resp.Data) != 1 {
		t.Fatalf("unexpected events payload: %+v", resp)
	}
	if resp.Data[0].TeamID != "team_a" {
		t.Fatalf("unexpected team id: %q", resp.Data[0].TeamID)
	}
}

func TestCCEventsIncludeRecordText(t *testing.T) {
	eventStore := ccevent.NewStore()
	sessionStore := session.NewStore()

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		SessionStore: sessionStore,
		EventStore:   eventStore,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/sessions", strings.NewReader(`{"title":"record text"}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 create session, got %d; body=%s", createRR.Code, createRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/events?event_type=session.created", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 events list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data []ccevent.Event `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal event list: %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatalf("expected non-empty session.created events")
	}
	recordText, _ := listResp.Data[0].Data["record_text"].(string)
	recordText = strings.TrimSpace(recordText)
	if recordText == "" {
		t.Fatalf("expected non-empty record_text in event data: %+v", listResp.Data[0].Data)
	}
	if !strings.Contains(recordText, "session.created") {
		t.Fatalf("expected record_text include event_type, got %q", recordText)
	}
}

func TestCCEventFlowForSessionTodoRun(t *testing.T) {
	eventStore := ccevent.NewStore()
	sessionStore := session.NewStore()
	todoStore := todo.NewStore()

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		SessionStore: sessionStore,
		TodoStore:    todoStore,
		EventStore:   eventStore,
	})

	createSession := httptest.NewRequest(http.MethodPost, "/v1/cc/sessions", strings.NewReader(`{"title":"workspace"}`))
	createSessionRR := httptest.NewRecorder()
	router.ServeHTTP(createSessionRR, createSession)
	if createSessionRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 session, got %d; body=%s", createSessionRR.Code, createSessionRR.Body.String())
	}
	var sess session.Session
	if err := json.Unmarshal(createSessionRR.Body.Bytes(), &sess); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}

	createTodo := httptest.NewRequest(http.MethodPost, "/v1/cc/todos", strings.NewReader(`{"title":"task","session_id":"`+sess.ID+`","status":"pending"}`))
	createTodoRR := httptest.NewRecorder()
	router.ServeHTTP(createTodoRR, createTodo)
	if createTodoRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 todo, got %d; body=%s", createTodoRR.Code, createTodoRR.Body.String())
	}
	var td todo.Todo
	if err := json.Unmarshal(createTodoRR.Body.Bytes(), &td); err != nil {
		t.Fatalf("unmarshal todo: %v", err)
	}

	updateTodo := httptest.NewRequest(http.MethodPut, "/v1/cc/todos/"+td.ID, strings.NewReader(`{"status":"completed"}`))
	updateTodoRR := httptest.NewRecorder()
	router.ServeHTTP(updateTodoRR, updateTodo)
	if updateTodoRR.Code != http.StatusOK {
		t.Fatalf("expected 200 todo update, got %d; body=%s", updateTodoRR.Code, updateTodoRR.Body.String())
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-test","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`))
	msgReq.Header.Set("anthropic-version", "2023-06-01")
	msgReq.Header.Set("x-cc-session-id", sess.ID)
	msgRR := httptest.NewRecorder()
	router.ServeHTTP(msgRR, msgReq)
	if msgRR.Code != http.StatusOK {
		t.Fatalf("expected 200 message, got %d; body=%s", msgRR.Code, msgRR.Body.String())
	}
	runID := strings.TrimSpace(msgRR.Header().Get("x-cc-run-id"))
	if runID == "" {
		t.Fatalf("expected x-cc-run-id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/events?session_id="+sess.ID, nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 events list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data []ccevent.Event `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal event list: %v", err)
	}
	if len(listResp.Data) < 5 {
		t.Fatalf("expected >=5 events, got %d", len(listResp.Data))
	}

	type seen struct {
		sessionCreated bool
		todoCreated    bool
		todoCompleted  bool
		runCreated     bool
		runCompleted   bool
		runRecordText  bool
		runOutputText  bool
	}
	var flags seen
	for _, ev := range listResp.Data {
		switch ev.EventType {
		case "session.created":
			flags.sessionCreated = true
		case "todo.created":
			flags.todoCreated = true
		case "todo.completed":
			flags.todoCompleted = true
		case "run.created":
			if ev.RunID == runID {
				flags.runCreated = true
			}
		case "run.completed":
			if ev.RunID == runID {
				flags.runCompleted = true
				if text, ok := ev.Data["record_text"].(string); ok && strings.TrimSpace(text) != "" {
					flags.runRecordText = true
				}
				if text, ok := ev.Data["output_text"].(string); ok && strings.TrimSpace(text) != "" {
					flags.runOutputText = true
				}
			}
		}
	}
	if !flags.sessionCreated || !flags.todoCreated || !flags.todoCompleted || !flags.runCreated || !flags.runCompleted {
		t.Fatalf("missing expected event flags: %+v", flags)
	}
	if !flags.runRecordText || !flags.runOutputText {
		t.Fatalf("expected run.completed include record_text and output_text, got %+v", flags)
	}
}

func TestCCEventsNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/cc/events", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCEventFlowForPlanTodoLink(t *testing.T) {
	eventStore := ccevent.NewStore()
	planStore := plan.NewStore()
	todoStore := todo.NewStore()

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		PlanStore:    planStore,
		TodoStore:    todoStore,
		EventStore:   eventStore,
	})

	createPlan := httptest.NewRequest(http.MethodPost, "/v1/cc/plans", strings.NewReader(`{"title":"p","session_id":"sess_plan","steps":[{"title":"s1"},{"title":"s2"}]}`))
	createPlanRR := httptest.NewRecorder()
	router.ServeHTTP(createPlanRR, createPlan)
	if createPlanRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 plan, got %d; body=%s", createPlanRR.Code, createPlanRR.Body.String())
	}
	var p plan.Plan
	if err := json.Unmarshal(createPlanRR.Body.Bytes(), &p); err != nil {
		t.Fatalf("unmarshal plan: %v", err)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+p.ID+"/approve", strings.NewReader(`{}`))
	approveRR := httptest.NewRecorder()
	router.ServeHTTP(approveRR, approveReq)
	if approveRR.Code != http.StatusOK {
		t.Fatalf("expected 200 approve, got %d; body=%s", approveRR.Code, approveRR.Body.String())
	}

	execReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+p.ID+"/execute", strings.NewReader(`{}`))
	execRR := httptest.NewRecorder()
	router.ServeHTTP(execRR, execReq)
	if execRR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute start, got %d; body=%s", execRR.Code, execRR.Body.String())
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/v1/cc/plans/"+p.ID+"/execute", strings.NewReader(`{"complete":true}`))
	completeRR := httptest.NewRecorder()
	router.ServeHTTP(completeRR, completeReq)
	if completeRR.Code != http.StatusOK {
		t.Fatalf("expected 200 execute complete, got %d; body=%s", completeRR.Code, completeRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/events?plan_id="+p.ID, nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 events by plan, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data []ccevent.Event `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal event list: %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatalf("expected non-empty events for plan")
	}

	var hasPlanCreated, hasPlanApproved, hasPlanExecuting, hasPlanCompleted bool
	var hasTodoCreated, hasTodoCompleted bool
	for _, ev := range listResp.Data {
		switch ev.EventType {
		case "plan.created":
			hasPlanCreated = true
		case "plan.approved":
			hasPlanApproved = true
		case "plan.executing":
			hasPlanExecuting = true
		case "plan.completed":
			hasPlanCompleted = true
		case "todo.created":
			hasTodoCreated = true
		case "todo.completed":
			hasTodoCompleted = true
		}
	}
	if !hasPlanCreated || !hasPlanApproved || !hasPlanExecuting || !hasPlanCompleted || !hasTodoCreated || !hasTodoCompleted {
		t.Fatalf("missing expected plan-link event set: created=%v approved=%v executing=%v completed=%v todo_created=%v todo_completed=%v",
			hasPlanCreated, hasPlanApproved, hasPlanExecuting, hasPlanCompleted, hasTodoCreated, hasTodoCompleted)
	}
}
