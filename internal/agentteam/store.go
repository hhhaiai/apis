package agentteam

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type TeamInfo struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"created_at"`
	AgentCount int       `json:"agent_count"`
	TaskCount  int       `json:"task_count"`
}

type CreateInput struct {
	ID     string  `json:"id,omitempty"`
	Name   string  `json:"name"`
	Agents []Agent `json:"agents,omitempty"`
}

type CreateTaskInput struct {
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	AssignedTo  string   `json:"assigned_to,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

type Store struct {
	mu            sync.RWMutex
	teams         map[string]*teamRecord
	order         []string
	counter       uint64
	taskFn        TaskFunc
	taskEventHook TaskEventHook
}

type teamRecord struct {
	id        string
	name      string
	createdAt time.Time
	team      *Team
}

func NewStore(taskFn TaskFunc) *Store {
	return &Store{
		teams:  map[string]*teamRecord{},
		order:  []string{},
		taskFn: taskFn,
	}
}

func (s *Store) Create(in CreateInput) (TeamInfo, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return TeamInfo{}, fmt.Errorf("team name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = s.nextIDLocked()
	}
	if _, exists := s.teams[id]; exists {
		return TeamInfo{}, fmt.Errorf("team %q already exists", id)
	}

	team := NewTeam(id, name, s.taskFn)
	team.SetTaskEventHook(s.taskEventHook)
	for _, a := range in.Agents {
		if err := team.AddAgent(a); err != nil {
			return TeamInfo{}, err
		}
	}

	record := &teamRecord{
		id:        id,
		name:      name,
		createdAt: time.Now().UTC(),
		team:      team,
	}
	s.teams[id] = record
	s.order = append(s.order, id)
	return snapshotTeam(record), nil
}

func (s *Store) Get(teamID string) (TeamInfo, bool) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return TeamInfo{}, false
	}

	s.mu.RLock()
	record, ok := s.teams[teamID]
	s.mu.RUnlock()
	if !ok {
		return TeamInfo{}, false
	}
	return snapshotTeam(record), true
}

func (s *Store) List(limit int) []TeamInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.order) {
		limit = len(s.order)
	}
	out := make([]TeamInfo, 0, limit)
	for i := len(s.order) - 1; i >= 0 && len(out) < limit; i-- {
		id := s.order[i]
		if record, ok := s.teams[id]; ok {
			out = append(out, snapshotTeam(record))
		}
	}
	return out
}

func (s *Store) AddAgent(teamID string, a Agent) (Agent, error) {
	record, err := s.teamByID(teamID)
	if err != nil {
		return Agent{}, err
	}
	if err := record.team.AddAgent(a); err != nil {
		return Agent{}, err
	}
	out, ok := record.team.GetAgent(a.ID)
	if !ok {
		return Agent{}, fmt.Errorf("agent %q not found after add", strings.TrimSpace(a.ID))
	}
	return out, nil
}

func (s *Store) ListAgents(teamID string) ([]Agent, error) {
	record, err := s.teamByID(teamID)
	if err != nil {
		return nil, err
	}
	return record.team.ListAgents(), nil
}

func (s *Store) AddTask(teamID string, in CreateTaskInput) (Task, error) {
	record, err := s.teamByID(teamID)
	if err != nil {
		return Task{}, err
	}
	return record.team.AddTask(
		strings.TrimSpace(in.Title),
		strings.TrimSpace(in.Description),
		strings.TrimSpace(in.AssignedTo),
		cleanStrings(in.DependsOn),
	)
}

func (s *Store) ListTasks(teamID string) ([]Task, error) {
	record, err := s.teamByID(teamID)
	if err != nil {
		return nil, err
	}
	return record.team.ListTasks(), nil
}

func (s *Store) SendMessage(teamID, from, to, content string) (Message, error) {
	record, err := s.teamByID(teamID)
	if err != nil {
		return Message{}, err
	}
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	content = strings.TrimSpace(content)
	if from == "" {
		return Message{}, fmt.Errorf("from is required")
	}
	if to == "" {
		return Message{}, fmt.Errorf("to is required")
	}
	if content == "" {
		return Message{}, fmt.Errorf("content is required")
	}
	if from != "*" {
		if _, ok := record.team.GetAgent(from); !ok {
			return Message{}, fmt.Errorf("agent %q not found", from)
		}
	}
	if to != "*" {
		if _, ok := record.team.GetAgent(to); !ok {
			return Message{}, fmt.Errorf("agent %q not found", to)
		}
	}
	return record.team.SendMessage(from, to, content), nil
}

func (s *Store) ReadMailbox(teamID, agentID string) ([]Message, error) {
	record, err := s.teamByID(teamID)
	if err != nil {
		return nil, err
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	if _, ok := record.team.GetAgent(agentID); !ok {
		return nil, fmt.Errorf("agent %q not found", agentID)
	}
	return record.team.ReadMailbox(agentID), nil
}

func (s *Store) Orchestrate(ctx context.Context, teamID string) error {
	record, err := s.teamByID(teamID)
	if err != nil {
		return err
	}
	return record.team.Orchestrate(ctx)
}

// SetTaskEventHook sets lifecycle hook for existing and future teams.
func (s *Store) SetTaskEventHook(hook TaskEventHook) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taskEventHook = hook
	for _, record := range s.teams {
		if record == nil || record.team == nil {
			continue
		}
		record.team.SetTaskEventHook(hook)
	}
}

func (s *Store) teamByID(teamID string) (*teamRecord, error) {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil, fmt.Errorf("team id is required")
	}

	s.mu.RLock()
	record, ok := s.teams[teamID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("team %q not found", teamID)
	}
	return record, nil
}

func snapshotTeam(record *teamRecord) TeamInfo {
	if record == nil || record.team == nil {
		return TeamInfo{}
	}
	return TeamInfo{
		ID:         record.id,
		Name:       record.name,
		CreatedAt:  record.createdAt,
		AgentCount: len(record.team.ListAgents()),
		TaskCount:  len(record.team.ListTasks()),
	}
}

func (s *Store) nextIDLocked() string {
	n := atomic.AddUint64(&s.counter, 1)
	return fmt.Sprintf("team_%d_%x", time.Now().Unix(), n)
}

func cleanStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}
