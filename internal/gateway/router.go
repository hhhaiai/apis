package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"ccgateway/internal/agentteam"
	"ccgateway/internal/ccevent"
	"ccgateway/internal/ccrun"
	"ccgateway/internal/eval"
	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/modelmap"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/plan"
	"ccgateway/internal/plugin"
	"ccgateway/internal/policy"
	"ccgateway/internal/runlog"
	"ccgateway/internal/session"
	"ccgateway/internal/settings"
	"ccgateway/internal/subagent"
	"ccgateway/internal/todo"
	"ccgateway/internal/toolcatalog"
	"ccgateway/internal/toolruntime"
)

type Dependencies struct {
	Orchestrator    orchestrator.Service
	Policy          policy.Engine
	ModelMapper     modelmap.Mapper
	Settings        *settings.Store
	ToolCatalog     *toolcatalog.Catalog
	ToolExecutor    toolruntime.Executor
	SessionStore    SessionStore
	RunStore        RunStore
	TodoStore       TodoStore
	PlanStore       PlanStore
	EventStore      EventStore
	TeamStore       TeamStore
	SubagentStore   SubagentStore
	MCPRegistry     MCPRegistry
	PluginStore     PluginStore
	SkillEngine     SkillEngine
	CostTracker     CostTracker
	Evaluator       *eval.Evaluator
	SchedulerStatus StatusProvider
	ProbeStatus     StatusProvider
	AdminToken      string
	RunLogger       runlog.Logger
}

type StatusProvider interface {
	Snapshot() map[string]any
}

type SessionStore interface {
	Create(in session.CreateInput) (session.Session, error)
	Fork(parentID string, in session.CreateInput) (session.Session, error)
	Get(id string) (session.Session, bool)
	List(limit int) []session.Session
	AppendMessage(sessionID string, msg session.SessionMessage) error
	GetMessages(sessionID string) ([]session.SessionMessage, error)
}

type TodoStore interface {
	Create(in todo.CreateInput) (todo.Todo, error)
	Get(id string) (todo.Todo, bool)
	Update(id string, in todo.UpdateInput) (todo.Todo, error)
	List(filter todo.ListFilter) []todo.Todo
}

type RunStore interface {
	Create(in ccrun.CreateInput) (ccrun.Run, error)
	Get(id string) (ccrun.Run, bool)
	List(filter ccrun.ListFilter) []ccrun.Run
	Complete(id string, in ccrun.CompleteInput) (ccrun.Run, error)
}

type PlanStore interface {
	Create(in plan.CreateInput) (plan.Plan, error)
	Get(id string) (plan.Plan, bool)
	List(filter plan.ListFilter) []plan.Plan
	Approve(id string, in plan.ApproveInput) (plan.Plan, error)
	Execute(id string, in plan.ExecuteInput) (plan.Plan, error)
}

type EventStore interface {
	Append(in ccevent.AppendInput) (ccevent.Event, error)
	List(filter ccevent.ListFilter) []ccevent.Event
	Subscribe(filter ccevent.ListFilter) (<-chan ccevent.Event, func())
}

type TeamStore interface {
	Create(in agentteam.CreateInput) (agentteam.TeamInfo, error)
	Get(teamID string) (agentteam.TeamInfo, bool)
	List(limit int) []agentteam.TeamInfo
	AddAgent(teamID string, in agentteam.Agent) (agentteam.Agent, error)
	ListAgents(teamID string) ([]agentteam.Agent, error)
	AddTask(teamID string, in agentteam.CreateTaskInput) (agentteam.Task, error)
	ListTasks(teamID string) ([]agentteam.Task, error)
	SendMessage(teamID, from, to, content string) (agentteam.Message, error)
	ReadMailbox(teamID, agentID string) ([]agentteam.Message, error)
	Orchestrate(ctx context.Context, teamID string) error
}

type SubagentStore interface {
	Get(id string) (subagent.Agent, bool)
	List(parentID string) []subagent.Agent
	Terminate(id string) error
	TerminateWithMeta(id, by, reason string) (subagent.Agent, error)
	Delete(id, by, reason string) (subagent.Agent, error)
}

type MCPRegistry interface {
	Register(in mcpregistry.RegisterInput) (mcpregistry.Server, error)
	Update(id string, in mcpregistry.UpdateInput) (mcpregistry.Server, error)
	Delete(id string) error
	Get(id string) (mcpregistry.Server, bool)
	List(limit int) []mcpregistry.Server
	CheckHealth(ctx context.Context, id string) (mcpregistry.Server, error)
	Reconnect(ctx context.Context, id string) (mcpregistry.Server, error)
	ListTools(ctx context.Context, id string) ([]mcpregistry.Tool, error)
	CallTool(ctx context.Context, id, name string, input map[string]any) (mcpregistry.ToolCallResult, error)
	CallToolAny(ctx context.Context, name string, input map[string]any) (mcpregistry.ToolCallResult, error)
}

type PluginStore interface {
	Install(p plugin.Plugin) error
	Uninstall(name string) error
	Get(name string) (plugin.Plugin, bool)
	List() []plugin.Plugin
	Enable(name string) error
	Disable(name string) error
}

// CostTracker tracks per-model, per-session costs with optional budget.
type CostTracker interface {
	Snapshot() map[string]any
}

