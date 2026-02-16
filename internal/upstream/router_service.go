package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"ccgateway/internal/orchestrator"
)

type RouterConfig struct {
	Routes              map[string][]string
	DefaultRoute        []string
	Timeout             time.Duration
	Retries             int
	ReflectionPasses    int
	ParallelCandidates  int
	EnableResponseJudge bool
	Judge               CandidateJudge
	Selector            CandidateSelector
	Dispatcher          *Dispatcher
}

type RouterService struct {
	mu                 sync.RWMutex
	adapters           map[string]Adapter
	adapterSpecs       []AdapterSpec
	adapterOrder       []string
	routesExact        map[string][]string
	routePatterns      []routePattern
	defaultRoute       []string
	timeout            time.Duration
	retries            int
	reflectPasses      int
	parallelCandidates int
	enableJudge        bool
	judge              CandidateJudge
	selector           CandidateSelector
	dispatcher         *Dispatcher
}

type routePattern struct {
	pattern     string
	adapters    []string
	specificity int
}

type CandidateSelector interface {
	Order(req orchestrator.Request, candidates []string, wantStream bool) []string
	ObserveSuccess(adapterName, model string, latency time.Duration)
	ObserveFailure(adapterName, model string, err error)
}

func NewRouterService(cfg RouterConfig, adapters []Adapter) *RouterService {
	adapterMap := make(map[string]Adapter, len(adapters))
	order := make([]string, 0, len(adapters))
	specs := make([]AdapterSpec, 0, len(adapters))
	for _, a := range adapters {
		if a == nil {
			continue
		}
		name := strings.TrimSpace(a.Name())
		if name == "" {
			continue
		}
		adapterMap[name] = a
		order = append(order, name)
		specs = append(specs, snapshotAdapterSpec(a))
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	retries := cfg.Retries
	if retries < 0 {
		retries = 0
	}
	parallelCandidates := cfg.ParallelCandidates
	if parallelCandidates <= 0 {
		parallelCandidates = 1
	}
	judge := cfg.Judge
	if judge == nil {
		judge = NewHeuristicJudge()
	}

	exact, patterns := splitRoutes(cfg.Routes)
	return &RouterService{
		adapters:           adapterMap,
		adapterSpecs:       specs,
		adapterOrder:       order,
		routesExact:        exact,
		routePatterns:      patterns,
		defaultRoute:       append([]string(nil), cfg.DefaultRoute...),
		timeout:            timeout,
		retries:            retries,
		reflectPasses:      cfg.ReflectionPasses,
		parallelCandidates: parallelCandidates,
		enableJudge:        cfg.EnableResponseJudge,
		judge:              judge,
		selector:           cfg.Selector,
		dispatcher:         cfg.Dispatcher,
	}
}

func (s *RouterService) Complete(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	candidates := s.routeForRequest(ctx, req)
	if s.selector != nil {
		candidates = s.selector.Order(req, candidates, false)
	}
	if len(candidates) == 0 {
		return orchestrator.Response{}, fmt.Errorf("no upstream adapter available")
	}
	s.mu.RLock()
	retries := s.retries
	timeout := s.timeout
	reflectPasses := s.reflectPasses
	parallelCandidates := s.parallelCandidates
	enableJudge := s.enableJudge
	s.mu.RUnlock()
	if req.Metadata != nil {
		if v, ok := intFromAny(req.Metadata["routing_retries"]); ok && v >= 0 {
			retries = v
		}
		if v, ok := intFromAny(req.Metadata["reflection_passes"]); ok && v >= 0 {
			reflectPasses = v
		}
		if ms, ok := intFromAny(req.Metadata["routing_timeout_ms"]); ok && ms > 0 {
			timeout = time.Duration(ms) * time.Millisecond
		}
		if v, ok := intFromAny(req.Metadata["parallel_candidates"]); ok && v > 0 {
			parallelCandidates = v
		}
		if v, ok := req.Metadata["enable_response_judge"]; ok {
			enableJudge = boolFromAny(v)
		}
	}
	if parallelCandidates <= 0 {
		parallelCandidates = 1
	}
	if parallelCandidates > len(candidates) {
		parallelCandidates = len(candidates)
	}

	results, err := s.runCandidates(ctx, req, candidates, retries, timeout, parallelCandidates)
	if err != nil {
		return orchestrator.Response{}, err
	}

	chosen := s.pickCandidate(ctx, req, results, enableJudge)
	chosen.resp.Trace.Provider = chosen.adapterName
	chosen.resp.Trace.Model = req.Model
	chosen.resp.Trace.FallbackUsed = chosen.order > 0
	chosen.resp.Trace.CandidateCount = len(results)
	chosen.resp.Trace.JudgeEnabled = enableJudge && len(results) > 1
	chosen.resp.Trace.SelectedBy = chosen.selectedBy
	if reflectPasses > 0 {
		chosen.resp = s.applyReflectionLoop(ctx, chosen.resp, req, reflectPasses)
	}
	return chosen.resp, nil
}

func (s *RouterService) Stream(ctx context.Context, req orchestrator.Request) (<-chan orchestrator.StreamEvent, <-chan error) {
	events := make(chan orchestrator.StreamEvent, 16)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		candidates := s.routeForRequest(ctx, req)
		if s.selector != nil {
			candidates = s.selector.Order(req, candidates, true)
		}
		if len(candidates) == 0 {
			errs <- fmt.Errorf("no upstream adapter available")
			return
		}

		var lastErr error
		strict := boolFromAny(req.Metadata["strict_stream_passthrough"])
		strictSoft := true
		if req.Metadata != nil {
			if v, ok := req.Metadata["strict_stream_passthrough_soft"]; ok {
				strictSoft = boolFromAny(v)
			}
		}
		for _, name := range candidates {
			s.mu.RLock()
			adapter, ok := s.adapters[name]
			s.mu.RUnlock()
			if !ok {
				lastErr = fmt.Errorf("adapter %q not registered", name)
				continue
			}

			streaming, ok := adapter.(StreamingAdapter)
			if !ok {
				if s.selector != nil {
					s.selector.ObserveFailure(name, req.Model, fmt.Errorf("adapter does not support streaming"))
				}
				resp, err := s.Complete(ctx, req)
				if err != nil {
					lastErr = err
					continue
				}
				emitSyntheticStream(events, resp)
				return
			}

			streamEvents, streamErrs := streaming.Stream(ctx, req)
			streamStarted := time.Now()
			started := false
			evCh := streamEvents
			errCh := streamErrs

			for {
				select {
				case ev, ok := <-evCh:
					if !ok {
						evCh = nil
						if errCh == nil {
							if started {
								if s.selector != nil {
									s.selector.ObserveSuccess(name, req.Model, time.Since(streamStarted))
								}
								return
							}
							lastErr = fmt.Errorf("stream ended before any event from adapter %q", name)
							if s.selector != nil {
								s.selector.ObserveFailure(name, req.Model, lastErr)
							}
							goto nextAdapter
						}
						continue
					}
					started = true
					events <- ev
				case err, ok := <-errCh:
					if !ok {
						errCh = nil
						if evCh == nil {
							if started {
								if s.selector != nil {
									s.selector.ObserveSuccess(name, req.Model, time.Since(streamStarted))
								}
								return
							}
							lastErr = fmt.Errorf("stream closed without events from adapter %q", name)
							if s.selector != nil {
								s.selector.ObserveFailure(name, req.Model, lastErr)
							}
							goto nextAdapter
						}
						continue
					}
					if err == nil {
						continue
					}
					if started {
						if s.selector != nil {
							s.selector.ObserveFailure(name, req.Model, err)
						}
						errs <- err
						return
					}
					if strict && strictSoft && errors.Is(err, ErrStrictPassthroughUnsupported) {
						resp, cErr := s.Complete(ctx, req)
						if cErr != nil {
							lastErr = cErr
							if s.selector != nil {
								s.selector.ObserveFailure(name, req.Model, cErr)
							}
							goto nextAdapter
						}
						if s.selector != nil {
							s.selector.ObserveSuccess(name, req.Model, time.Since(streamStarted))
						}
						emitSyntheticStream(events, resp)
						return
					}
					lastErr = err
					if s.selector != nil {
						s.selector.ObserveFailure(name, req.Model, err)
					}
					goto nextAdapter
				case <-ctx.Done():
					if s.selector != nil {
						s.selector.ObserveFailure(name, req.Model, ctx.Err())
					}
					errs <- ctx.Err()
					return
				}
			}
		nextAdapter:
		}

		if lastErr == nil {
			lastErr = fmt.Errorf("all adapters failed")
		}
		errs <- lastErr
	}()

	return events, errs
}

