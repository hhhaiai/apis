package probe

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/scheduler"
	"ccgateway/internal/upstream"
)

type Config struct {
	Enabled         bool
	Interval        time.Duration
	Timeout         time.Duration
	DefaultModels   []string
	ModelsByAdapter map[string][]string
	StreamSmoke     bool
	ToolSmoke       bool
}

type ConfigPatch struct {
	Enabled         *bool               `json:"enabled,omitempty"`
	IntervalMS      *int64              `json:"interval_ms,omitempty"`
	TimeoutMS       *int64              `json:"timeout_ms,omitempty"`
	DefaultModels   []string            `json:"default_models,omitempty"`
	ModelsByAdapter map[string][]string `json:"models_by_adapter,omitempty"`
	StreamSmoke     *bool               `json:"stream_smoke,omitempty"`
	ToolSmoke       *bool               `json:"tool_smoke,omitempty"`
}

type Runner struct {
	mu              sync.RWMutex
	cfg             Config
	adapters        []upstream.Adapter
	health          *scheduler.Engine
	totalRuns       int64
	lastRunAt       time.Time
	lastRunDuration time.Duration
	lastRunChecks   int
	lastRunErrors   int
}

type modelHintAdapter interface {
	upstream.Adapter
	ModelHint() string
}

func NewRunner(cfg Config, adapters []upstream.Adapter, health *scheduler.Engine) *Runner {
	if health == nil {
		return nil
	}
	cfg = sanitizeConfig(cfg)
	return &Runner{
		cfg:      cfg,
		adapters: append([]upstream.Adapter(nil), adapters...),
		health:   health,
	}
}

func (r *Runner) Start(ctx context.Context) {
	if r == nil || !r.Config().Enabled {
		return
	}
	go r.loop(ctx)
}

func (r *Runner) RunOnce(ctx context.Context) {
	if r == nil {
		return
	}
	cfg := r.Config()
	if !cfg.Enabled {
		return
	}
	started := time.Now()
	checks := 0
	errors := 0
	for _, adapter := range r.adapters {
		if adapter == nil {
			continue
		}
		name := strings.TrimSpace(adapter.Name())
		if name == "" {
			continue
		}
		models := r.modelsForAdapter(cfg, name, adapter)
		for _, model := range models {
			model = strings.TrimSpace(model)
			if model == "" {
				continue
			}
			checks++
			if !r.probeOne(ctx, cfg, adapter, model) {
				errors++
			}
		}
	}
	r.mu.Lock()
	r.totalRuns++
	r.lastRunAt = time.Now()
	r.lastRunDuration = time.Since(started)
	r.lastRunChecks = checks
	r.lastRunErrors = errors
	r.mu.Unlock()
}

func (r *Runner) loop(ctx context.Context) {
	r.RunOnce(ctx)
	ticker := time.NewTicker(r.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.RunOnce(ctx)
		}
	}
}

func (r *Runner) probeOne(ctx context.Context, cfg Config, adapter upstream.Adapter, model string) bool {
	started := time.Now()
	pr := scheduler.ProbeResult{
		CheckedAt: started,
	}

	completeReq := orchestrator.Request{
		Model:     model,
		MaxTokens: 16,
		System:    "health probe",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "ping"},
		},
	}
	probeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	_, err := adapter.Complete(probeCtx, completeReq)
	cancel()
	if err != nil {
		pr.Error = err.Error()
		pr.Exists = false
		r.health.UpdateProbe(adapter.Name(), model, pr)
		return false
	}

	pr.Exists = true
	pr.Latency = time.Since(started)

	if cfg.StreamSmoke {
		pr.StreamChecked = true
		pr.StreamOK = r.streamSmoke(ctx, cfg, adapter, model)
		if !pr.StreamOK && pr.Error == "" {
			pr.Error = "stream smoke failed"
		}
	}

	if cfg.ToolSmoke {
		pr.ToolChecked = true
		ok, terr := r.toolSmoke(ctx, cfg, adapter, model)
		pr.ToolOK = ok
		if terr != nil && pr.Error == "" {
			pr.Error = terr.Error()
		}
	}
	r.health.UpdateProbe(adapter.Name(), model, pr)
	if strings.TrimSpace(pr.Error) != "" {
		return false
	}
	if !pr.Exists {
		return false
	}
	if pr.StreamChecked && !pr.StreamOK {
		return false
	}
	if pr.ToolChecked && !pr.ToolOK {
		return false
	}
	return true
}

func (r *Runner) streamSmoke(ctx context.Context, cfg Config, adapter upstream.Adapter, model string) bool {
	streaming, ok := adapter.(upstream.StreamingAdapter)
	if !ok {
		return false
	}
	req := orchestrator.Request{
		Model:     model,
		MaxTokens: 16,
		Messages: []orchestrator.Message{
			{Role: "user", Content: "ping"},
		},
	}
	probeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	events, errs := streaming.Stream(probeCtx, req)

	timeout := time.NewTimer(cfg.Timeout)
	defer timeout.Stop()
	evCh := events
	errCh := errs
	for {
		select {
		case _, ok := <-evCh:
			if ok {
				return true
			}
			evCh = nil
			if errCh == nil {
				return false
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
				if evCh == nil {
					return false
				}
				continue
			}
			if err == nil {
				continue
			}
			return false
		case <-timeout.C:
			return false
		case <-probeCtx.Done():
			return false
		}
	}
}

