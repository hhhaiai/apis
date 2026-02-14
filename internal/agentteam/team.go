package agentteam

import (
	"context"
	"fmt"
	"strings"
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

// TaskEvent is emitted when task execution state changes.
type TaskEvent struct {
	EventType  string    `json:"event_type"`
	TeamID     string    `json:"team_id"`
	TeamName   string    `json:"team_name"`
	Task       Task      `json:"task"`
	Agent      Agent     `json:"agent"`
	RecordText string    `json:"record_text"`
	CreatedAt  time.Time `json:"created_at"`
}

// TaskEventHook handles task lifecycle events.
type TaskEventHook func(event TaskEvent)

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
	taskHook  TaskEventHook
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

// SetTaskEventHook sets task lifecycle callback.
func (t *Team) SetTaskEventHook(hook TaskEventHook) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.taskHook = hook
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
		runningSnapshot := *task
		t.mu.Unlock()

		if !hasAgent {
			// Pick first available agent
			agents := t.ListAgents()
			if len(agents) > 0 {
				agent = agents[0]
			}
		}
		t.emitTaskEvent("team.task.running", runningSnapshot, agent)

		execTask := *task
		execTask.Meta = copyMeta(execTask.Meta)
		if execTask.Meta == nil {
			execTask.Meta = map[string]any{}
		}
		execTask.Meta["team_id"] = t.id
		execTask.Meta["team_name"] = t.name

		result, err := t.taskFn(ctx, agent, execTask)

		t.mu.Lock()
		end := time.Now().UTC()
		task.CompletedAt = &end
		eventType := "team.task.completed"
		if err != nil {
			task.Status = TaskFailed
			task.Result = fmt.Sprintf("error: %v", err)
			eventType = "team.task.failed"
		} else {
			task.Status = TaskCompleted
			task.Result = result
		}
		doneSnapshot := *task
		t.mu.Unlock()
		t.emitTaskEvent(eventType, doneSnapshot, agent)
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

func copyMeta(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}

func (t *Team) emitTaskEvent(eventType string, task Task, agent Agent) {
	t.mu.RLock()
	hook := t.taskHook
	teamID := t.id
	teamName := t.name
	t.mu.RUnlock()
	if hook == nil {
		return
	}
	hook(TaskEvent{
		EventType:  strings.TrimSpace(eventType),
		TeamID:     strings.TrimSpace(teamID),
		TeamName:   strings.TrimSpace(teamName),
		Task:       task,
		Agent:      agent,
		RecordText: taskEventRecordText(eventType, teamID, task, agent),
		CreatedAt:  time.Now().UTC(),
	})
}

func taskEventRecordText(eventType, teamID string, task Task, agent Agent) string {
	parts := []string{
		strings.TrimSpace(eventType),
		"team_id=" + strings.TrimSpace(teamID),
		"task_id=" + strings.TrimSpace(task.ID),
	}
	if title := truncateText(normalizeSpaces(task.Title), 100); title != "" {
		parts = append(parts, fmt.Sprintf(`title="%s"`, title))
	}
	if status := strings.TrimSpace(string(task.Status)); status != "" {
		parts = append(parts, "status="+status)
	}
	if assignedTo := strings.TrimSpace(task.AssignedTo); assignedTo != "" {
		parts = append(parts, "assigned_to="+assignedTo)
	}
	if agentID := strings.TrimSpace(agent.ID); agentID != "" {
		parts = append(parts, "agent_id="+agentID)
	}
	if result := truncateText(normalizeSpaces(task.Result), 220); result != "" {
		parts = append(parts, fmt.Sprintf(`output="%s"`, result))
	}
	return strings.Join(parts, " | ")
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
