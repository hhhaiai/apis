package statepersist

import (
	"errors"
	"sync"

	"ccgateway/internal/ccrun"
	"ccgateway/internal/plan"
	"ccgateway/internal/todo"
)

type RunStateStore interface {
	Snapshot() ccrun.StoreState
	Restore(state ccrun.StoreState) error
	SetOnChange(fn func())
}

type PlanStateStore interface {
	Snapshot() plan.StoreState
	Restore(state plan.StoreState) error
	SetOnChange(fn func())
}

type TodoStateStore interface {
	Snapshot() todo.StoreState
	Restore(state todo.StoreState) error
	SetOnChange(fn func())
}

type Manager struct {
	mu      sync.Mutex
	backend Backend
	runs    RunStateStore
	plans   PlanStateStore
	todos   TodoStateStore
	onError func(error)
}

func NewManager(backend Backend, runs RunStateStore, plans PlanStateStore, todos TodoStateStore) *Manager {
	return &Manager{
		backend: backend,
		runs:    runs,
		plans:   plans,
		todos:   todos,
	}
}

func (m *Manager) SetOnError(fn func(error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onError = fn
}

func (m *Manager) LoadAll() error {
	if m.backend == nil {
		return nil
	}
	if m.runs != nil {
		var state ccrun.StoreState
		if err := m.backend.Load("runs", &state); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		} else if err == nil {
			if err := m.runs.Restore(state); err != nil {
				return err
			}
		}
	}
	if m.plans != nil {
		var state plan.StoreState
		if err := m.backend.Load("plans", &state); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		} else if err == nil {
			if err := m.plans.Restore(state); err != nil {
				return err
			}
		}
	}
	if m.todos != nil {
		var state todo.StoreState
		if err := m.backend.Load("todos", &state); err != nil && !errors.Is(err, ErrNotFound) {
			return err
		} else if err == nil {
			if err := m.todos.Restore(state); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) SaveAll() error {
	if m.backend == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.runs != nil {
		if err := m.backend.Save("runs", m.runs.Snapshot()); err != nil {
			return err
		}
	}
	if m.plans != nil {
		if err := m.backend.Save("plans", m.plans.Snapshot()); err != nil {
			return err
		}
	}
	if m.todos != nil {
		if err := m.backend.Save("todos", m.todos.Snapshot()); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) BindAutoSave() {
	autoSave := func() {
		if err := m.SaveAll(); err != nil {
			m.dispatchError(err)
		}
	}
	if m.runs != nil {
		m.runs.SetOnChange(autoSave)
	}
	if m.plans != nil {
		m.plans.SetOnChange(autoSave)
	}
	if m.todos != nil {
		m.todos.SetOnChange(autoSave)
	}
}

func (m *Manager) dispatchError(err error) {
	m.mu.Lock()
	fn := m.onError
	m.mu.Unlock()
	if fn != nil && err != nil {
		fn(err)
	}
}