func (r *Runner) toolSmoke(ctx context.Context, cfg Config, adapter upstream.Adapter, model string) (bool, error) {
	req := orchestrator.Request{
		Model:     model,
		MaxTokens: 32,
		System:    "tool probe",
		Messages: []orchestrator.Message{
			{Role: "user", Content: "You must call tool get_weather and return nothing else."},
		},
		Tools: []orchestrator.Tool{
			{
				Name:        "get_weather",
				Description: "probe tool",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
					"required": []string{"city"},
				},
			},
		},
	}

	probeCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	resp, err := adapter.Complete(probeCtx, req)
	if err != nil {
		return false, err
	}
	if resp.StopReason == "tool_use" {
		return true, nil
	}
	for _, b := range resp.Blocks {
		if b.Type == "tool_use" {
			return true, nil
		}
	}
	return false, fmt.Errorf("tool smoke expected tool_use, got stop_reason=%s", strings.TrimSpace(resp.StopReason))
}

func (r *Runner) modelsForAdapter(cfg Config, name string, adapter upstream.Adapter) []string {
	if cfgModels, ok := cfg.ModelsByAdapter[name]; ok && len(cfgModels) > 0 {
		return append([]string(nil), cfgModels...)
	}
	if len(cfg.DefaultModels) > 0 {
		return append([]string(nil), cfg.DefaultModels...)
	}
	if hinted, ok := adapter.(modelHintAdapter); ok {
		if m := strings.TrimSpace(hinted.ModelHint()); m != "" {
			return []string{m}
		}
	}
	return nil
}

func (r *Runner) Snapshot() map[string]any {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg := cloneConfig(r.cfg)
	return map[string]any{
		"enabled":              cfg.Enabled,
		"interval_ms":          cfg.Interval.Milliseconds(),
		"timeout_ms":           cfg.Timeout.Milliseconds(),
		"stream_smoke":         cfg.StreamSmoke,
		"tool_smoke":           cfg.ToolSmoke,
		"default_models":       append([]string(nil), cfg.DefaultModels...),
		"models_by_adapter":    copyModelsByAdapter(cfg.ModelsByAdapter),
		"total_runs":           r.totalRuns,
		"last_run_at":          r.lastRunAt,
		"last_run_duration_ms": r.lastRunDuration.Milliseconds(),
		"last_run_checks":      r.lastRunChecks,
		"last_run_errors":      r.lastRunErrors,
	}
}

func (r *Runner) Config() Config {
	if r == nil {
		return Config{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return cloneConfig(r.cfg)
}

func (r *Runner) UpdateConfigPatch(patch ConfigPatch) (Config, error) {
	if r == nil {
		return Config{}, fmt.Errorf("probe runner is nil")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	next := cloneConfig(r.cfg)
	if patch.Enabled != nil {
		next.Enabled = *patch.Enabled
	}
	if patch.IntervalMS != nil {
		next.Interval = time.Duration(*patch.IntervalMS) * time.Millisecond
	}
	if patch.TimeoutMS != nil {
		next.Timeout = time.Duration(*patch.TimeoutMS) * time.Millisecond
	}
	if patch.StreamSmoke != nil {
		next.StreamSmoke = *patch.StreamSmoke
	}
	if patch.ToolSmoke != nil {
		next.ToolSmoke = *patch.ToolSmoke
	}
	if patch.DefaultModels != nil {
		next.DefaultModels = sanitizeModelList(patch.DefaultModels)
	}
	if patch.ModelsByAdapter != nil {
		next.ModelsByAdapter = sanitizeModelsByAdapter(patch.ModelsByAdapter)
	}
	next = sanitizeConfig(next)
	if next.Interval <= 0 {
		return cloneConfig(r.cfg), fmt.Errorf("interval_ms must be > 0")
	}
	if next.Timeout <= 0 {
		return cloneConfig(r.cfg), fmt.Errorf("timeout_ms must be > 0")
	}
	r.cfg = next
	return cloneConfig(r.cfg), nil
}

func cloneConfig(in Config) Config {
	out := in
	out.DefaultModels = append([]string(nil), in.DefaultModels...)
	out.ModelsByAdapter = copyModelsByAdapter(in.ModelsByAdapter)
	return out
}

func sanitizeConfig(in Config) Config {
	out := cloneConfig(in)
	if out.Interval <= 0 {
		out.Interval = 45 * time.Second
	}
	if out.Timeout <= 0 {
		out.Timeout = 8 * time.Second
	}
	out.DefaultModels = sanitizeModelList(out.DefaultModels)
	out.ModelsByAdapter = sanitizeModelsByAdapter(out.ModelsByAdapter)
	return out
}

func sanitizeModelList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func sanitizeModelsByAdapter(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		list := sanitizeModelList(v)
		out[k] = append([]string(nil), list...)
	}
	return out
}

func copyModelsByAdapter(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(in))
	for k, models := range in {
		out[k] = append([]string(nil), models...)
	}
	return out
}
