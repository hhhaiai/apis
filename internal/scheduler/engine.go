package scheduler

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"ccgateway/internal/orchestrator"
)

type Config struct {
	FailureThreshold   int
	Cooldown           time.Duration
	StrictProbeGate    bool
	RequireStreamProbe bool
	RequireToolProbe   bool
}

type ConfigPatch struct {
	FailureThreshold   *int   `json:"failure_threshold,omitempty"`
	CooldownMS         *int64 `json:"cooldown_ms,omitempty"`
	StrictProbeGate    *bool  `json:"strict_probe_gate,omitempty"`
	RequireStreamProbe *bool  `json:"require_stream_probe,omitempty"`
	RequireToolProbe   *bool  `json:"require_tool_probe,omitempty"`
}

type ProbeResult struct {
	CheckedAt     time.Time
	Exists        bool
	StreamChecked bool
	StreamOK      bool
	ToolChecked   bool
	ToolOK        bool
	Latency       time.Duration
	Error         string
}

type Engine struct {
	mu       sync.RWMutex
	cfg      Config
	adapters map[string]*adapterState
}

type adapterState struct {
	name                string
	successes           int64
	failures            int64
	consecutiveFailures int
	lastLatency         time.Duration
	lastError           string
	lastSuccessAt       time.Time
	lastFailureAt       time.Time
	cooldownUntil       time.Time
	models              map[string]modelProbe
}

type modelProbe struct {
	CheckedAt     time.Time
	ExistsKnown   bool
	Exists        bool
	StreamKnown   bool
	StreamOK      bool
	ToolKnown     bool
	ToolOK        bool
	LastError     string
	LastLatencyMS int64
}

type scoredCandidate struct {
	name    string
	score   float64
	allowed bool
	order   int
}

func NewEngine(cfg Config, adapterNames []string) *Engine {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 3
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 30 * time.Second
	}
	e := &Engine{
		cfg:      cfg,
		adapters: map[string]*adapterState{},
	}
	for _, name := range adapterNames {
		e.ensureAdapterLocked(name)
	}
	return e
}

func (e *Engine) Order(req orchestrator.Request, candidates []string, wantStream bool) []string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(candidates) == 0 {
		return nil
	}
	now := time.Now()
	model := strings.TrimSpace(req.Model)
	needTool := len(req.Tools) > 0
	scored := make([]scoredCandidate, 0, len(candidates))

	for i, name := range candidates {
		st := e.ensureAdapterLocked(name)
		allowed := e.allowed(st, model, wantStream, needTool, now)
		score := e.score(st, model, wantStream, needTool, now)
		scored = append(scored, scoredCandidate{
			name:    name,
			score:   score,
			allowed: allowed,
			order:   i,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].allowed != scored[j].allowed {
			return scored[i].allowed
		}
		if scored[i].score == scored[j].score {
			if scored[i].order == scored[j].order {
				return scored[i].name < scored[j].name
			}
			return scored[i].order < scored[j].order
		}
		return scored[i].score > scored[j].score
	})

	out := make([]string, 0, len(scored))
	for _, c := range scored {
		if c.allowed {
			out = append(out, c.name)
		}
	}
	if len(out) > 0 {
		return out
	}
	if e.cfg.StrictProbeGate {
		return nil
	}
	for _, c := range scored {
		out = append(out, c.name)
	}
	return out
}

func (e *Engine) ObserveSuccess(adapterName, model string, latency time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.ensureAdapterLocked(adapterName)
	st.successes++
	st.consecutiveFailures = 0
	st.lastLatency = latency
	st.lastSuccessAt = time.Now()
	st.lastError = ""
	model = strings.TrimSpace(model)
	if model != "" {
		mp := st.models[model]
		mp.ExistsKnown = true
		mp.Exists = true
		mp.CheckedAt = time.Now()
		mp.LastLatencyMS = latency.Milliseconds()
		st.models[model] = mp
	}
}

func (e *Engine) ObserveFailure(adapterName, model string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.ensureAdapterLocked(adapterName)
	st.failures++
	st.consecutiveFailures++
	st.lastFailureAt = time.Now()
	st.lastError = strings.TrimSpace(errorText(err))
	if st.consecutiveFailures >= e.cfg.FailureThreshold {
		st.cooldownUntil = time.Now().Add(e.cfg.Cooldown)
	}
	model = strings.TrimSpace(model)
	if model != "" {
		mp := st.models[model]
		mp.CheckedAt = time.Now()
		mp.LastError = st.lastError
		if isModelNotFound(err) {
			mp.ExistsKnown = true
			mp.Exists = false
		}
		st.models[model] = mp
	}
}

func (e *Engine) UpdateProbe(adapterName, model string, result ProbeResult) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := e.ensureAdapterLocked(adapterName)
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	at := result.CheckedAt
	if at.IsZero() {
		at = time.Now()
	}
	mp := st.models[model]
	mp.CheckedAt = at
	mp.ExistsKnown = true
	mp.Exists = result.Exists
	if result.StreamChecked {
		mp.StreamKnown = true
		mp.StreamOK = result.StreamOK
	}
	if result.ToolChecked {
		mp.ToolKnown = true
		mp.ToolOK = result.ToolOK
	}
	if result.Latency > 0 {
		mp.LastLatencyMS = result.Latency.Milliseconds()
	}
	mp.LastError = strings.TrimSpace(result.Error)
	st.models[model] = mp
}

