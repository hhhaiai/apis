package agentteam

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Agent represents a team member.
type Agent struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Role  string         `json:"role"` // lead, researcher, implementer, tester, reviewer
	Model string         `json:"model,omitempty"`
	Meta  map[string]any `json:"metadata,omitempty"`
}

// TaskStatus describes task progress.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskBlocked   TaskStatus = "blocked"
)

// Task represents a unit of work for the team.
type Task struct {
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description,omitempty"`
	AssignedTo  string         `json:"assigned_to,omitempty"` // agent ID
	DependsOn   []string       `json:"depends_on,omitempty"`  // task IDs
	Status      TaskStatus     `json:"status"`
	Result      string         `json:"result,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Meta        map[string]any `json:"metadata,omitempty"`
}

// Message represents inter-agent communication.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"` // agent ID or "*" for broadcast
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// TaskFunc is called to actually execute a task.
type TaskFunc func(ctx context.Context, agent Agent, task Task) (string, error)

// Team manages a group of agents working together.
type Team struct {
	mu        sync.RWMutex
	id        string
	name      string
	agents    map[string]Agent
	tasks     map[string]*Task
	taskOrder []string
	messages  []Message
	mailbox   map[string][]Message // agentID -> messages
	counter   uint64
	taskFn    TaskFunc
}

// NewTeam creates a new team.
func NewTeam(id, name string, taskFn TaskFunc) *Team {
	if taskFn == nil {
		taskFn = func(_ context.Context, _ Agent, t Task) (string, error) {
			return fmt.Sprintf("task %q completed (no executor configured)", t.Title), nil
		}
	}
	return &Team{
		id:      id,
		name:    name,
		agents:  make(map[string]Agent),
		tasks:   make(map[string]*Task),
		mailbox: make(map[string][]Message),
		taskFn:  taskFn,
	}
}

// AddAgent adds an agent to the team.
func (t *Team) AddAgent(a Agent) error {
	if a.ID == "" || a.Name == "" {
		return fmt.Errorf("agent requires id and name")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.agents[a.ID] = a
	if t.mailbox[a.ID] == nil {
		t.mailbox[a.ID] = []Message{}
	}
	return nil
}

// RemoveAgent removes an agent.
func (t *Team) RemoveAgent(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.agents, id)
}

// GetAgent returns an agent by ID.
func (t *Team) GetAgent(id string) (Agent, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	a, ok := t.agents[id]
	return a, ok
}

// ListAgents returns all agents.
func (t *Team) ListAgents() []Agent {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Agent, 0, len(t.agents))
	for _, a := range t.agents {
		out = append(out, a)
	}
	return out
}

// AddTask creates a task.
func (t *Team) AddTask(title, description, assignedTo string, dependsOn []string) (Task, error) {
	if title == "" {
		return Task{}, fmt.Errorf("task title is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	id := fmt.Sprintf("task_%d", atomic.AddUint64(&t.counter, 1))
	task := &Task{
		ID:          id,
		Title:       title,
		Description: description,
		AssignedTo:  assignedTo,
		DependsOn:   dependsOn,
		Status:      TaskPending,
		CreatedAt:   time.Now().UTC(),
	}
	t.tasks[id] = task
	t.taskOrder = append(t.taskOrder, id)
	return *task, nil
}

// GetTask retrieves a task.
func (t *Team) GetTask(id string) (Task, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	task, ok := t.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *task, true
}

// ListTasks returns all tasks.
func (t *Team) ListTasks() []Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Task, 0, len(t.taskOrder))
	for _, id := range t.taskOrder {
		if task, ok := t.tasks[id]; ok {
			out = append(out, *task)
		}
	}
	return out
}

// SendMessage sends a message between agents.
func (t *Team) SendMessage(from, to, content string) Message {
	t.mu.Lock()
	defer t.mu.Unlock()

	id := fmt.Sprintf("msg_%d", atomic.AddUint64(&t.counter, 1))
	msg := Message{
		ID:        id,
		From:      from,
		To:        to,
		Content:   content,
		Timestamp: time.Now().UTC(),
	}
	t.messages = append(t.messages, msg)

	if to == "*" {
		for aid := range t.agents {
			if aid != from {
				t.mailbox[aid] = append(t.mailbox[aid], msg)
			}
		}
	} else {
		t.mailbox[to] = append(t.mailbox[to], msg)
	}
	return msg
}

// ReadMailbox returns all messages for an agent.
func (t *Team) ReadMailbox(agentID string) []Message {
	t.mu.RLock()
	defer t.mu.RUnlock()
	msgs := t.mailbox[agentID]
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out
}

// Orchestrate executes tasks respecting dependency order.
func (t *Team) Orchestrate(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		task := t.nextReady()
		if task == nil {
			// Check if all done
			if t.allDone() {
				return nil
			}
			return fmt.Errorf("no tasks ready and not all completed (possible dependency cycle)")
		}

		t.mu.Lock()
		now := time.Now().UTC()
		task.Status = TaskRunning
		task.StartedAt = &now
		agent, hasAgent := t.agents[task.AssignedTo]
		t.mu.Unlock()

		if !hasAgent {
			// Pick first available agent
			agents := t.ListAgents()
			if len(agents) > 0 {
				agent = agents[0]
			}
		}

		result, err := t.taskFn(ctx, agent, *task)

		t.mu.Lock()
		end := time.Now().UTC()
		task.CompletedAt = &end
		if err != nil {
			task.Status = TaskFailed
			task.Result = fmt.Sprintf("error: %v", err)
		} else {
			task.Status = TaskCompleted
			task.Result = result
		}
		t.mu.Unlock()
	}
}

func (t *Team) nextReady() *Task {
	t.mu.RLock()
	defer t.mu.RUnlock()

	for _, id := range t.taskOrder {
		task := t.tasks[id]
		if task.Status != TaskPending {
			continue
		}
		// Check dependencies
		allMet := true
		for _, depID := range task.DependsOn {
			dep, ok := t.tasks[depID]
			if !ok || dep.Status != TaskCompleted {
				allMet = false
				break
			}
		}
		if allMet {
			return task
		}
	}
	return nil
}

func (t *Team) allDone() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, task := range t.tasks {
		if task.Status == TaskPending || task.Status == TaskRunning {
			return false
		}
	}
	return true
}