type candidateResult struct {
	candidateName string
	adapterName   string
	resp          orchestrator.Response
	err           error
	latency       time.Duration
	order         int
	selectedBy    string
}

func (s *RouterService) runCandidates(
	ctx context.Context,
	req orchestrator.Request,
	candidates []string,
	retries int,
	timeout time.Duration,
	parallel int,
) ([]candidateResult, error) {
	if parallel <= 1 || len(candidates) <= 1 {
		var lastErr error
		for i, name := range candidates {
			r := s.runCandidate(ctx, req, name, i, retries, timeout)
			if r.err == nil {
				return []candidateResult{r}, nil
			}
			lastErr = r.err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("all adapters failed")
	}

	limited := candidates
	if parallel < len(candidates) {
		limited = candidates[:parallel]
	}

	out := make(chan candidateResult, len(limited))
	for i, name := range limited {
		go func(order int, adapterName string) {
			out <- s.runCandidate(ctx, req, adapterName, order, retries, timeout)
		}(i, name)
	}

	success := make([]candidateResult, 0, len(limited))
	var lastErr error
	for i := 0; i < len(limited); i++ {
		r := <-out
		if r.err != nil {
			lastErr = r.err
			continue
		}
		success = append(success, r)
	}
	if len(success) > 0 {
		return success, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("all adapters failed")
	}
	return nil, lastErr
}

func (s *RouterService) runCandidate(
	ctx context.Context,
	req orchestrator.Request,
	name string,
	order int,
	retries int,
	timeout time.Duration,
) candidateResult {
	s.mu.RLock()
	adapter, ok := s.adapters[name]
	s.mu.RUnlock()
	if !ok {
		return candidateResult{
			candidateName: name,
			adapterName:   name,
			order:         order,
			err:           fmt.Errorf("adapter %q not registered", name),
		}
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		started := time.Now()
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		resp, err := adapter.Complete(attemptCtx, req)
		cancel()
		if err != nil {
			if s.selector != nil {
				s.selector.ObserveFailure(name, req.Model, err)
			}
			lastErr = err
			continue
		}
		latency := time.Since(started)
		if s.selector != nil {
			s.selector.ObserveSuccess(name, req.Model, latency)
		}
		return candidateResult{
			candidateName: name,
			adapterName:   adapter.Name(),
			resp:          resp,
			latency:       latency,
			order:         order,
			selectedBy:    "priority",
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("adapter %q failed", name)
	}
	return candidateResult{
		candidateName: name,
		adapterName:   adapter.Name(),
		order:         order,
		err:           lastErr,
	}
}

func (s *RouterService) pickCandidate(ctx context.Context, req orchestrator.Request, candidates []candidateResult, enableJudge bool) candidateResult {
	if len(candidates) == 1 {
		only := candidates[0]
		only.selectedBy = "single"
		return only
	}

	if enableJudge && s.judge != nil {
		judged := make([]JudgedCandidate, 0, len(candidates))
		for _, c := range candidates {
			judged = append(judged, JudgedCandidate{
				AdapterName: c.adapterName,
				Response:    c.resp,
				Latency:     c.latency,
				Order:       c.order,
			})
		}
		idx, err := s.judge.Select(ctx, req, judged)
		if err == nil && idx >= 0 && idx < len(candidates) {
			chosen := candidates[idx]
			chosen.selectedBy = "judge"
			return chosen
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].order == candidates[j].order {
			return candidates[i].latency < candidates[j].latency
		}
		return candidates[i].order < candidates[j].order
	})
	chosen := candidates[0]
	chosen.selectedBy = "priority"
	return chosen
}

func emitSyntheticStream(events chan<- orchestrator.StreamEvent, resp orchestrator.Response) {
	events <- orchestrator.StreamEvent{Type: "message_start"}
	for i, b := range resp.Blocks {
		events <- orchestrator.StreamEvent{Type: "content_block_start", Index: i, Block: b}
		switch b.Type {
		case "text":
			for _, c := range splitTextDeltas(b.Text, 24) {
				if c == "" {
					continue
				}
				events <- orchestrator.StreamEvent{
					Type:      "content_block_delta",
					Index:     i,
					DeltaText: c,
				}
			}
		case "tool_use":
			raw, _ := json.Marshal(b.Input)
			events <- orchestrator.StreamEvent{
				Type:      "content_block_delta",
				Index:     i,
				DeltaJSON: string(raw),
			}
		}
		events <- orchestrator.StreamEvent{Type: "content_block_stop", Index: i}
	}

	events <- orchestrator.StreamEvent{
		Type:       "message_delta",
		StopReason: resp.StopReason,
		Usage:      resp.Usage,
	}
	events <- orchestrator.StreamEvent{Type: "message_stop"}
}

func (s *RouterService) routeForRequest(ctx context.Context, req orchestrator.Request) []string {
	if route := routeFromMetadata(req.Metadata); len(route) > 0 {
		return route
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	// Dispatcher-based routing: if enabled and election is done, use it
	if s.dispatcher != nil {
		if dispatched := s.dispatcher.RouteRequest(ctx, req, s.adapterOrder); len(dispatched) > 0 {
			return dispatched
		}
	}
	model := req.Model
	if seq, ok := s.routesExact[model]; ok && len(seq) > 0 {
		return append([]string(nil), seq...)
	}
	for _, p := range s.routePatterns {
		matched, err := path.Match(p.pattern, model)
		if err != nil {
			continue
		}
		if matched && len(p.adapters) > 0 {
			return append([]string(nil), p.adapters...)
		}
	}
	if seq, ok := s.routesExact["*"]; ok && len(seq) > 0 {
		return append([]string(nil), seq...)
	}
	if len(s.defaultRoute) > 0 {
		return append([]string(nil), s.defaultRoute...)
	}
	return append([]string(nil), s.adapterOrder...)
}

func splitRoutes(in map[string][]string) (map[string][]string, []routePattern) {
	exact := map[string][]string{}
	patterns := make([]routePattern, 0)
	for k, v := range cloneRoutes(in) {
		if strings.Contains(k, "*") && k != "*" {
			patterns = append(patterns, routePattern{
				pattern:     k,
				adapters:    v,
				specificity: len(strings.ReplaceAll(k, "*", "")),
			})
		} else {
			exact[k] = v
		}
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].specificity == patterns[j].specificity {
			return patterns[i].pattern < patterns[j].pattern
		}
		return patterns[i].specificity > patterns[j].specificity
	})
	return exact, patterns
}

