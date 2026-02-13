package upstream

import (
	"strings"
	"sync"
	"sync/atomic"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/scheduler"
)

// DispatchConfig controls the task dispatch behavior.
type DispatchConfig struct {
	Enabled bool `json:"enabled"`
}

// Dispatcher routes requests to scheduler or worker adapters based on complexity.
type Dispatcher struct {
	mu       sync.RWMutex
	cfg      DispatchConfig
	election *scheduler.Election
	counter  uint64 // for round-robin
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(cfg DispatchConfig, election *scheduler.Election) *Dispatcher {
	return &Dispatcher{
		cfg:      cfg,
		election: election,
	}
}

// ClassifyComplexity determines if a request is "complex" (should go to scheduler model)
// or "simple" (can go to any worker).
func ClassifyComplexity(req orchestrator.Request) string {
	// Complex if: has tools, long context, or planning keywords
	if len(req.Tools) > 0 {
		return "complex"
	}

	// Count total message length
	totalLen := 0
	for _, m := range req.Messages {
		if s, ok := m.Content.(string); ok {
			totalLen += len(s)
		}
	}
	if totalLen > 4000 {
		return "complex"
	}

	// Check for planning/reasoning keywords in system prompt
	sys := renderSystem(req.System)
	planningKeywords := []string{
		"plan", "architect", "design", "analyze", "debug",
		"refactor", "review", "thinking", "reasoning", "step by step",
	}
	sysLower := strings.ToLower(sys)
	for _, kw := range planningKeywords {
		if strings.Contains(sysLower, kw) {
			return "complex"
		}
	}

	return "simple"
}

// RouteRequest returns the ordered list of adapter names to try for a given request.
// If dispatch is enabled and election has completed:
//   - Complex requests → scheduler adapter first
//   - Simple requests → round-robin among workers, scheduler as fallback
//
// If dispatch is disabled or no election result, returns nil (use default routing).
func (d *Dispatcher) RouteRequest(req orchestrator.Request, allAdapters []string) []string {
	if d == nil || !d.cfg.Enabled || d.election == nil {
		return nil
	}

	result := d.election.Result()
	if result == nil {
		return nil
	}

	complexity := ClassifyComplexity(req)
	schedulerName := result.SchedulerAdapter

	switch complexity {
	case "complex":
		// Scheduler model handles complex requests, workers as fallback
		out := []string{schedulerName}
		for _, w := range result.Workers {
			out = append(out, w.AdapterName)
		}
		return out

	default: // "simple"
		workers := d.election.WorkerAdapters()
		if len(workers) == 0 {
			// Only scheduler exists, use it
			return []string{schedulerName}
		}

		// Round-robin among workers
		idx := atomic.AddUint64(&d.counter, 1)
		n := len(workers)
		ordered := make([]string, 0, n+1)
		for i := 0; i < n; i++ {
			ordered = append(ordered, workers[(int(idx)+i)%n])
		}
		// Add scheduler as last fallback
		ordered = append(ordered, schedulerName)
		return ordered
	}
}

// Snapshot returns the current dispatch state for admin/status reporting.
func (d *Dispatcher) Snapshot() map[string]any {
	if d == nil {
		return nil
	}
	snap := map[string]any{
		"enabled": d.cfg.Enabled,
	}
	if d.election != nil {
		snap["election"] = d.election.Snapshot()
	}
	return snap
}

func renderSystem(system any) string {
	switch s := system.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return ""
	}
}
