package gateway

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/plan"
	"ccgateway/internal/todo"
)

func (s *server) handleCCPlans(w http.ResponseWriter, r *http.Request) {
	if s.planStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "plan store is not configured")
		return
	}
	switch r.Method {
	case http.MethodPost:
		var req plan.CreateInput
		if err := decodeJSONBodyStrict(r, &req, false); err != nil {
			s.reportRequestDecodeIssue(r, err)
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
			return
		}
		out, err := s.planStore.Create(req)
		if err != nil {
			writePlanStoreError(w, err)
			return
		}
		s.appendEvent(ccevent.AppendInput{
			EventType: "plan.created",
			SessionID: out.SessionID,
			RunID:     out.RunID,
			PlanID:    out.ID,
			Data: map[string]any{
				"status": out.Status,
				"title":  out.Title,
			},
		})
		s.ensurePlanTodos(out)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(out)
	case http.MethodGet:
		limit, ok := parseNonNegativeInt(r.URL.Query().Get("limit"))
		if !ok {
			s.writeError(w, http.StatusBadRequest, "invalid_request_error", "limit must be an integer >= 0")
			return
		}
		filter := plan.ListFilter{
			Limit:     limit,
			Status:    r.URL.Query().Get("status"),
			SessionID: r.URL.Query().Get("session_id"),
			RunID:     r.URL.Query().Get("run_id"),
		}
		items := s.planStore.List(filter)
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  items,
			"count": len(items),
		})
	default:
		s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
	}
}