func (e *Engine) Snapshot() map[string]any {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := map[string]any{}
	for name, st := range e.adapters {
		models := map[string]any{}
		for model, mp := range st.models {
			models[model] = map[string]any{
				"checked_at":      mp.CheckedAt,
				"exists_known":    mp.ExistsKnown,
				"exists":          mp.Exists,
				"stream_known":    mp.StreamKnown,
				"stream_ok":       mp.StreamOK,
				"tool_known":      mp.ToolKnown,
				"tool_ok":         mp.ToolOK,
				"last_error":      mp.LastError,
				"last_latency_ms": mp.LastLatencyMS,
			}
		}
		out[name] = map[string]any{
			"successes":            st.successes,
			"failures":             st.failures,
			"consecutive_failures": st.consecutiveFailures,
			"last_error":           st.lastError,
			"last_latency_ms":      st.lastLatency.Milliseconds(),
			"cooldown_until":       st.cooldownUntil,
			"models":               models,
		}
	}
	return out
}

func (e *Engine) Config() Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

func (e *Engine) UpdateConfigPatch(patch ConfigPatch) (Config, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	next := e.cfg
	if patch.FailureThreshold != nil {
		next.FailureThreshold = *patch.FailureThreshold
	}
	if patch.CooldownMS != nil {
		next.Cooldown = time.Duration(*patch.CooldownMS) * time.Millisecond
	}
	if patch.StrictProbeGate != nil {
		next.StrictProbeGate = *patch.StrictProbeGate
	}
	if patch.RequireStreamProbe != nil {
		next.RequireStreamProbe = *patch.RequireStreamProbe
	}
	if patch.RequireToolProbe != nil {
		next.RequireToolProbe = *patch.RequireToolProbe
	}
	if next.FailureThreshold <= 0 {
		return e.cfg, errors.New("failure_threshold must be > 0")
	}
	if next.Cooldown <= 0 {
		return e.cfg, errors.New("cooldown_ms must be > 0")
	}
	e.cfg = next
	return e.cfg, nil
}

func (e *Engine) AdminSnapshot() map[string]any {
	cfg := e.Config()
	return map[string]any{
		"config": map[string]any{
			"failure_threshold":    cfg.FailureThreshold,
			"cooldown_ms":          cfg.Cooldown.Milliseconds(),
			"strict_probe_gate":    cfg.StrictProbeGate,
			"require_stream_probe": cfg.RequireStreamProbe,
			"require_tool_probe":   cfg.RequireToolProbe,
		},
		"adapters": e.Snapshot(),
	}
}

func (e *Engine) allowed(st *adapterState, model string, wantStream, needTool bool, now time.Time) bool {
	if !st.cooldownUntil.IsZero() && now.Before(st.cooldownUntil) {
		return false
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return true
	}
	mp, ok := st.models[model]
	if !ok {
		return true
	}
	if mp.ExistsKnown && !mp.Exists {
		return false
	}
	if wantStream && e.cfg.RequireStreamProbe && mp.StreamKnown && !mp.StreamOK {
		return false
	}
	if needTool && e.cfg.RequireToolProbe && mp.ToolKnown && !mp.ToolOK {
		return false
	}
	return true
}

func (e *Engine) score(st *adapterState, model string, wantStream, needTool bool, now time.Time) float64 {
	score := 100.0
	if !st.cooldownUntil.IsZero() && now.Before(st.cooldownUntil) {
		return -1000
	}
	score -= float64(st.consecutiveFailures) * 15
	if st.lastLatency > 0 {
		penalty := float64(st.lastLatency.Milliseconds()) / 120.0
		if penalty > 30 {
			penalty = 30
		}
		score -= penalty
	}
	total := st.successes + st.failures
	if total > 0 {
		successRate := float64(st.successes) / float64(total)
		score += (successRate - 0.5) * 40
	}

	model = strings.TrimSpace(model)
	if model == "" {
		return score
	}
	mp, ok := st.models[model]
	if !ok {
		return score
	}
	if mp.ExistsKnown && !mp.Exists {
		score -= 500
	}
	if wantStream && mp.StreamKnown {
		if mp.StreamOK {
			score += 3
		} else {
			score -= 20
		}
	}
	if needTool && mp.ToolKnown {
		if mp.ToolOK {
			score += 3
		} else {
			score -= 20
		}
	}
	return score
}

func (e *Engine) ensureAdapterLocked(name string) *adapterState {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unknown"
	}
	st, ok := e.adapters[name]
	if ok {
		return st
	}
	st = &adapterState{
		name:   name,
		models: map[string]modelProbe{},
	}
	e.adapters[name] = st
	return st
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func isModelNotFound(err error) bool {
	if err == nil {
		return false
	}
	raw := strings.ToLower(strings.TrimSpace(err.Error()))
	if raw == "" {
		return false
	}
	hints := []string{
		"model not found",
		"unknown model",
		"invalid model",
		"no such model",
		"model does not exist",
	}
	for _, h := range hints {
		if strings.Contains(raw, h) {
			return true
		}
	}
	return errors.Is(err, ErrModelNotFound)
}

var ErrModelNotFound = errors.New("model not found")
