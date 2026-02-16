package gateway_test

import (
	. "ccgateway/internal/gateway"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ccgateway/internal/agentteam"
	"ccgateway/internal/ccevent"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/policy"
	"ccgateway/internal/subagent"
)

func TestCCSubagentsListGetAndFilter(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, a subagent.Agent) (string, error) {
		return "ok:" + a.Task, nil
	})
	a1, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_p1",
		Model:    "model-a",
		Task:     "task-a",
	})
	if err != nil {
		t.Fatalf("spawn a1: %v", err)
	}
	a2, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_p2",
		Model:    "model-b",
		Task:     "task-b",
	})
	if err != nil {
		t.Fatalf("spawn a2: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := manager.Wait(ctx, a1.ID, 5*time.Millisecond); err != nil {
		t.Fatalf("wait a1: %v", err)
	}
	if _, err := manager.Wait(ctx, a2.ID, 5*time.Millisecond); err != nil {
		t.Fatalf("wait a2: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		SubagentStore: manager,
	})

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents?parent_id=team_p1", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []subagent.Agent `json:"data"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list resp: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Data) != 1 || listResp.Data[0].ID != a1.ID {
		t.Fatalf("unexpected parent filter list resp: %+v", listResp)
	}

	filterReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents?status=completed&model=model-a", nil)
	filterRR := httptest.NewRecorder()
	router.ServeHTTP(filterRR, filterReq)
	if filterRR.Code != http.StatusOK {
		t.Fatalf("expected 200 filtered list, got %d; body=%s", filterRR.Code, filterRR.Body.String())
	}
	var filterResp struct {
		Data  []subagent.Agent `json:"data"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(filterRR.Body.Bytes(), &filterResp); err != nil {
		t.Fatalf("unmarshal filtered resp: %v", err)
	}
	if filterResp.Count != 1 || len(filterResp.Data) != 1 || filterResp.Data[0].ID != a1.ID {
		t.Fatalf("unexpected status/model filter resp: %+v", filterResp)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents/"+a1.ID, nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 get, got %d; body=%s", getRR.Code, getRR.Body.String())
	}
	var got subagent.Agent
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get resp: %v", err)
	}
	if got.ID != a1.ID {
		t.Fatalf("unexpected get id: %q", got.ID)
	}
}

func TestCCSubagentsTeamFilter(t *testing.T) {
	manager := subagent.NewManager(nil)
	teamStore := agentteam.NewStore(agentteam.NewSubagentTaskFunc(manager))
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		TeamStore:     teamStore,
		SubagentStore: manager,
	})

	createReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams", strings.NewReader(`{
		"id":"team_q",
		"name":"TeamQ",
		"agents":[{"id":"lead_q","name":"LeadQ","role":"lead","model":"gpt-team"}]
	}`))
	createRR := httptest.NewRecorder()
	router.ServeHTTP(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 create team, got %d; body=%s", createRR.Code, createRR.Body.String())
	}

	taskReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_q/tasks", strings.NewReader(`{"title":"sync with subagent","assigned_to":"lead_q"}`))
	taskRR := httptest.NewRecorder()
	router.ServeHTTP(taskRR, taskReq)
	if taskRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 create task, got %d; body=%s", taskRR.Code, taskRR.Body.String())
	}

	runReq := httptest.NewRequest(http.MethodPost, "/v1/cc/teams/team_q/orchestrate", nil)
	runRR := httptest.NewRecorder()
	router.ServeHTTP(runRR, runReq)
	if runRR.Code != http.StatusOK {
		t.Fatalf("expected 200 orchestrate, got %d; body=%s", runRR.Code, runRR.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents?team_id=team_q", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 team filter list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []subagent.Agent `json:"data"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal team list: %v", err)
	}
	if listResp.Count != 1 || len(listResp.Data) != 1 {
		t.Fatalf("expected one team subagent, got %+v", listResp)
	}
	if listResp.Data[0].ParentID != "team_q" {
		t.Fatalf("expected parent_id team_q, got %q", listResp.Data[0].ParentID)
	}
}

