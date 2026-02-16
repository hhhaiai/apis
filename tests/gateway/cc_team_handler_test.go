package gateway_test

import (
	. "ccgateway/internal/gateway"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ccgateway/internal/agentteam"
	"ccgateway/internal/ccevent"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
)

func TestCCTeamsCreateAndCoordinate(t *testing.T) {
	teamStore := agentteam.NewStore(func(_ context.Context, a agentteam.Agent, task agentteam.Task) (string, error) {
		return fmt.Sprintf("done:%s:%s", a.ID, task.ID), nil
	})
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TeamStore:    teamStore,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams", strings.NewReader(`{
		"id":"team_alpha",
		"name":"Alpha",
		"agents":[
			{"id":"lead","name":"Lead","role":"lead"},
			{"id":"dev","name":"Developer","role":"implementer"}
		]
	}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body=%s", createRR.Code, createRR.Body.String())
	}
	var created agentteam.TeamInfo
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created team: %v", err)
	}
	if created.ID != "team_alpha" || created.AgentCount != 2 {
		t.Fatalf("unexpected created team payload: %+v", created)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/teams/team_alpha", nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 get team, got %d; body=%s", getRR.Code, getRR.Body.String())
	}

	addAgentReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_alpha/agents", strings.NewReader(`{"id":"qa","name":"QA","role":"tester"}`))
	addAgentRR := httptest.NewRecorder()
	router.ServeHTTP(addAgentRR, addAgentReq)
	if addAgentRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 add agent, got %d; body=%s", addAgentRR.Code, addAgentRR.Body.String())
	}

	task1Req := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_alpha/tasks", strings.NewReader(`{"title":"Design split","assigned_to":"lead"}`))
	task1RR := httptest.NewRecorder()
	router.ServeHTTP(task1RR, task1Req)
	if task1RR.Code != http.StatusCreated {
		t.Fatalf("expected 201 task1, got %d; body=%s", task1RR.Code, task1RR.Body.String())
	}
	var task1 agentteam.Task
	if err := json.Unmarshal(task1RR.Body.Bytes(), &task1); err != nil {
		t.Fatalf("unmarshal task1: %v", err)
	}

	task2Body := fmt.Sprintf(`{"title":"Implement API","assigned_to":"dev","depends_on":["%s"]}`, task1.ID)
	task2Req := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_alpha/tasks", strings.NewReader(task2Body))
	task2RR := httptest.NewRecorder()
	router.ServeHTTP(task2RR, task2Req)
	if task2RR.Code != http.StatusCreated {
		t.Fatalf("expected 201 task2, got %d; body=%s", task2RR.Code, task2RR.Body.String())
	}

	listTeamsReq := httptest.NewRequest(http.MethodGet, "/v1/cc/teams?limit=1", nil)
	listTeamsRR := httptest.NewRecorder()
	router.ServeHTTP(listTeamsRR, listTeamsReq)
	if listTeamsRR.Code != http.StatusOK {
		t.Fatalf("expected 200 list teams, got %d; body=%s", listTeamsRR.Code, listTeamsRR.Body.String())
	}
	var teamsResp struct {
		Data  []agentteam.TeamInfo `json:"data"`
		Count int                  `json:"count"`
	}
	if err := json.Unmarshal(listTeamsRR.Body.Bytes(), &teamsResp); err != nil {
		t.Fatalf("unmarshal list teams: %v", err)
	}
	if teamsResp.Count != 1 || len(teamsResp.Data) != 1 || teamsResp.Data[0].ID != "team_alpha" {
		t.Fatalf("unexpected list teams payload: %+v", teamsResp)
	}

	listTasksReq := httptest.NewRequest(http.MethodGet, "/v1/cc/teams/team_alpha/tasks", nil)
	listTasksRR := httptest.NewRecorder()
	router.ServeHTTP(listTasksRR, listTasksReq)
	if listTasksRR.Code != http.StatusOK {
		t.Fatalf("expected 200 list tasks, got %d; body=%s", listTasksRR.Code, listTasksRR.Body.String())
	}
	var tasksResp struct {
		Data  []agentteam.Task `json:"data"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(listTasksRR.Body.Bytes(), &tasksResp); err != nil {
		t.Fatalf("unmarshal list tasks: %v", err)
	}
	if tasksResp.Count != 2 || len(tasksResp.Data) != 2 {
		t.Fatalf("unexpected task list payload: %+v", tasksResp)
	}

	sendMsgReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_alpha/messages", strings.NewReader(`{"from":"lead","to":"dev","content":"start implementation"}`))
	sendMsgRR := httptest.NewRecorder()
	router.ServeHTTP(sendMsgRR, sendMsgReq)
	if sendMsgRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 send message, got %d; body=%s", sendMsgRR.Code, sendMsgRR.Body.String())
	}

	readMailboxReq := httptest.NewRequest(http.MethodGet, "/v1/cc/teams/team_alpha/messages?agent_id=dev", nil)
	readMailboxRR := httptest.NewRecorder()
	router.ServeHTTP(readMailboxRR, readMailboxReq)
	if readMailboxRR.Code != http.StatusOK {
		t.Fatalf("expected 200 read mailbox, got %d; body=%s", readMailboxRR.Code, readMailboxRR.Body.String())
	}
	var mailboxResp struct {
		AgentID string              `json:"agent_id"`
		Data    []agentteam.Message `json:"data"`
		Count   int                 `json:"count"`
	}
	if err := json.Unmarshal(readMailboxRR.Body.Bytes(), &mailboxResp); err != nil {
		t.Fatalf("unmarshal mailbox: %v", err)
	}
	if mailboxResp.AgentID != "dev" || mailboxResp.Count != 1 || len(mailboxResp.Data) != 1 {
		t.Fatalf("unexpected mailbox payload: %+v", mailboxResp)
	}

	orchestrateReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_alpha/orchestrate", nil)
	orchestrateRR := httptest.NewRecorder()
	router.ServeHTTP(orchestrateRR, orchestrateReq)
	if orchestrateRR.Code != http.StatusOK {
		t.Fatalf("expected 200 orchestrate, got %d; body=%s", orchestrateRR.Code, orchestrateRR.Body.String())
	}
	var orchestrateResp struct {
		Team  agentteam.TeamInfo `json:"team"`
		Tasks []agentteam.Task   `json:"tasks"`
		Count int                `json:"count"`
	}
	if err := json.Unmarshal(orchestrateRR.Body.Bytes(), &orchestrateResp); err != nil {
		t.Fatalf("unmarshal orchestrate response: %v", err)
	}
	if orchestrateResp.Team.ID != "team_alpha" || orchestrateResp.Count != 2 || len(orchestrateResp.Tasks) != 2 {
		t.Fatalf("unexpected orchestrate payload: %+v", orchestrateResp)
	}
	for _, item := range orchestrateResp.Tasks {
		if item.Status != agentteam.TaskCompleted {
			t.Fatalf("expected completed task status, got %q for task %s", item.Status, item.ID)
		}
	}
}

func TestCCTeamsNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/v1/cc/teams", strings.NewReader(`{"name":"x"}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCTeamMessagesRequireAgentID(t *testing.T) {
	teamStore := agentteam.NewStore(nil)
	_, err := teamStore.Create(agentteam.CreateInput{
		ID:   "team_beta",
		Name: "Beta",
		Agents: []agentteam.Agent{
			{ID: "a1", Name: "A1", Role: "lead"},
		},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TeamStore:    teamStore,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cc/teams/team_beta/messages", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCTeamTaskLifecycleEventsForSync(t *testing.T) {
	eventStore := ccevent.NewStore()
	teamStore := agentteam.NewStore(func(_ context.Context, _ agentteam.Agent, task agentteam.Task) (string, error) {
		return "sync:" + task.ID, nil
	})
	teamStore.SetTaskEventHook(func(event agentteam.TaskEvent) {
		_, _ = eventStore.Append(ccevent.AppendInput{
			EventType: event.EventType,
			Data: map[string]any{
				"team_id":     event.TeamID,
				"task_id":     event.Task.ID,
				"status":      event.Task.Status,
				"output_text": event.Task.Result,
				"record_text": event.RecordText,
			},
		})
	})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TeamStore:    teamStore,
		EventStore:   eventStore,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams", strings.NewReader(`{
		"id":"team_sync",
		"name":"SyncTeam",
		"agents":[{"id":"lead_sync","name":"LeadSync","role":"lead"}]
	}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 create, got %d; body=%s", createRR.Code, createRR.Body.String())
	}

	taskReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_sync/tasks", strings.NewReader(`{"title":"sync-task","assigned_to":"lead_sync"}`))
	taskRR := httptest.NewRecorder()
	router.ServeHTTP(taskRR, taskReq)
	if taskRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 task, got %d; body=%s", taskRR.Code, taskRR.Body.String())
	}

	orchestrateReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_sync/orchestrate", nil)
	orchestrateRR := httptest.NewRecorder()
	router.ServeHTTP(orchestrateRR, orchestrateReq)
	if orchestrateRR.Code != http.StatusOK {
		t.Fatalf("expected 200 orchestrate, got %d; body=%s", orchestrateRR.Code, orchestrateRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/events?event_type=team.task.completed", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 events list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data []ccevent.Event `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal events list: %v", err)
	}
	if len(listResp.Data) == 0 {
		t.Fatalf("expected completed lifecycle events")
	}
	found := false
	for _, ev := range listResp.Data {
		if ev.EventType != "team.task.completed" {
			continue
		}
		if strings.TrimSpace(fmt.Sprint(ev.Data["team_id"])) != "team_sync" {
			continue
		}
		recordText := strings.TrimSpace(fmt.Sprint(ev.Data["record_text"]))
		if recordText == "" {
			t.Fatalf("expected non-empty record_text in %+v", ev.Data)
		}
		if !strings.Contains(recordText, "team.task.completed") {
			t.Fatalf("unexpected record_text: %q", recordText)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("expected completed lifecycle event for team_sync, got %+v", listResp.Data)
	}
}

func TestCCTeamsRejectUnknownFieldsOnCreate(t *testing.T) {
	teamStore := agentteam.NewStore(nil)
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator: orchestrator.NewSimpleService(),
		Policy:       policy.NewNoopEngine(),
		ModelMapper:  modelmap.NewIdentityMapper(),
		TeamStore:    teamStore,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/teams", strings.NewReader(`{"id":"team_x","name":"X","unknown_field":1}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown field, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
