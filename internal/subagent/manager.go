package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Agent represents a spawned subagent.
type Agent struct {
	ID          string    `json:"id"`
	ParentID    string    `json:"parent_id,omitempty"`
	Model       string    `json:"model"`
	Permissions []string  `json:"permissions,omitempty"`
	Status      string    `json:"status"` // pending, running, completed, failed
	Task        string    `json:"task,omitempty"`
	Result      string    `json:"result,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// SpawnConfig defines how to create a subagent.
type SpawnConfig struct {
	ParentID    string   `json:"parent_id,omitempty"`
	Model       string   `json:"model"`
	Task        string   `json:"task"`
	Permissions []string `json:"permissions,omitempty"`
}

// TaskFunc is a function that executes a subagent's work.
// It receives the agent config and returns a result string or error.
type TaskFunc func(ctx context.Context, agent Agent) (string, error)

// Manager manages subagent lifecycle.
type Manager struct {
	mu      sync.RWMutex
	agents  map[string]Agent
	counter uint64
	taskFn  TaskFunc // pluggable task executor
}

// NewManager creates a new subagent manager.
func NewManager(taskFn TaskFunc) *Manager {
	if taskFn == nil {
		taskFn = defaultTaskFunc
	}
	return &Manager{
		agents: make(map[string]Agent),
		taskFn: taskFn,
	}
}

// Spawn creates and starts a new subagent.
func (m *Manager) Spawn(ctx context.Context, cfg SpawnConfig) (Agent, error) {
	if strings.TrimSpace(cfg.Task) == "" {
		return Agent{}, fmt.Errorf("task is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = "default"
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("agent_%d_%x", now.Unix(), atomic.AddUint64(&m.counter, 1))

	agent := Agent{
		ID:          id,
		ParentID:    strings.TrimSpace(cfg.ParentID),
		Model:       cfg.Model,
		Permissions: cfg.Permissions,
		Status:      "pending",
		Task:        cfg.Task,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	m.mu.Lock()
	m.agents[id] = agent
	m.mu.Unlock()

	// Run the task asynchronously
	go m.runAgent(ctx, id)

	return agent, nil
}

// Get retrieves a subagent by ID.
func (m *Manager) Get(id string) (Agent, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.agents[strings.TrimSpace(id)]
	return a, ok
}

// List returns all subagents, optionally filtered by parent ID.
func (m *Manager) List(parentID string) []Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	parentID = strings.TrimSpace(parentID)
	out := make([]Agent, 0, len(m.agents))
	for _, a := range m.agents {
		if parentID != "" && a.ParentID != parentID {
			continue
		}
		out = append(out, a)
	}
	return out
}

// Terminate stops a subagent.
func (m *Manager) Terminate(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[id]
	if !ok {
		return fmt.Errorf("agent %q not found", id)
	}
	a.Status = "terminated"
	a.UpdatedAt = time.Now().UTC()
	m.agents[id] = a
	return nil
}

func (m *Manager) runAgent(ctx context.Context, id string) {
	m.updateStatus(id, "running", "", "")

	agent, ok := m.Get(id)
	if !ok {
		return
	}

	result, err := m.taskFn(ctx, agent)
	if err != nil {
		m.updateStatus(id, "failed", "", err.Error())
		return
	}
	m.updateStatus(id, "completed", result, "")
}

func (m *Manager) updateStatus(id, status, result, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[id]
	if !ok {
		return
	}
	a.Status = status
	if result != "" {
		a.Result = result
	}
	if errMsg != "" {
		a.Error = errMsg
	}
	a.UpdatedAt = time.Now().UTC()
	m.agents[id] = a
}

func defaultTaskFunc(_ context.Context, agent Agent) (string, error) {
	return fmt.Sprintf("Task '%s' completed by agent %s", agent.Task, agent.ID), nil
}