func TestCCSubagentsNotConfigured(t *testing.T) {
	router := newTestRouter(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCSubagentTerminate(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, _ subagent.Agent) (string, error) {
		time.Sleep(120 * time.Millisecond)
		return "done", nil
	})
	created, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_t",
		Model:    "model-x",
		Task:     "run long",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		SubagentStore: manager,
	})

	termReq := httptest.NewRequest(http.MethodPost, "/v1/cc/subagents/"+created.ID+"/terminate", strings.NewReader(`{"by":"lead","reason":"manual stop"}`))
	termRR := httptest.NewRecorder()
	router.ServeHTTP(termRR, termReq)
	if termRR.Code != http.StatusOK {
		t.Fatalf("expected 200 terminate, got %d; body=%s", termRR.Code, termRR.Body.String())
	}
	var terminated subagent.Agent
	if err := json.Unmarshal(termRR.Body.Bytes(), &terminated); err != nil {
		t.Fatalf("unmarshal terminated: %v", err)
	}
	if terminated.Status != "terminated" {
		t.Fatalf("expected terminated status, got %q", terminated.Status)
	}
	if terminated.TerminatedBy != "lead" {
		t.Fatalf("expected terminated_by lead, got %q", terminated.TerminatedBy)
	}
	if terminated.TerminationReason != "manual stop" {
		t.Fatalf("expected termination_reason manual stop, got %q", terminated.TerminationReason)
	}

	time.Sleep(180 * time.Millisecond)
	getReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents/"+created.ID, nil)
	getRR := httptest.NewRecorder()
	router.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("expected 200 get, got %d; body=%s", getRR.Code, getRR.Body.String())
	}
	var got subagent.Agent
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if got.Status != "terminated" {
		t.Fatalf("expected status remain terminated, got %q", got.Status)
	}
}