func (s *server) handleCCPlanByPath(w http.ResponseWriter, r *http.Request) {
	if s.planStore == nil {
		s.writeError(w, http.StatusNotImplemented, "api_error", "plan store is not configured")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/cc/plans/")
	path = strings.Trim(path, "/")
	if path == "" {
		s.writeError(w, http.StatusNotFound, "not_found_error", "plan endpoint not found")
		return
	}
	parts := strings.Split(path, "/")
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		out, ok := s.planStore.Get(parts[0])
		if !ok {
			s.writeError(w, http.StatusNotFound, "not_found_error", "plan not found")
			return
		}
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(out)
		return
	}
	if len(parts) == 2 && parts[1] == "approve" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCPlanApprove(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "execute" {
		if r.Method != http.MethodPost {
			s.writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
			return
		}
		s.handleCCPlanExecute(w, r, parts[0])
		return
	}
	s.writeError(w, http.StatusNotFound, "not_found_error", "plan endpoint not found")
}

func (s *server) handleCCPlanApprove(w http.ResponseWriter, r *http.Request, planID string) {
	var req plan.ApproveInput
	if err := decodeJSONBodyStrict(r, &req, true); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	out, err := s.planStore.Approve(planID, req)
	if err != nil {
		writePlanStoreError(w, err)
		return
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: "plan.approved",
		SessionID: out.SessionID,
		RunID:     out.RunID,
		PlanID:    out.ID,
		Data: map[string]any{
			"status": out.Status,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) handleCCPlanExecute(w http.ResponseWriter, r *http.Request, planID string) {
	var req plan.ExecuteInput
	if err := decodeJSONBodyStrict(r, &req, true); err != nil {
		s.reportRequestDecodeIssue(r, err)
		s.writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON body")
		return
	}
	out, err := s.planStore.Execute(planID, req)
	if err != nil {
		writePlanStoreError(w, err)
		return
	}
	out = s.syncPlanTodos(out, req)
	eventType := "plan.executing"
	if out.Status == plan.StatusCompleted {
		eventType = "plan.completed"
	} else if out.Status == plan.StatusFailed {
		eventType = "plan.failed"
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: eventType,
		SessionID: out.SessionID,
		RunID:     out.RunID,
		PlanID:    out.ID,
		Data: map[string]any{
			"status": out.Status,
		},
	})
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

func (s *server) ensurePlanTodos(p plan.Plan) {
	if s.todoStore == nil || len(p.Steps) == 0 {
		return
	}
	created := 0
	for i, step := range p.Steps {
		title := strings.TrimSpace(step.Title)
		if title == "" {
			continue
		}
		td, err := s.todoStore.Create(todo.CreateInput{
			SessionID:   p.SessionID,
			RunID:       p.RunID,
			PlanID:      p.ID,
			Title:       title,
			Description: strings.TrimSpace(step.Description),
			Status:      string(todo.StatusPending),
			Metadata: map[string]any{
				"source":     "plan_step",
				"step_index": i,
			},
		})
		if err != nil {
			continue
		}
		created++
		s.appendEvent(ccevent.AppendInput{
			EventType: "todo.created",
			SessionID: td.SessionID,
			RunID:     td.RunID,
			PlanID:    td.PlanID,
			TodoID:    td.ID,
			Data: map[string]any{
				"status":    td.Status,
				"title":     td.Title,
				"plan_sync": true,
			},
		})
	}
	if created > 0 {
		s.appendEvent(ccevent.AppendInput{
			EventType: "plan.todos_created",
			SessionID: p.SessionID,
			RunID:     p.RunID,
			PlanID:    p.ID,
			Data: map[string]any{
				"count": created,
			},
		})
	}
}

func (s *server) syncPlanTodos(p plan.Plan, req plan.ExecuteInput) plan.Plan {
	if s.todoStore == nil {
		return p
	}
	todos := s.planTodosOrdered(p.ID)
	if len(todos) == 0 {
		return p
	}

	if req.Complete {
		s.syncPlanTodosToStatus(p, todos, todo.StatusCompleted)
		return p
	}
	if req.Failed {
		s.syncPlanTodosToStatus(p, todos, todo.StatusBlocked)
		return p
	}
	if p.Status != plan.StatusExecuting {
		return p
	}

	completedID, startedID := s.advancePlanTodoStep(p, todos)
	refresh := s.planTodosOrdered(p.ID)
	pendingCount := countTodoStatus(refresh, todo.StatusPending)
	inProgressCount := countTodoStatus(refresh, todo.StatusInProgress)
	if completedID != "" || startedID != "" {
		s.appendEvent(ccevent.AppendInput{
			EventType: "plan.step_advanced",
			SessionID: p.SessionID,
			RunID:     p.RunID,
			PlanID:    p.ID,
			Data: map[string]any{
				"completed_todo_id":  completedID,
				"started_todo_id":    startedID,
				"remaining_pending":  pendingCount,
				"remaining_running":  inProgressCount,
				"total_linked_todos": len(refresh),
			},
		})
	}

	if pendingCount == 0 && inProgressCount == 0 {
		completed, err := s.planStore.Execute(p.ID, plan.ExecuteInput{Complete: true})
		if err == nil {
			p = completed
			s.appendEvent(ccevent.AppendInput{
				EventType: "plan.auto_completed",
				SessionID: p.SessionID,
				RunID:     p.RunID,
				PlanID:    p.ID,
				Data: map[string]any{
					"reason": "all_linked_todos_completed",
				},
			})
		}
	}
	return p
}

func (s *server) syncPlanTodosToStatus(p plan.Plan, todos []todo.Todo, target todo.Status) {
	updated := 0
	statusText := string(target)
	for _, td := range todos {
		if !shouldSyncTodoToTarget(td.Status, target) {
			continue
		}
		next, err := s.todoStore.Update(td.ID, todo.UpdateInput{Status: &statusText})
		if err != nil {
			continue
		}
		updated++
		s.emitPlanSyncedTodoEvent(next)
	}
	if updated > 0 {
		s.appendEvent(ccevent.AppendInput{
			EventType: "plan.todos_synced",
			SessionID: p.SessionID,
			RunID:     p.RunID,
			PlanID:    p.ID,
			Data: map[string]any{
				"plan_status": p.Status,
				"count":       updated,
				"target":      target,
			},
		})
	}
}

func shouldSyncTodoToTarget(current, target todo.Status) bool {
	switch target {
	case todo.StatusCompleted:
		return current != todo.StatusCompleted && current != todo.StatusCanceled
	case todo.StatusBlocked:
		return current == todo.StatusPending || current == todo.StatusInProgress
	default:
		return false
	}
}

func (s *server) advancePlanTodoStep(p plan.Plan, todos []todo.Todo) (string, string) {
	var completedID string
	var startedID string

	inProgressIdx := -1
	for i := range todos {
		if todos[i].Status == todo.StatusInProgress {
			inProgressIdx = i
			break
		}
	}
	if inProgressIdx >= 0 {
		statusText := string(todo.StatusCompleted)
		next, err := s.todoStore.Update(todos[inProgressIdx].ID, todo.UpdateInput{Status: &statusText})
		if err == nil {
			completedID = next.ID
			s.emitPlanSyncedTodoEvent(next)
		}
	}

	startAt := 0
	if inProgressIdx >= 0 {
		startAt = inProgressIdx + 1
	}
	for i := startAt; i < len(todos); i++ {
		if todos[i].Status != todo.StatusPending {
			continue
		}
		statusText := string(todo.StatusInProgress)
		next, err := s.todoStore.Update(todos[i].ID, todo.UpdateInput{Status: &statusText})
		if err != nil {
			continue
		}
		startedID = next.ID
		s.emitPlanSyncedTodoEvent(next)
		break
	}
	if startedID == "" {
		for i := 0; i < startAt; i++ {
			if todos[i].Status != todo.StatusPending {
				continue
			}
			statusText := string(todo.StatusInProgress)
			next, err := s.todoStore.Update(todos[i].ID, todo.UpdateInput{Status: &statusText})
			if err != nil {
				continue
			}
			startedID = next.ID
			s.emitPlanSyncedTodoEvent(next)
			break
		}
	}

	if completedID != "" || startedID != "" {
		s.appendEvent(ccevent.AppendInput{
			EventType: "plan.todos_synced",
			SessionID: p.SessionID,
			RunID:     p.RunID,
			PlanID:    p.ID,
			Data: map[string]any{
				"plan_status": p.Status,
				"count":       boolCount(completedID != "") + boolCount(startedID != ""),
				"target":      "step_progress",
			},
		})
	}
	return completedID, startedID
}

func (s *server) emitPlanSyncedTodoEvent(next todo.Todo) {
	eventType := "todo.updated"
	if next.Status == todo.StatusCompleted {
		eventType = "todo.completed"
	}
	s.appendEvent(ccevent.AppendInput{
		EventType: eventType,
		SessionID: next.SessionID,
		RunID:     next.RunID,
		PlanID:    next.PlanID,
		TodoID:    next.ID,
		Data: map[string]any{
			"status":    next.Status,
			"plan_sync": true,
		},
	})
}

func boolCount(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *server) planTodosOrdered(planID string) []todo.Todo {
	items := s.todoStore.List(todo.ListFilter{PlanID: planID})
	if len(items) <= 1 {
		return items
	}
	sort.SliceStable(items, func(i, j int) bool {
		iStep, iOK := todoStepIndex(items[i])
		jStep, jOK := todoStepIndex(items[j])
		switch {
		case iOK && jOK && iStep != jStep:
			return iStep < jStep
		case iOK && !jOK:
			return true
		case !iOK && jOK:
			return false
		default:
			return items[i].CreatedAt.Before(items[j].CreatedAt)
		}
	})
	return items
}

func countTodoStatus(items []todo.Todo, target todo.Status) int {
	n := 0
	for _, td := range items {
		if td.Status == target {
			n++
		}
	}
	return n
}

func todoStepIndex(td todo.Todo) (int, bool) {
	if len(td.Metadata) == 0 {
		return 0, false
	}
	raw, ok := td.Metadata["step_index"]
	if !ok {
		return 0, false
	}
	switch v := raw.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func writePlanStoreError(w http.ResponseWriter, err error) {
	msg := strings.TrimSpace(err.Error())
	switch {
	case strings.Contains(msg, "not found"):
		writeErrorEnvelope(w, http.StatusNotFound, "not_found_error", msg)
	case strings.Contains(msg, "already exists"):
		writeErrorEnvelope(w, http.StatusConflict, "invalid_request_error", msg)
	default:
		writeErrorEnvelope(w, http.StatusBadRequest, "invalid_request_error", msg)
	}
}