func (s *RouterService) GetUpstreamConfig() UpstreamAdminConfig {
	return s.snapshotUpstreamConfig(true)
}

func (s *RouterService) snapshotUpstreamConfig(maskSecrets bool) UpstreamAdminConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := UpstreamAdminConfig{
		Adapters:     cloneAdapterSpecs(s.adapterSpecs, maskSecrets),
		DefaultRoute: append([]string(nil), s.defaultRoute...),
		ModelRoutes:  composeRoutesForAdmin(s.routesExact, s.routePatterns),
	}
	return out
}

func (s *RouterService) UpdateUpstreamConfig(cfg UpstreamAdminConfig) (UpstreamAdminConfig, error) {
	current := s.snapshotUpstreamConfig(false)
	if len(cfg.Adapters) == 0 {
		cfg.Adapters = current.Adapters
	}
	if cfg.ModelRoutes == nil {
		cfg.ModelRoutes = current.ModelRoutes
	}
	if len(cfg.DefaultRoute) == 0 {
		cfg.DefaultRoute = current.DefaultRoute
	}

	adapters, err := BuildAdaptersFromSpecs(cfg.Adapters)
	if err != nil {
		return UpstreamAdminConfig{}, err
	}
	if len(adapters) == 0 {
		return UpstreamAdminConfig{}, fmt.Errorf("at least one adapter is required")
	}

	adapterMap := make(map[string]Adapter, len(adapters))
	order := make([]string, 0, len(adapters))
	specs := make([]AdapterSpec, 0, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		name := strings.TrimSpace(adapter.Name())
		if name == "" {
			continue
		}
		adapterMap[name] = adapter
		order = append(order, name)
		specs = append(specs, snapshotAdapterSpec(adapter))
	}
	if len(order) == 0 {
		return UpstreamAdminConfig{}, fmt.Errorf("no valid adapters")
	}

	routes := cleanModelRoutes(cfg.ModelRoutes)
	for model, route := range routes {
		if len(route) == 0 {
			continue
		}
		for _, adapterName := range route {
			if _, ok := adapterMap[adapterName]; !ok {
				return UpstreamAdminConfig{}, fmt.Errorf("route %q references unknown adapter %q", model, adapterName)
			}
		}
	}

	defaultRoute := cleanRoute(cfg.DefaultRoute)
	if len(defaultRoute) == 0 {
		defaultRoute = append([]string(nil), order...)
	}
	for _, adapterName := range defaultRoute {
		if _, ok := adapterMap[adapterName]; !ok {
			return UpstreamAdminConfig{}, fmt.Errorf("default route references unknown adapter %q", adapterName)
		}
	}

	exact, patterns := splitRoutes(routes)

	s.mu.Lock()
	s.adapters = adapterMap
	s.adapterOrder = order
	s.adapterSpecs = specs
	s.defaultRoute = defaultRoute
	s.routesExact = exact
	s.routePatterns = patterns
	s.mu.Unlock()

	return s.GetUpstreamConfig(), nil
}

