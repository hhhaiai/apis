package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
)

type RuntimeSettings struct {
	UseModeModelOverride   bool                        `json:"use_mode_model_override"`
	ModeModels             map[string]string           `json:"mode_models"`
	ModelMappings          map[string]string           `json:"model_mappings"`
	ModelMapStrict         bool                        `json:"model_map_strict"`
	ModelMapFallback       string                      `json:"model_map_fallback"`
	VisionSupportHints     map[string]bool             `json:"vision_support_hints"`
	ToolAliases            map[string]string           `json:"tool_aliases"`
	PromptPrefixes         map[string]string           `json:"prompt_prefixes"`
	AllowExperimentalTools bool                        `json:"allow_experimental_tools"`
	AllowUnknownTools      bool                        `json:"allow_unknown_tools"`
	Routing                RoutingSettings             `json:"routing"`
	ToolLoop               ToolLoopSettings            `json:"tool_loop"`
	IntelligentDispatch    IntelligentDispatchSettings `json:"intelligent_dispatch"`
}

type RoutingSettings struct {
	Retries             int                 `json:"retries"`
	ReflectionPasses    int                 `json:"reflection_passes"`
	TimeoutMS           int                 `json:"timeout_ms"`
	ParallelCandidates  int                 `json:"parallel_candidates"`
	EnableResponseJudge bool                `json:"enable_response_judge"`
	ModeRoutes          map[string][]string `json:"mode_routes"`
}

type ToolLoopSettings struct {
	Mode          string `json:"mode"`
	MaxSteps      int    `json:"max_steps"`
	EmulationMode string `json:"emulation_mode"`
	PlannerModel  string `json:"planner_model"`
}

// IntelligentDispatchSettings 智能调度设置
type IntelligentDispatchSettings struct {
	Enabled              bool                           `json:"enabled"`               // 默认启用
	MinScoreDifference   float64                        `json:"min_score_difference"`  // 选举最小分数差
	ReElectIntervalMS    int64                          `json:"re_elect_interval_ms"`  // 重新选举间隔(毫秒)
	FallbackToScheduler  bool                           `json:"fallback_to_scheduler"` // 失败时回退到调度器
	ModelPolicies        map[string]ModelDispatchPolicy `json:"model_policies"`        // 按模型配置调度策略
	ComplexityThresholds ComplexityThresholds           `json:"complexity_thresholds"` // 复杂度阈值
}

// ModelDispatchPolicy 模型调度策略
type ModelDispatchPolicy struct {
	PreferredAdapter string `json:"preferred_adapter"` // 强制使用某适配器
	ForceScheduler   bool   `json:"force_scheduler"`   // 强制走调度器
	ComplexityLevel  string `json:"complexity_level"`  // low/medium/high/auto
}

// ComplexityThresholds 复杂度阈值配置
type ComplexityThresholds struct {
	LongContextChars   int `json:"long_context_chars"`   // 长上下文阈值默认4000
	ToolCountThreshold int `json:"tool_count_threshold"` // 工具数量阈值默认1
}

type Store struct {
	mu   sync.RWMutex
	data RuntimeSettings
}

func DefaultRuntimeSettings() RuntimeSettings {
	return RuntimeSettings{
		UseModeModelOverride:   false,
		ModeModels:             map[string]string{},
		ModelMappings:          map[string]string{},
		ModelMapStrict:         false,
		ModelMapFallback:       "",
		VisionSupportHints:     map[string]bool{},
		ToolAliases:            map[string]string{},
		PromptPrefixes:         map[string]string{},
		AllowExperimentalTools: false,
		AllowUnknownTools:      true,
		Routing: RoutingSettings{
			Retries:             1,
			ReflectionPasses:    1,
			TimeoutMS:           30000,
			ParallelCandidates:  1,
			EnableResponseJudge: false,
			ModeRoutes:          map[string][]string{},
		},
		ToolLoop: ToolLoopSettings{
			Mode:          "client_loop",
			MaxSteps:      4,
			EmulationMode: "native",
			PlannerModel:  "",
		},
		IntelligentDispatch: IntelligentDispatchSettings{
			Enabled:             true, // 默认启用智能调度
			MinScoreDifference:  5.0,
			ReElectIntervalMS:   600000, // 10分钟
			FallbackToScheduler: true,   // 失败时回退到调度器
			ModelPolicies:       map[string]ModelDispatchPolicy{},
			ComplexityThresholds: ComplexityThresholds{
				LongContextChars:   4000,
				ToolCountThreshold: 1,
			},
		},
	}
}

