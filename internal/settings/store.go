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
	UseModeModelOverride   bool              `json:"use_mode_model_override"`
	ModeModels             map[string]string `json:"mode_models"`
	ModelMappings          map[string]string `json:"model_mappings"`
	ModelMapStrict         bool              `json:"model_map_strict"`
	ModelMapFallback       string            `json:"model_map_fallback"`
	PromptPrefixes         map[string]string `json:"prompt_prefixes"`
	AllowExperimentalTools bool              `json:"allow_experimental_tools"`
	AllowUnknownTools      bool              `json:"allow_unknown_tools"`
	Routing                RoutingSettings   `json:"routing"`
	ToolLoop               ToolLoopSettings  `json:"tool_loop"`
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
	Mode     string `json:"mode"`
	MaxSteps int    `json:"max_steps"`
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
			Mode:     "client_loop",
			MaxSteps: 4,
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
	var parsed RuntimeSettings
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("invalid RUNTIME_SETTINGS_JSON: %w", err)
	}
	merged := merge(defaults, parsed)
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
	case "", "client_loop", "server_loop":
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
	return out
}

func clone(in RuntimeSettings) RuntimeSettings {
	out := in
	out.ModeModels = copyStringMap(in.ModeModels)
	out.ModelMappings = copyStringMap(in.ModelMappings)
	out.PromptPrefixes = copyStringMap(in.PromptPrefixes)
	out.Routing.ModeRoutes = copyModeRoutes(in.Routing.ModeRoutes)
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