func cloneAdapterSpecs(in []AdapterSpec, maskSecrets bool) []AdapterSpec {
	if len(in) == 0 {
		return nil
	}
	out := make([]AdapterSpec, 0, len(in))
	for _, spec := range in {
		copySpec := sanitizeAdapterSpec(spec)
		if maskSecrets && strings.TrimSpace(copySpec.APIKey) != "" {
			copySpec.APIKey = "***"
		}
		out = append(out, copySpec)
	}
	return out
}

func snapshotAdapterSpec(adapter Adapter) AdapterSpec {
	if provider, ok := adapter.(interface{ AdminSpec() AdapterSpec }); ok {
		return sanitizeAdapterSpec(provider.AdminSpec())
	}
	return AdapterSpec{
		Name: strings.TrimSpace(adapter.Name()),
	}
}

func composeRoutesForAdmin(exact map[string][]string, patterns []routePattern) map[string][]string {
	out := cloneRoutes(exact)
	for _, p := range patterns {
		out[p.pattern] = append([]string(nil), p.adapters...)
	}
	return out
}

func routeFromMetadata(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata["routing_adapter_route"]
	if !ok {
		return nil
	}
	switch route := raw.(type) {
	case []string:
		out := make([]string, 0, len(route))
		for _, item := range route {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(route))
		for _, item := range route {
			v, ok := item.(string)
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			if v != "" {
				out = append(out, v)
			}
		}
		return out
	default:
		return nil
	}
}