func NewStore(initial RuntimeSettings) *Store {
	fixed := sanitize(initial)
	return &Store{data: fixed}
}

func NewFromEnv() (*Store, error) {
	defaults := DefaultRuntimeSettings()
	raw := strings.TrimSpace(os.Getenv("RUNTIME_SETTINGS_JSON"))
	if raw == "" {
		return NewStore(defaults), nil
	}
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rawMap); err != nil {
		return nil, fmt.Errorf("invalid RUNTIME_SETTINGS_JSON: %w", err)
	}
	var parsed RuntimeSettings
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("invalid RUNTIME_SETTINGS_JSON: %w", err)
	}
	merged := merge(defaults, parsed)
	if dispatchRaw, ok := rawMap["intelligent_dispatch"]; ok {
		var dispatchMap map[string]json.RawMessage
		if err := json.Unmarshal(dispatchRaw, &dispatchMap); err == nil {
			if _, hasEnabled := dispatchMap["enabled"]; !hasEnabled {
				merged.IntelligentDispatch.Enabled = defaults.IntelligentDispatch.Enabled
			}
			if _, hasFallback := dispatchMap["fallback_to_scheduler"]; !hasFallback {
				merged.IntelligentDispatch.FallbackToScheduler = defaults.IntelligentDispatch.FallbackToScheduler
			}
		}
	} else {
		merged.IntelligentDispatch.Enabled = defaults.IntelligentDispatch.Enabled
		merged.IntelligentDispatch.FallbackToScheduler = defaults.IntelligentDispatch.FallbackToScheduler
	}
	return NewStore(merged), nil
}

func (s *Store) Get() RuntimeSettings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.data)
}

func (s *Store) Put(in RuntimeSettings) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = sanitize(in)
}

func (s *Store) ResolveModel(mode, requestedModel string) string {
	mode = normalizeMode(mode)
	requestedModel = strings.TrimSpace(requestedModel)
	cfg := s.Get()
	if !cfg.UseModeModelOverride {
		return requestedModel
	}
	if cfg.ModeModels != nil {
		if m := strings.TrimSpace(cfg.ModeModels[mode]); m != "" {
			return m
		}
		if m := strings.TrimSpace(cfg.ModeModels["default"]); m != "" {
			return m
		}
	}
	return requestedModel
}

func (s *Store) ResolveModelMapping(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("model is required")
	}
	cfg := s.Get()
	if target, ok := cfg.ModelMappings[model]; ok && strings.TrimSpace(target) != "" {
		return strings.TrimSpace(target), nil
	}
	for pattern, target := range cfg.ModelMappings {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || !strings.Contains(pattern, "*") {
			continue
		}
		matched, err := path.Match(pattern, model)
		if err != nil || !matched {
			continue
		}
		target = strings.TrimSpace(target)
		if target != "" {
			return target, nil
		}
	}
	if fb := strings.TrimSpace(cfg.ModelMapFallback); fb != "" {
		return fb, nil
	}
	if cfg.ModelMapStrict {
		return "", fmt.Errorf("model %q is not mapped", model)
	}
	return model, nil
}

func (s *Store) ResolveVisionSupport(model string) (supported bool, known bool) {
	model = strings.TrimSpace(model)
	if model == "" {
		return false, false
	}
	cfg := s.Get()
	if len(cfg.VisionSupportHints) == 0 {
		return false, false
	}
	if v, ok := cfg.VisionSupportHints[model]; ok {
		return v, true
	}
	for pattern, v := range cfg.VisionSupportHints {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || !strings.Contains(pattern, "*") {
			continue
		}
		matched, err := path.Match(pattern, model)
		if err != nil || !matched {
			continue
		}
		return v, true
	}
	return false, false
}

func (s *Store) PromptPrefix(mode string) string {
	mode = normalizeMode(mode)
	cfg := s.Get()
	if cfg.PromptPrefixes == nil {
		return ""
	}
	if p := strings.TrimSpace(cfg.PromptPrefixes[mode]); p != "" {
		return p
	}
	return strings.TrimSpace(cfg.PromptPrefixes["default"])
}