func TestCCSubagentDelete(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, _ subagent.Agent) (string, error) {
		time.Sleep(120 * time.Millisecond)
		return "done", nil
	})
	created, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_del",
		Model:    "model-x",
		Task:     "run long",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		SubagentStore: manager,
	})

	delReq := httptest.NewRequest(http.MethodDelete, "/v1/cc/subagents/"+created.ID, strings.NewReader(`{"by":"admin","reason":"cleanup"}`))
	delRR := httptest.NewRecorder()
	router.ServeHTTP(delRR, delReq)
	if delRR.Code != http.StatusOK {
		t.Fatalf("expected 200 delete, got %d; body=%s", delRR.Code, delRR.Body.String())
	}
	var deleted subagent.Agent
	if err := json.Unmarshal(delRR.Body.Bytes(), &deleted); err != nil {
		t.Fatalf("unmarshal deleted: %v", err)
	}
	if deleted.Status != "deleted" {
		t.Fatalf("expected deleted status, got %q", deleted.Status)
	}
	if deleted.DeletedBy != "admin" || deleted.DeletionReason != "cleanup" {
		t.Fatalf("unexpected delete audit: %+v", deleted)
	}

	// deleted records are hidden by default
	listReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents?parent_id=team_del", nil)
	listRR := httptest.NewRecorder()
	router.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("expected 200 list, got %d; body=%s", listRR.Code, listRR.Body.String())
	}
	var listResp struct {
		Data  []subagent.Agent `json:"data"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp.Count != 0 {
		t.Fatalf("expected default list hide deleted, got %+v", listResp)
	}

	// include_deleted=true should return deleted items
	listDeletedReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents?parent_id=team_del&include_deleted=true", nil)
	listDeletedRR := httptest.NewRecorder()
	router.ServeHTTP(listDeletedRR, listDeletedReq)
	if listDeletedRR.Code != http.StatusOK {
		t.Fatalf("expected 200 include_deleted list, got %d; body=%s", listDeletedRR.Code, listDeletedRR.Body.String())
	}
	var listDeletedResp struct {
		Data  []subagent.Agent `json:"data"`
		Count int              `json:"count"`
	}
	if err := json.Unmarshal(listDeletedRR.Body.Bytes(), &listDeletedResp); err != nil {
		t.Fatalf("unmarshal include_deleted list: %v", err)
	}
	if listDeletedResp.Count != 1 || len(listDeletedResp.Data) != 1 || listDeletedResp.Data[0].ID != created.ID {
		t.Fatalf("expected one deleted item, got %+v", listDeletedResp)
	}
}

func TestCCSubagentTerminateAndDeleteAllowEmptyBody(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, _ subagent.Agent) (string, error) {
		time.Sleep(120 * time.Millisecond)
		return "done", nil
	})
	terminateTarget, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_empty",
		Model:    "model-x",
		Task:     "task-terminate",
	})
	if err != nil {
		t.Fatalf("spawn terminate target: %v", err)
	}
	deleteTarget, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_empty",
		Model:    "model-x",
		Task:     "task-delete",
	})
	if err != nil {
		t.Fatalf("spawn delete target: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		SubagentStore: manager,
	})

	terminateReq := httptest.NewRequest(http.MethodPost, "/v1/cc/subagents/"+terminateTarget.ID+"/terminate", nil)
	terminateRR := httptest.NewRecorder()
	router.ServeHTTP(terminateRR, terminateReq)
	if terminateRR.Code != http.StatusOK {
		t.Fatalf("expected 200 terminate with empty body, got %d; body=%s", terminateRR.Code, terminateRR.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/cc/subagents/"+deleteTarget.ID, nil)
	deleteRR := httptest.NewRecorder()
	router.ServeHTTP(deleteRR, deleteReq)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("expected 200 delete with empty body, got %d; body=%s", deleteRR.Code, deleteRR.Body.String())
	}
}

func TestCCSubagentTerminateRejectTrailingJSON(t *testing.T) {
	manager := subagent.NewManager(func(_ context.Context, _ subagent.Agent) (string, error) {
		time.Sleep(120 * time.Millisecond)
		return "done", nil
	})
	created, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_bad_json",
		Model:    "model-x",
		Task:     "task",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		SubagentStore: manager,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/cc/subagents/"+created.ID+"/terminate", strings.NewReader(`{"by":"lead"} {}`))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d; body=%s", rr.Code, rr.Body.String())
	}
}

func TestCCSubagentTimeline(t *testing.T) {
	manager := subagent.NewManager(nil)
	created, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_timeline",
		Model:    "model-timeline",
		Task:     "timeline task",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	eventStore := ccevent.NewStore()
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType:  "subagent.created",
		SubagentID: created.ID,
	})
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType:  "subagent.terminated",
		SubagentID: created.ID,
	})
	_, _ = eventStore.Append(ccevent.AppendInput{
		EventType:  "subagent.deleted",
		SubagentID: "other_subagent",
	})

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		EventStore:    eventStore,
		SubagentStore: manager,
	})

	timelineReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents/"+created.ID+"/timeline?limit=1", nil)
	timelineRR := httptest.NewRecorder()
	router.ServeHTTP(timelineRR, timelineReq)
	if timelineRR.Code != http.StatusOK {
		t.Fatalf("expected 200 timeline, got %d; body=%s", timelineRR.Code, timelineRR.Body.String())
	}
	var timelineResp struct {
		SubagentID string          `json:"subagent_id"`
		Data       []ccevent.Event `json:"data"`
		Count      int             `json:"count"`
	}
	if err := json.Unmarshal(timelineRR.Body.Bytes(), &timelineResp); err != nil {
		t.Fatalf("unmarshal timeline: %v", err)
	}
	if timelineResp.SubagentID != created.ID {
		t.Fatalf("unexpected timeline subagent_id: %q", timelineResp.SubagentID)
	}
	if timelineResp.Count != 1 || len(timelineResp.Data) != 1 {
		t.Fatalf("unexpected timeline payload: %+v", timelineResp)
	}
	if timelineResp.Data[0].EventType != "subagent.terminated" {
		t.Fatalf("expected latest filtered event subagent.terminated, got %q", timelineResp.Data[0].EventType)
	}
	if timelineResp.Data[0].SubagentID != created.ID {
		t.Fatalf("unexpected timeline event subagent_id: %q", timelineResp.Data[0].SubagentID)
	}

	aliasReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents/"+created.ID+"/events?event_type=subagent.created", nil)
	aliasRR := httptest.NewRecorder()
	router.ServeHTTP(aliasRR, aliasReq)
	if aliasRR.Code != http.StatusOK {
		t.Fatalf("expected 200 alias events, got %d; body=%s", aliasRR.Code, aliasRR.Body.String())
	}
	var aliasResp struct {
		Data  []ccevent.Event `json:"data"`
		Count int             `json:"count"`
	}
	if err := json.Unmarshal(aliasRR.Body.Bytes(), &aliasResp); err != nil {
		t.Fatalf("unmarshal alias events: %v", err)
	}
	if aliasResp.Count != 1 || len(aliasResp.Data) != 1 || aliasResp.Data[0].EventType != "subagent.created" {
		t.Fatalf("unexpected alias events response: %+v", aliasResp)
	}
}

func TestCCSubagentStream(t *testing.T) {
	manager := subagent.NewManager(nil)
	created, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_stream",
		Model:    "model-stream",
		Task:     "stream task",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	eventStore := ccevent.NewStore()
	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		EventStore:    eventStore,
		SubagentStore: manager,
	})

	ctx, cancel := context.WithCancel(context.Background())
	streamReq := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents/"+created.ID+"/stream?event_type=subagent.terminated", nil).WithContext(ctx)
	streamRR := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(streamRR, streamReq)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "subagent.deleted", SubagentID: created.ID})
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "subagent.terminated", SubagentID: created.ID})
	_, _ = eventStore.Append(ccevent.AppendInput{EventType: "subagent.terminated", SubagentID: "other_subagent"})
	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatalf("stream handler did not stop after cancel")
	}

	if streamRR.Code != http.StatusOK {
		t.Fatalf("expected 200 stream, got %d; body=%s", streamRR.Code, streamRR.Body.String())
	}
	if ct := streamRR.Header().Get("content-type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream content type, got %q", ct)
	}
	body := streamRR.Body.String()
	if !strings.Contains(body, "event: subagent.terminated") {
		t.Fatalf("expected terminated SSE event, got body=%q", body)
	}
	if !strings.Contains(body, `"subagent_id":"`+created.ID+`"`) {
		t.Fatalf("expected created subagent id in stream body, got body=%q", body)
	}
	if strings.Contains(body, `"subagent_id":"other_subagent"`) {
		t.Fatalf("unexpected other subagent event in body=%q", body)
	}
	if strings.Contains(body, "subagent.deleted") {
		t.Fatalf("unexpected filtered event type in body=%q", body)
	}
}

func TestCCSubagentStreamWithoutEventStore(t *testing.T) {
	manager := subagent.NewManager(nil)
	created, err := manager.Spawn(context.Background(), subagent.SpawnConfig{
		ParentID: "team_stream_no_event",
		Model:    "model-stream",
		Task:     "stream task",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}

	router := newTestRouterWithDeps(t, Dependencies{
		Orchestrator:  orchestrator.NewSimpleService(),
		Policy:        policy.NewNoopEngine(),
		ModelMapper:   modelmap.NewIdentityMapper(),
		SubagentStore: manager,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/cc/subagents/"+created.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 stream without event store, got %d; body=%s", rr.Code, rr.Body.String())
	}
}
