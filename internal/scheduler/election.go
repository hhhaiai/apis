package scheduler

import (
	"sort"
	"sync"
	"time"
)

// ElectionConfig controls how the scheduler model is elected.
type ElectionConfig struct {
	Enabled            bool          `json:"enabled"`
	MinScoreDifference float64       `json:"min_score_difference"` // min score gap to elect (default 5)
	ReElectInterval    time.Duration `json:"re_elect_interval"`    // how often to re-evaluate (default 10m)
}

// ElectionResult represents the current election state.
type ElectionResult struct {
	SchedulerAdapter string    `json:"scheduler_adapter"`
	SchedulerModel   string    `json:"scheduler_model"`
	SchedulerScore   float64   `json:"scheduler_score"`
	Workers          []Worker  `json:"workers"`
	ElectedAt        time.Time `json:"elected_at"`
	Reason           string    `json:"reason"`
}

// Worker represents a non-scheduler adapter that receives tasks.
type Worker struct {
	AdapterName string  `json:"adapter_name"`
	Model       string  `json:"model"`
	Score       float64 `json:"score"`
}

// IntelligenceScore is the input to the election: one score per adapter.
type IntelligenceScore struct {
	AdapterName string
	Model       string
	Score       float64 // 0-100
	TestedAt    time.Time
}

// Election manages the scheduler model election process.
type Election struct {
	mu       sync.RWMutex
	cfg      ElectionConfig
	scores   []IntelligenceScore
	result   *ElectionResult
	onChange func(result ElectionResult)
}

// NewElection creates a new Election manager.
func NewElection(cfg ElectionConfig) *Election {
	if cfg.MinScoreDifference <= 0 {
		cfg.MinScoreDifference = 5
	}
	if cfg.ReElectInterval <= 0 {
		cfg.ReElectInterval = 10 * time.Minute
	}
	return &Election{
		cfg:    cfg,
		scores: make([]IntelligenceScore, 0),
	}
}

// SetOnChange registers a callback for when the election result changes.
func (e *Election) SetOnChange(fn func(result ElectionResult)) {
	e.mu.Lock()
	e.onChange = fn
	e.mu.Unlock()
}

// UpdateScores receives new intelligence scores and triggers an election.
func (e *Election) UpdateScores(scores []IntelligenceScore) {
	e.mu.Lock()
	e.scores = append([]IntelligenceScore(nil), scores...)
	e.mu.Unlock()
	e.Elect()
}

// Elect runs the election algorithm: highest score becomes scheduler.
func (e *Election) Elect() {
	e.mu.Lock()
	if len(e.scores) == 0 {
		e.mu.Unlock()
		return
	}

	// Sort by score descending
	sorted := append([]IntelligenceScore(nil), e.scores...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Score > sorted[j].Score
	})

	best := sorted[0]
	workers := make([]Worker, 0, len(sorted)-1)
	for _, s := range sorted[1:] {
		workers = append(workers, Worker{
			AdapterName: s.AdapterName,
			Model:       s.Model,
			Score:       s.Score,
		})
	}

	reason := "highest_intelligence_score"
	// If only one adapter, it acts as both scheduler and worker
	if len(sorted) == 1 {
		reason = "single_adapter"
	}
	// If scores are too close, use latency/success as tiebreaker
	if len(sorted) > 1 && best.Score-sorted[1].Score < e.cfg.MinScoreDifference {
		reason = "close_scores_tiebreak"
	}

	result := ElectionResult{
		SchedulerAdapter: best.AdapterName,
		SchedulerModel:   best.Model,
		SchedulerScore:   best.Score,
		Workers:          workers,
		ElectedAt:        time.Now(),
		Reason:           reason,
	}
	e.result = &result
	fn := e.onChange
	e.mu.Unlock()

	if fn != nil {
		fn(result)
	}
}

// Result returns the current election result. Returns nil if no election has been held.
func (e *Election) Result() *ElectionResult {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.result == nil {
		return nil
	}
	r := *e.result
	r.Workers = append([]Worker(nil), e.result.Workers...)
	return &r
}

// IsScheduler returns true if the given adapter is the elected scheduler.
func (e *Election) IsScheduler(adapterName string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.result == nil {
		return false
	}
	return e.result.SchedulerAdapter == adapterName
}

// SchedulerAdapter returns the name of the elected scheduler adapter. Empty if none.
func (e *Election) SchedulerAdapter() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.result == nil {
		return ""
	}
	return e.result.SchedulerAdapter
}

// WorkerAdapters returns the list of non-scheduler adapter names.
func (e *Election) WorkerAdapters() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.result == nil {
		return nil
	}
	out := make([]string, 0, len(e.result.Workers))
	for _, w := range e.result.Workers {
		out = append(out, w.AdapterName)
	}
	return out
}

// Snapshot returns the current election state for admin/status reporting.
func (e *Election) Snapshot() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	snap := map[string]any{
		"enabled": e.cfg.Enabled,
	}
	if e.result != nil {
		snap["scheduler_adapter"] = e.result.SchedulerAdapter
		snap["scheduler_model"] = e.result.SchedulerModel
		snap["scheduler_score"] = e.result.SchedulerScore
		snap["elected_at"] = e.result.ElectedAt
		snap["reason"] = e.result.Reason
		snap["worker_count"] = len(e.result.Workers)
		workers := make([]map[string]any, 0, len(e.result.Workers))
		for _, w := range e.result.Workers {
			workers = append(workers, map[string]any{
				"adapter": w.AdapterName,
				"model":   w.Model,
				"score":   w.Score,
			})
		}
		snap["workers"] = workers
	}
	scores := make([]map[string]any, 0, len(e.scores))
	for _, s := range e.scores {
		scores = append(scores, map[string]any{
			"adapter":   s.AdapterName,
			"model":     s.Model,
			"score":     s.Score,
			"tested_at": s.TestedAt,
		})
	}
	snap["intelligence_scores"] = scores
	return snap
}