type server struct {
	orchestrator    orchestrator.Service
	policy          policy.Engine
	modelMapper     modelmap.Mapper
	settings        *settings.Store
	toolCatalog     *toolcatalog.Catalog
	toolExecutor    toolruntime.Executor
	sessionStore    SessionStore
	runStore        RunStore
	todoStore       TodoStore
	planStore       PlanStore
	eventStore      EventStore
	teamStore       TeamStore
	subagentStore   SubagentStore
	mcpRegistry     MCPRegistry
	pluginStore     PluginStore
	skillEngine     SkillEngine
	costTracker     CostTracker
	evaluator       *eval.Evaluator
	schedulerStatus StatusProvider
	probeStatus     StatusProvider
	adminToken      string
	runLogger       runlog.Logger
	idCounter       uint64
}

func NewRouter(deps Dependencies) http.Handler {
	if deps.Orchestrator == nil {
		panic("orchestrator dependency is required")
	}
	if deps.Policy == nil {
		panic("policy dependency is required")
	}
	if deps.ModelMapper == nil {
		deps.ModelMapper = modelmap.NewIdentityMapper()
	}
	if deps.ToolExecutor == nil {
		deps.ToolExecutor = newMCPAwareExecutor(toolruntime.NewDefaultExecutor(), deps.MCPRegistry)
	}

	s := &server{
		orchestrator:    deps.Orchestrator,
		policy:          deps.Policy,
		modelMapper:     deps.ModelMapper,
		settings:        deps.Settings,
		toolCatalog:     deps.ToolCatalog,
		toolExecutor:    deps.ToolExecutor,
		sessionStore:    deps.SessionStore,
		runStore:        deps.RunStore,
		todoStore:       deps.TodoStore,
		planStore:       deps.PlanStore,
		eventStore:      deps.EventStore,
		teamStore:       deps.TeamStore,
		subagentStore:   deps.SubagentStore,
		mcpRegistry:     deps.MCPRegistry,
		pluginStore:     deps.PluginStore,
		skillEngine:     deps.SkillEngine,
		costTracker:     deps.CostTracker,
		evaluator:       deps.Evaluator,
		schedulerStatus: deps.SchedulerStatus,
		probeStatus:     deps.ProbeStatus,
		adminToken:      strings.TrimSpace(deps.AdminToken),
		runLogger:       deps.RunLogger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRootHome)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/messages", s.handleMessages)
	mux.HandleFunc("/v1/messages/count_tokens", s.handleCountTokens)
	mux.HandleFunc("/v1/chat/completions", s.handleOpenAIChatCompletions)
	mux.HandleFunc("/v1/responses", s.handleOpenAIResponses)
	mux.HandleFunc("/v1/cc/sessions", s.handleCCSessions)
	mux.HandleFunc("/v1/cc/sessions/", s.handleCCSessionByPath)
	mux.HandleFunc("/v1/cc/runs", s.handleCCRuns)
	mux.HandleFunc("/v1/cc/runs/", s.handleCCRunByPath)
	mux.HandleFunc("/v1/cc/todos", s.handleCCTodos)
	mux.HandleFunc("/v1/cc/todos/", s.handleCCTodoByPath)
	mux.HandleFunc("/v1/cc/plans", s.handleCCPlans)
	mux.HandleFunc("/v1/cc/plans/", s.handleCCPlanByPath)
	mux.HandleFunc("/v1/cc/events", s.handleCCEvents)
	mux.HandleFunc("/v1/cc/events/stream", s.handleCCEventsStream)
	mux.HandleFunc("/v1/cc/teams", s.handleCCTeams)
	mux.HandleFunc("/v1/cc/teams/", s.handleCCTeamByPath)
	mux.HandleFunc("/v1/cc/subagents", s.handleCCSubagents)
	mux.HandleFunc("/v1/cc/subagents/", s.handleCCSubagentByPath)
	mux.HandleFunc("/v1/cc/mcp/servers", s.handleCCMCPServers)
	mux.HandleFunc("/v1/cc/mcp/servers/", s.handleCCMCPServerByPath)
	mux.HandleFunc("/v1/cc/plugins", s.handleCCPlugins)
	mux.HandleFunc("/v1/cc/plugins/", s.handleCCPluginByPath)
	mux.HandleFunc("/admin/settings", s.handleAdminSettings)
	mux.HandleFunc("/v1/cc/skills", s.handleCCSkills)
	mux.HandleFunc("/v1/cc/skills/", s.handleCCSkillByPath)
	mux.HandleFunc("/admin/tools", s.handleAdminTools)
	mux.HandleFunc("/admin/scheduler", s.handleAdminScheduler)
	mux.HandleFunc("/admin/probe", s.handleAdminProbe)
	mux.HandleFunc("/admin/auth/status", s.handleAdminAuthStatus)
	mux.HandleFunc("/admin/cost", s.handleAdminCost)
	mux.HandleFunc("/admin/status", s.handleAdminStatus)
	mux.HandleFunc("/admin/", s.handleAdminDashboard)
	mux.HandleFunc("/v1/cc/eval", s.handleCCEval)
	return withCommonHeaders(mux)
}

func withCommonHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-content-type-options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (s *server) nextID(prefix string) string {
	n := atomic.AddUint64(&s.idCounter, 1)
	return fmt.Sprintf("%s_%d_%x", prefix, time.Now().Unix(), n)
}

func (s *server) writeError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorEnvelope{
		Type: "error",
		Error: ErrorResponse{
			Type:    kind,
			Message: message,
		},
	})
}

func requireAnthropicVersion(r *http.Request) error {
	if strings.TrimSpace(r.Header.Get("anthropic-version")) == "" {
		return errors.New("missing anthropic-version header")
	}
	return nil
}

func (s *server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}`))
}