func (s *Store) ModeRoute(mode string) []string {
	mode = normalizeMode(mode)
	cfg := s.Get()
	if cfg.Routing.ModeRoutes == nil {
		return nil
	}
	if route := cfg.Routing.ModeRoutes[mode]; len(route) > 0 {
		return append([]string(nil), route...)
	}
	if route := cfg.Routing.ModeRoutes["default"]; len(route) > 0 {
		return append([]string(nil), route...)
	}
	return nil
}

func normalizeMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return "chat"
	}
	return mode
}

func merge(defaults, in RuntimeSettings) RuntimeSettings {
	out := defaults
	if in.ModeModels != nil {
		out.ModeModels = copyStringMap(in.ModeModels)
	}
	if in.ModelMappings != nil {
		out.ModelMappings = copyStringMap(in.ModelMappings)
	}
	if in.VisionSupportHints != nil {
		out.VisionSupportHints = copyBoolMap(in.VisionSupportHints)
	}
	if in.ToolAliases != nil {
		out.ToolAliases = copyStringMap(in.ToolAliases)
	}
	if in.PromptPrefixes != nil {
		out.PromptPrefixes = copyStringMap(in.PromptPrefixes)
	}
	if in.Routing.ModeRoutes != nil {
		out.Routing.ModeRoutes = copyModeRoutes(in.Routing.ModeRoutes)
	}
	out.UseModeModelOverride = in.UseModeModelOverride
	out.ModelMapStrict = in.ModelMapStrict
	out.ModelMapFallback = strings.TrimSpace(in.ModelMapFallback)
	out.AllowExperimentalTools = in.AllowExperimentalTools
	out.AllowUnknownTools = in.AllowUnknownTools
	if in.Routing.Retries != 0 {
		out.Routing.Retries = in.Routing.Retries
	}
	if in.Routing.ReflectionPasses != 0 {
		out.Routing.ReflectionPasses = in.Routing.ReflectionPasses
	}
	if in.Routing.TimeoutMS != 0 {
		out.Routing.TimeoutMS = in.Routing.TimeoutMS
	}
	if in.Routing.ParallelCandidates != 0 {
		out.Routing.ParallelCandidates = in.Routing.ParallelCandidates
	}
	out.Routing.EnableResponseJudge = in.Routing.EnableResponseJudge
	if strings.TrimSpace(in.ToolLoop.Mode) != "" {
		out.ToolLoop.Mode = strings.TrimSpace(in.ToolLoop.Mode)
	}
	if in.ToolLoop.MaxSteps != 0 {
		out.ToolLoop.MaxSteps = in.ToolLoop.MaxSteps
	}
	if strings.TrimSpace(in.ToolLoop.EmulationMode) != "" {
		out.ToolLoop.EmulationMode = strings.TrimSpace(in.ToolLoop.EmulationMode)
	}
	if strings.TrimSpace(in.ToolLoop.PlannerModel) != "" {
		out.ToolLoop.PlannerModel = strings.TrimSpace(in.ToolLoop.PlannerModel)
	}
	// IntelligentDispatch settings - allow explicit false to disable
	out.IntelligentDispatch.Enabled = in.IntelligentDispatch.Enabled
	if in.IntelligentDispatch.MinScoreDifference > 0 {
		out.IntelligentDispatch.MinScoreDifference = in.IntelligentDispatch.MinScoreDifference
	}
	if in.IntelligentDispatch.ReElectIntervalMS > 0 {
		out.IntelligentDispatch.ReElectIntervalMS = in.IntelligentDispatch.ReElectIntervalMS
	}
	// FallbackToScheduler - only set if explicitly provided (not zero value)
	out.IntelligentDispatch.FallbackToScheduler = in.IntelligentDispatch.FallbackToScheduler
	// Model policies
	if in.IntelligentDispatch.ModelPolicies != nil {
		out.IntelligentDispatch.ModelPolicies = copyModelPolicies(in.IntelligentDispatch.ModelPolicies)
	}
	// Complexity thresholds
	if in.IntelligentDispatch.ComplexityThresholds.LongContextChars > 0 {
		out.IntelligentDispatch.ComplexityThresholds.LongContextChars = in.IntelligentDispatch.ComplexityThresholds.LongContextChars
	}
	if in.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold > 0 {
		out.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold = in.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold
	}
	return sanitize(out)
}

