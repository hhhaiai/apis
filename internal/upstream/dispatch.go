package upstream

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/scheduler"
)

// ========== 任务复杂度分类 ==========

// DispatchConfig controls the task dispatch behavior.
type DispatchConfig struct {
	Enabled              bool    `json:"enabled"`
	FallbackToScheduler bool    `json:"fallback_to_scheduler"` // 失败时回退到调度器
	MinScoreDifference  float64 `json:"min_score_difference"` // 选举最小分数差
	ReElectIntervalMS   int64   `json:"re_elect_interval_ms"` // 重新选举间隔(毫秒)
}

// DispatchStats 调度统计信息
type DispatchStats struct {
	ComplexRouted   int64 `json:"complex_routed"`   // 复杂任务路由次数
	SimpleRouted   int64 `json:"simple_routed"`    // 简单任务路由次数
	FallbackCount   int64 `json:"fallback_count"`   // 回退次数
}

// DispatchEvent 调度事件
type DispatchEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	EventType string    `json:"event_type"` // route_selected, election_changed, fallback, config_updated
	Complexity string    `json:"complexity,omitempty"`
	Selected  string    `json:"selected,omitempty"`
	FallbackTo string   `json:"fallback_to,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// Dispatcher routes requests to scheduler or worker adapters based on complexity.
type Dispatcher struct {
	mu       sync.RWMutex
	cfg      DispatchConfig
	election *scheduler.Election
	counter  uint64 // for round-robin
	classifier *TaskClassifier

	// Stats
	stats DispatchStats

	// Event log (circular buffer)
	eventsMu         sync.RWMutex
	eventLog        []DispatchEvent
	eventLogIdx     int
	eventLogSize    int
	maxEventLogSize int
}

// NewDispatcher creates a new Dispatcher.
func NewDispatcher(cfg DispatchConfig, election *scheduler.Election) *Dispatcher {
	// Set defaults if not explicitly configured
	if !cfg.FallbackToScheduler {
		cfg.FallbackToScheduler = true // default enabled
	}
	if cfg.MinScoreDifference <= 0 {
		cfg.MinScoreDifference = 5.0 // default min score difference
	}
	if cfg.ReElectIntervalMS <= 0 {
		cfg.ReElectIntervalMS = 600000 // default 10 minutes
	}
	d := &Dispatcher{
		cfg:              cfg,
		election:         election,
		classifier:       NewTaskClassifier(),
		eventLog:         make([]DispatchEvent, 100),
		eventLogSize:     0,
		maxEventLogSize:  100,
	}
	return d
}

// ClassifyComplexity determines if a request is "complex" (should go to scheduler model)
// or "simple" (can go to any worker). Uses TaskClassifier for intelligent classification.
func (d *Dispatcher) ClassifyComplexity(ctx context.Context, req orchestrator.Request) string {
	// Use TaskClassifier for intelligent classification
	complexity := d.classifier.ClassifyTask(ctx, req.Messages)

	// Map TaskComplexity to dispatch complexity
	switch complexity {
	case ComplexityVeryHigh, ComplexityHigh:
		// High complexity tasks need scheduler (high intelligence model)
		return "complex"
	case ComplexityMedium:
		// Medium complexity - check additional factors
		if len(req.Tools) > 0 || d.hasLongContext(req) {
			return "complex"
		}
		return "simple"
	default:
		// Low complexity - check if has tools (tool use is complex)
		if len(req.Tools) > 0 {
			return "complex"
		}
		return "simple"
	}
}

// hasLongContext checks if the request has long context.
func (d *Dispatcher) hasLongContext(req orchestrator.Request) bool {
	totalLen := 0
	for _, m := range req.Messages {
		if s, ok := m.Content.(string); ok {
			totalLen += len(s)
		}
	}
	return totalLen > 4000
}

// ClassifyComplexityStatic determines complexity without dispatcher instance.
// Falls back to simple heuristic when classifier unavailable.
func ClassifyComplexityStatic(req orchestrator.Request) string {
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
func (d *Dispatcher) RouteRequest(ctx context.Context, req orchestrator.Request, allAdapters []string) []string {
	if d == nil || !d.cfg.Enabled || d.election == nil {
		return nil
	}

	result := d.election.Result()
	if result == nil {
		return nil
	}

	complexity := d.ClassifyComplexity(ctx, req)
	schedulerName := result.SchedulerAdapter

	switch complexity {
	case "complex":
		// Scheduler model handles complex requests, workers as fallback
		atomic.AddInt64(&d.stats.ComplexRouted, 1)

		// Check if scheduler is healthy (not in cooldown)
		if d.isSchedulerHealthy() {
			out := []string{schedulerName}
			for _, w := range result.Workers {
				out = append(out, w.AdapterName)
			}
			return out
		}

		// Scheduler not healthy, skip to workers
		atomic.AddInt64(&d.stats.FallbackCount, 1)
		if len(result.Workers) > 0 {
			out := make([]string, 0, len(result.Workers))
			for _, w := range result.Workers {
				out = append(out, w.AdapterName)
			}
			return out
		}
		// No workers, return scheduler as last resort
		return []string{schedulerName}

	default: // "simple"
		workers := d.election.WorkerAdapters()
		if len(workers) == 0 {
			// Only scheduler exists, use it
			atomic.AddInt64(&d.stats.SimpleRouted, 1)
			return []string{schedulerName}
		}

		// Check if workers are healthy
		healthyWorkers := d.filterHealthyWorkers(workers)
		if len(healthyWorkers) == 0 {
			// All workers unhealthy, fallback to scheduler if enabled
			if d.cfg.FallbackToScheduler {
				atomic.AddInt64(&d.stats.FallbackCount, 1)
				return []string{schedulerName}
			}
			// Fallback not enabled, return workers anyway
			atomic.AddInt64(&d.stats.SimpleRouted, 1)
			return workers
		}

		// Round-robin among healthy workers
		idx := atomic.AddUint64(&d.counter, 1)
		n := len(healthyWorkers)
		ordered := make([]string, 0, n+1)
		for i := 0; i < n; i++ {
			ordered = append(ordered, healthyWorkers[(int(idx)+i)%n])
		}
		// Add scheduler as last fallback
		if d.cfg.FallbackToScheduler {
			ordered = append(ordered, schedulerName)
		}
		atomic.AddInt64(&d.stats.SimpleRouted, 1)
		return ordered
	}
}

// isSchedulerHealthy checks if the scheduler is healthy based on election status
func (d *Dispatcher) isSchedulerHealthy() bool {
	if d.election == nil {
		return true
	}
	result := d.election.Result()
	if result == nil {
		return true
	}
	// Check if there's a valid scheduler adapter
	if result.SchedulerAdapter == "" {
		return false
	}
	// Check cooldown from scheduler engine if available
	// For now, consider healthy if election result exists
	return true
}

// filterHealthyWorkers filters out unhealthy workers
// Note: Actual health filtering is done by the scheduler Engine
// This returns all workers and lets the Engine handle health checks
func (d *Dispatcher) filterHealthyWorkers(workers []string) []string {
	if workers == nil || len(workers) == 0 {
		return nil
	}
	// Return all workers - Engine handles the actual health filtering
	// This ensures compatibility with the existing scheduler system
	return workers
}

// Snapshot returns the current dispatch state for admin/status reporting.
func (d *Dispatcher) Snapshot() map[string]any {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	cfg := d.cfg
	election := d.election
	stats := DispatchStats{
		ComplexRouted: atomic.LoadInt64(&d.stats.ComplexRouted),
		SimpleRouted: atomic.LoadInt64(&d.stats.SimpleRouted),
		FallbackCount: atomic.LoadInt64(&d.stats.FallbackCount),
	}
	d.mu.RUnlock()

	snap := map[string]any{
		"enabled":               cfg.Enabled,
		"fallback_to_scheduler": cfg.FallbackToScheduler,
		"min_score_difference":  cfg.MinScoreDifference,
		"re_elect_interval_ms":  cfg.ReElectIntervalMS,
		"stats": map[string]int64{
			"complex_routed": stats.ComplexRouted,
			"simple_routed":  stats.SimpleRouted,
			"fallback_count": stats.FallbackCount,
		},
		"last_updated": time.Now().UTC().Format(time.RFC3339),
	}
	if election != nil {
		snap["election"] = election.Snapshot()
	}
	// Include recent events (has its own lock)
	snap["recent_events"] = d.GetEventLog(20)
	return snap
}

// UpdateConfig updates the dispatch configuration dynamically.
func (d *Dispatcher) UpdateConfig(cfg DispatchConfig) {
	if d == nil {
		return
	}
	// Apply defaults for any unset values
	if !cfg.FallbackToScheduler {
		cfg.FallbackToScheduler = true
	}
	if cfg.MinScoreDifference <= 0 {
		cfg.MinScoreDifference = 5.0
	}
	if cfg.ReElectIntervalMS <= 0 {
		cfg.ReElectIntervalMS = 600000
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cfg = cfg
}

// GetConfig returns the current dispatch configuration.
func (d *Dispatcher) GetConfig() DispatchConfig {
	if d == nil {
		return DispatchConfig{Enabled: false}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// ResetStats resets the dispatch statistics.
func (d *Dispatcher) ResetStats() {
	if d == nil {
		return
	}
	atomic.StoreInt64(&d.stats.ComplexRouted, 0)
	atomic.StoreInt64(&d.stats.SimpleRouted, 0)
	atomic.StoreInt64(&d.stats.FallbackCount, 0)
}

// RerunElection triggers a re-election manually.
func (d *Dispatcher) RerunElection() {
	if d == nil || d.election == nil {
		return
	}
	d.election.Elect()
	d.logEvent(DispatchEvent{
		EventType: "election_changed",
		Reason:    "manual_rerun",
	})
}

// logEvent adds an event to the circular event log.
func (d *Dispatcher) logEvent(event DispatchEvent) {
	if d == nil {
		return
	}
	event.Timestamp = time.Now()
	d.eventsMu.Lock()
	defer d.eventsMu.Unlock()

	d.eventLog[d.eventLogIdx] = event
	d.eventLogIdx = (d.eventLogIdx + 1) % d.maxEventLogSize
	if d.eventLogSize < d.maxEventLogSize {
		d.eventLogSize++
	}
}

// GetEventLog returns the recent dispatch events.
func (d *Dispatcher) GetEventLog(limit int) []DispatchEvent {
	if d == nil {
		return nil
	}

	d.eventsMu.RLock()
	defer d.eventsMu.RUnlock()

	maxSize := d.maxEventLogSize
	if limit <= 0 || limit > maxSize {
		limit = maxSize
	}

	if d.eventLogSize == 0 {
		return nil
	}

	result := make([]DispatchEvent, 0, limit)
	// Start from the oldest event
	startIdx := (d.eventLogIdx - d.eventLogSize + maxSize) % maxSize
	for i := 0; i < d.eventLogSize && i < limit; i++ {
		idx := (startIdx + i) % maxSize
		if d.eventLog[idx].EventType != "" {
			result = append(result, d.eventLog[idx])
		}
	}
	return result
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
