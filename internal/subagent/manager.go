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
	ID                string     `json:"id"`
	ParentID          string     `json:"parent_id,omitempty"`
	Model             string     `json:"model"`
	Permissions       []string   `json:"permissions,omitempty"`
	Status            string     `json:"status"` // pending, running, completed, failed, terminated, deleted
	Task              string     `json:"task,omitempty"`
	Result            string     `json:"result,omitempty"`
	Error             string     `json:"error,omitempty"`
	TerminatedBy      string     `json:"terminated_by,omitempty"`
	TerminationReason string     `json:"termination_reason,omitempty"`
	TerminatedAt      *time.Time `json:"terminated_at,omitempty"`
	DeletedBy         string     `json:"deleted_by,omitempty"`
	DeletionReason    string     `json:"deletion_reason,omitempty"`
	DeletedAt         *time.Time `json:"deleted_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
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

// LifecycleEvent is emitted on subagent lifecycle transitions.
type LifecycleEvent struct {
	EventType  string    `json:"event_type"`
	Agent      Agent     `json:"agent"`
	RecordText string    `json:"record_text"`
	CreatedAt  time.Time `json:"created_at"`
}

// LifecycleHook handles lifecycle events.
type LifecycleHook func(event LifecycleEvent)

// Manager manages subagent lifecycle.
type Manager struct {
	mu            sync.RWMutex
	agents        map[string]Agent
	counter       uint64
	taskFn        TaskFunc // pluggable task executor
	lifecycleHook LifecycleHook
}

// NewManager creates a new subagent manager.
func NewManager(taskFn TaskFunc) *Manager {
	return NewManagerWithLifecycle(taskFn, nil)
}

// NewManagerWithLifecycle creates a new subagent manager with lifecycle hook.
func NewManagerWithLifecycle(taskFn TaskFunc, hook LifecycleHook) *Manager {
	if taskFn == nil {
		taskFn = defaultTaskFunc
	}
	return &Manager{
		agents:        make(map[string]Agent),
		taskFn:        taskFn,
		lifecycleHook: hook,
	}
}

// SetLifecycleHook updates lifecycle hook at runtime.
func (m *Manager) SetLifecycleHook(hook LifecycleHook) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lifecycleHook = hook
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
	m.emitLifecycle("subagent.created", agent)

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
	_, err := m.TerminateWithMeta(id, "", "")
	return err
}

// TerminateWithMeta stops a subagent and records operator metadata.
func (m *Manager) TerminateWithMeta(id, by, reason string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}

	m.mu.Lock()
	a, ok := m.agents[id]
	if !ok {
		m.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	if strings.EqualFold(strings.TrimSpace(a.Status), "deleted") {
		m.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q is deleted", id)
	}
	now := time.Now().UTC()
	a.Status = "terminated"
	a.TerminatedBy = strings.TrimSpace(by)
	a.TerminationReason = strings.TrimSpace(reason)
	a.TerminatedAt = &now
	a.UpdatedAt = now
	m.agents[id] = a
	m.mu.Unlock()
	m.emitLifecycle("subagent.terminated", a)
	return a, nil
}

// Delete performs a soft delete by marking subagent status as deleted.
func (m *Manager) Delete(id, by, reason string) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}

	m.mu.Lock()
	a, ok := m.agents[id]
	if !ok {
		m.mu.Unlock()
		return Agent{}, fmt.Errorf("agent %q not found", id)
	}
	now := time.Now().UTC()
	a.Status = "deleted"
	a.DeletedBy = strings.TrimSpace(by)
	a.DeletionReason = strings.TrimSpace(reason)
	a.DeletedAt = &now
	a.UpdatedAt = now
	m.agents[id] = a
	m.mu.Unlock()
	m.emitLifecycle("subagent.deleted", a)
	return a, nil
}

// Wait blocks until a subagent reaches a terminal state or context is canceled.
func (m *Manager) Wait(ctx context.Context, id string, pollInterval time.Duration) (Agent, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Agent{}, fmt.Errorf("agent id is required")
	}
	if pollInterval <= 0 {
		pollInterval = 20 * time.Millisecond
	}

	check := func() (Agent, bool, error) {
		a, ok := m.Get(id)
		if !ok {
			return Agent{}, false, fmt.Errorf("agent %q not found", id)
		}
		switch strings.TrimSpace(strings.ToLower(a.Status)) {
		case "completed":
			return a, true, nil
		case "failed":
			msg := strings.TrimSpace(a.Error)
			if msg == "" {
				msg = "subagent failed"
			}
			return Agent{}, true, fmt.Errorf(msg)
		case "terminated":
			msg := "subagent terminated"
			if reason := strings.TrimSpace(a.TerminationReason); reason != "" {
				msg += ": " + reason
			}
			return Agent{}, true, fmt.Errorf(msg)
		case "deleted":
			msg := "subagent deleted"
			if reason := strings.TrimSpace(a.DeletionReason); reason != "" {
				msg += ": " + reason
			}
			return Agent{}, true, fmt.Errorf(msg)
		default:
			return Agent{}, false, nil
		}
	}

	if out, done, err := check(); done {
		return out, err
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return Agent{}, ctx.Err()
		case <-ticker.C:
			out, done, err := check()
			if done {
				return out, err
			}
		}
	}
}

func (m *Manager) runAgent(ctx context.Context, id string) {
	if updated, ok := m.updateStatus(id, "running", "", ""); ok {
		m.emitLifecycle("subagent.running", updated)
	}

	agent, ok := m.Get(id)
	if !ok {
		return
	}

	result, err := m.taskFn(ctx, agent)
	if err != nil {
		if updated, ok := m.updateStatus(id, "failed", "", err.Error()); ok {
			m.emitLifecycle("subagent.failed", updated)
		}
		return
	}
	if updated, ok := m.updateStatus(id, "completed", result, ""); ok {
		m.emitLifecycle("subagent.completed", updated)
	}
}

func (m *Manager) updateStatus(id, status, result, errMsg string) (Agent, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, ok := m.agents[id]
	if !ok {
		return Agent{}, false
	}
	current := strings.ToLower(strings.TrimSpace(a.Status))
	next := strings.ToLower(strings.TrimSpace(status))
	if current == "terminated" && next != "terminated" {
		// Preserve explicit termination; background task completion must not revive it.
		return Agent{}, false
	}
	if current == "deleted" && next != "deleted" {
		// Preserve deleted status.
		return Agent{}, false
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
	return a, true
}

func (m *Manager) emitLifecycle(eventType string, agent Agent) {
	m.mu.RLock()
	hook := m.lifecycleHook
	m.mu.RUnlock()
	if hook == nil {
		return
	}
	hook(LifecycleEvent{
		EventType:  strings.TrimSpace(eventType),
		Agent:      agent,
		RecordText: lifecycleRecordText(eventType, agent),
		CreatedAt:  time.Now().UTC(),
	})
}

func lifecycleRecordText(eventType string, agent Agent) string {
	parts := []string{
		strings.TrimSpace(eventType),
		"subagent_id=" + strings.TrimSpace(agent.ID),
	}
	if parentID := strings.TrimSpace(agent.ParentID); parentID != "" {
		parts = append(parts, "parent_id="+parentID)
	}
	if model := strings.TrimSpace(agent.Model); model != "" {
		parts = append(parts, "model="+model)
	}
	switch strings.TrimSpace(eventType) {
	case "subagent.created":
		if task := truncateText(normalizeSpaces(agent.Task), 160); task != "" {
			parts = append(parts, fmt.Sprintf(`task="%s"`, task))
		}
	case "subagent.completed":
		if result := truncateText(normalizeSpaces(agent.Result), 220); result != "" {
			parts = append(parts, fmt.Sprintf(`result="%s"`, result))
		}
	case "subagent.failed":
		if errText := truncateText(normalizeSpaces(agent.Error), 200); errText != "" {
			parts = append(parts, fmt.Sprintf(`error="%s"`, errText))
		}
	case "subagent.terminated":
		if by := strings.TrimSpace(agent.TerminatedBy); by != "" {
			parts = append(parts, "by="+by)
		}
		if reason := truncateText(normalizeSpaces(agent.TerminationReason), 160); reason != "" {
			parts = append(parts, fmt.Sprintf(`reason="%s"`, reason))
		}
	case "subagent.deleted":
		if by := strings.TrimSpace(agent.DeletedBy); by != "" {
			parts = append(parts, "by="+by)
		}
		if reason := truncateText(normalizeSpaces(agent.DeletionReason), 160); reason != "" {
			parts = append(parts, fmt.Sprintf(`reason="%s"`, reason))
		}
	}
	return strings.Join(parts, " | ")
}

func defaultTaskFunc(_ context.Context, agent Agent) (string, error) {
	return fmt.Sprintf("Task '%s' completed by agent %s", agent.Task, agent.ID), nil
}

func normalizeSpaces(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	return strings.Join(strings.Fields(text), " ")
}

func truncateText(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return ""
	}
	rs := []rune(text)
	if len(rs) <= limit {
		return text
	}
	return strings.TrimSpace(string(rs[:limit])) + "..."
}