func sanitize(in RuntimeSettings) RuntimeSettings {
	out := clone(in)
	if out.ModeModels == nil {
		out.ModeModels = map[string]string{}
	}
	if out.ModelMappings == nil {
		out.ModelMappings = map[string]string{}
	}
	if out.VisionSupportHints == nil {
		out.VisionSupportHints = map[string]bool{}
	}
	if out.ToolAliases == nil {
		out.ToolAliases = map[string]string{}
	}
	out.ModelMapFallback = strings.TrimSpace(out.ModelMapFallback)
	if out.PromptPrefixes == nil {
		out.PromptPrefixes = map[string]string{}
	}
	if out.Routing.ModeRoutes == nil {
		out.Routing.ModeRoutes = map[string][]string{}
	}
	if out.Routing.Retries < 0 {
		out.Routing.Retries = 0
	}
	if out.Routing.ReflectionPasses < 0 {
		out.Routing.ReflectionPasses = 0
	}
	if out.Routing.TimeoutMS <= 0 {
		out.Routing.TimeoutMS = 30000
	}
	if out.Routing.ParallelCandidates <= 0 {
		out.Routing.ParallelCandidates = 1
	}
	mode := strings.ToLower(strings.TrimSpace(out.ToolLoop.Mode))
	switch mode {
	case "", "client_loop", "server_loop", "server", "native", "react", "json", "hybrid":
		if mode == "" {
			mode = "client_loop"
		}
		out.ToolLoop.Mode = mode
	default:
		out.ToolLoop.Mode = "client_loop"
	}
	if out.ToolLoop.MaxSteps <= 0 {
		out.ToolLoop.MaxSteps = 4
	}
	emuMode := strings.ToLower(strings.TrimSpace(out.ToolLoop.EmulationMode))
	switch emuMode {
	case "", "native", "react", "json", "hybrid":
		if emuMode == "" {
			emuMode = "native"
		}
		out.ToolLoop.EmulationMode = emuMode
	default:
		out.ToolLoop.EmulationMode = "native"
	}
	out.ToolLoop.PlannerModel = strings.TrimSpace(out.ToolLoop.PlannerModel)
	// IntelligentDispatch validation
	if out.IntelligentDispatch.MinScoreDifference <= 0 {
		out.IntelligentDispatch.MinScoreDifference = 5.0
	}
	if out.IntelligentDispatch.ReElectIntervalMS <= 0 {
		out.IntelligentDispatch.ReElectIntervalMS = 600000 // 10分钟
	}
	if out.IntelligentDispatch.ModelPolicies == nil {
		out.IntelligentDispatch.ModelPolicies = map[string]ModelDispatchPolicy{}
	}
	if out.IntelligentDispatch.ComplexityThresholds.LongContextChars <= 0 {
		out.IntelligentDispatch.ComplexityThresholds.LongContextChars = 4000
	}
	if out.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold <= 0 {
		out.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold = 1
	}
	return out
}

func clone(in RuntimeSettings) RuntimeSettings {
	out := in
	out.ModeModels = copyStringMap(in.ModeModels)
	out.ModelMappings = copyStringMap(in.ModelMappings)
	out.VisionSupportHints = copyBoolMap(in.VisionSupportHints)
	out.ToolAliases = copyStringMap(in.ToolAliases)
	out.PromptPrefixes = copyStringMap(in.PromptPrefixes)
	out.Routing.ModeRoutes = copyModeRoutes(in.Routing.ModeRoutes)
	out.IntelligentDispatch.ModelPolicies = copyModelPolicies(in.IntelligentDispatch.ModelPolicies)
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func copyModeRoutes(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return map[string][]string{}
	}
	out := make(map[string][]string, len(in))
	for k, route := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		clean := make([]string, 0, len(route))
		for _, item := range route {
			item = strings.TrimSpace(item)
			if item != "" {
				clean = append(clean, item)
			}
		}
		out[k] = clean
	}
	return out
}

func copyBoolMap(in map[string]bool) map[string]bool {
	if len(in) == 0 {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	return out
}

func copyModelPolicies(in map[string]ModelDispatchPolicy) map[string]ModelDispatchPolicy {
	if len(in) == 0 {
		return map[string]ModelDispatchPolicy{}
	}
	out := make(map[string]ModelDispatchPolicy, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = ModelDispatchPolicy{
			PreferredAdapter: strings.TrimSpace(v.PreferredAdapter),
			ForceScheduler:   v.ForceScheduler,
			ComplexityLevel:  strings.TrimSpace(v.ComplexityLevel),
		}
	}
	return out
}