func intFromAny(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0, false
		}
		var x int
		_, err := fmt.Sscanf(n, "%d", &x)
		if err != nil {
			return 0, false
		}
		return x, true
	default:
		return 0, false
	}
}

func cloneRoutes(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, v := range in {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// UpdateDispatchConfig updates the dispatcher configuration dynamically.
func (s *RouterService) UpdateDispatchConfig(enabled bool) error {
	s.mu.RLock()
	dispatcher := s.dispatcher
	s.mu.RUnlock()

	if dispatcher == nil {
		return fmt.Errorf("dispatcher is not configured")
	}
	dispatcher.UpdateConfig(DispatchConfig{Enabled: enabled})
	return nil
}

// UpdateDispatchConfigFull updates the dispatcher with full configuration.
func (s *RouterService) UpdateDispatchConfigFull(cfg DispatchConfig) error {
	s.mu.RLock()
	dispatcher := s.dispatcher
	s.mu.RUnlock()

	if dispatcher == nil {
		return fmt.Errorf("dispatcher is not configured")
	}
	dispatcher.UpdateConfig(cfg)
	return nil
}

// GetDispatchStatus returns the current dispatch status for admin API.
func (s *RouterService) GetDispatchStatus() map[string]any {
	s.mu.RLock()
	dispatcher := s.dispatcher
	s.mu.RUnlock()

	if dispatcher == nil {
		return map[string]any{
			"available": false,
			"reason":    "dispatcher not configured",
		}
	}

	return dispatcher.Snapshot()
}

// TriggerDispatchRerun triggers a manual re-election
func (s *RouterService) TriggerDispatchRerun() error {
	s.mu.RLock()
	dispatcher := s.dispatcher
	s.mu.RUnlock()

	if dispatcher == nil {
		return fmt.Errorf("dispatcher is not configured")
	}
	dispatcher.RerunElection()
	return nil
}

// ResetDispatchStats resets dispatch statistics
func (s *RouterService) ResetDispatchStats() error {
	s.mu.RLock()
	dispatcher := s.dispatcher
	s.mu.RUnlock()

	if dispatcher == nil {
		return fmt.Errorf("dispatcher is not configured")
	}
	dispatcher.ResetStats()
	return nil
}
