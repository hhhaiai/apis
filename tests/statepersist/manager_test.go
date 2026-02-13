package statepersist_test

import (
	. "ccgateway/internal/statepersist"
	"testing"

	"ccgateway/internal/ccrun"
	"ccgateway/internal/plan"
	"ccgateway/internal/todo"
)

func TestManagerSaveLoadAll(t *testing.T) {
	backend, err := NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	runs := ccrun.NewStore()
	plans := plan.NewStore()
	todos := todo.NewStore()

	if _, err := runs.Create(ccrun.CreateInput{ID: "run_1", Path: "/v1/messages", SessionID: "sess_1"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := plans.Create(plan.CreateInput{ID: "plan_1", Title: "p", SessionID: "sess_1"}); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	if _, err := todos.Create(todo.CreateInput{ID: "todo_1", Title: "t", SessionID: "sess_1", PlanID: "plan_1"}); err != nil {
		t.Fatalf("create todo: %v", err)
	}

	manager := NewManager(backend, runs, plans, todos)
	if err := manager.SaveAll(); err != nil {
		t.Fatalf("save all: %v", err)
	}

	runs2 := ccrun.NewStore()
	plans2 := plan.NewStore()
	todos2 := todo.NewStore()
	manager2 := NewManager(backend, runs2, plans2, todos2)
	if err := manager2.LoadAll(); err != nil {
		t.Fatalf("load all: %v", err)
	}

	if list := runs2.List(ccrun.ListFilter{}); len(list) != 1 || list[0].ID != "run_1" {
		t.Fatalf("unexpected runs after load: %+v", list)
	}
	if list := plans2.List(plan.ListFilter{}); len(list) != 1 || list[0].ID != "plan_1" {
		t.Fatalf("unexpected plans after load: %+v", list)
	}
	if list := todos2.List(todo.ListFilter{}); len(list) != 1 || list[0].ID != "todo_1" {
		t.Fatalf("unexpected todos after load: %+v", list)
	}
}

func TestManagerAutoSaveOnChange(t *testing.T) {
	backend, err := NewFileBackend(t.TempDir())
	if err != nil {
		t.Fatalf("new backend: %v", err)
	}
	runs := ccrun.NewStore()
	plans := plan.NewStore()
	todos := todo.NewStore()
	manager := NewManager(backend, runs, plans, todos)
	manager.BindAutoSave()

	if _, err := plans.Create(plan.CreateInput{ID: "plan_2", Title: "persist-me"}); err != nil {
		t.Fatalf("create plan: %v", err)
	}

	plans2 := plan.NewStore()
	manager2 := NewManager(backend, nil, plans2, nil)
	if err := manager2.LoadAll(); err != nil {
		t.Fatalf("load all: %v", err)
	}
	got := plans2.List(plan.ListFilter{})
	if len(got) != 1 || got[0].ID != "plan_2" {
		t.Fatalf("unexpected loaded plans: %+v", got)
	}
}
