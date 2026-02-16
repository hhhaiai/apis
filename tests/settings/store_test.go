package settings_test

import (
	. "ccgateway/internal/settings"
	"testing"
)

func TestStoreResolveModel(t *testing.T) {
	s := NewStore(RuntimeSettings{
		UseModeModelOverride: true,
		ModeModels: map[string]string{
			"chat":    "chat-model",
			"plan":    "plan-model",
			"default": "default-model",
		},
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
	})

	if got := s.ResolveModel("plan", "client"); got != "plan-model" {
		t.Fatalf("expected plan-model, got %q", got)
	}
	if got := s.ResolveModel("other", "client"); got != "default-model" {
		t.Fatalf("expected default-model, got %q", got)
	}
}

func TestStorePromptAndRoute(t *testing.T) {
	s := NewStore(RuntimeSettings{
		UseModeModelOverride: false,
		ModeModels:           map[string]string{},
		PromptPrefixes: map[string]string{
			"plan": "PLAN FIRST",
		},
		AllowExperimentalTools: false,
		AllowUnknownTools:      true,
		Routing: RoutingSettings{
			Retries:             2,
			ReflectionPasses:    3,
			TimeoutMS:           12000,
			ParallelCandidates:  2,
			EnableResponseJudge: true,
			ModeRoutes: map[string][]string{
				"plan": []string{"a", "b"},
			},
		},
		ToolLoop: ToolLoopSettings{
			Mode:          "server_loop",
			MaxSteps:      5,
			EmulationMode: "json",
			PlannerModel:  "planner-tools",
		},
	})

	if got := s.PromptPrefix("plan"); got != "PLAN FIRST" {
		t.Fatalf("unexpected prompt prefix: %q", got)
	}
	route := s.ModeRoute("plan")
	if len(route) != 2 || route[0] != "a" {
		t.Fatalf("unexpected mode route: %+v", route)
	}
	cfg := s.Get()
	if cfg.Routing.ParallelCandidates != 2 {
		t.Fatalf("expected parallel candidates=2, got %d", cfg.Routing.ParallelCandidates)
	}
	if !cfg.Routing.EnableResponseJudge {
		t.Fatalf("expected response judge enabled")
	}
	if cfg.ToolLoop.Mode != "server_loop" {
		t.Fatalf("expected server_loop mode, got %q", cfg.ToolLoop.Mode)
	}
	if cfg.ToolLoop.MaxSteps != 5 {
		t.Fatalf("expected max steps=5, got %d", cfg.ToolLoop.MaxSteps)
	}
	if cfg.ToolLoop.EmulationMode != "json" {
		t.Fatalf("expected emulation_mode=json, got %q", cfg.ToolLoop.EmulationMode)
	}
	if cfg.ToolLoop.PlannerModel != "planner-tools" {
		t.Fatalf("expected planner_model=planner-tools, got %q", cfg.ToolLoop.PlannerModel)
	}
}

func TestStoreResolveModelMapping(t *testing.T) {
	s := NewStore(RuntimeSettings{
		ModelMappings: map[string]string{
			"claude-*": "qwen-max",
		},
		ModelMapStrict:   true,
		ModelMapFallback: "fallback-model",
	})

	got, err := s.ResolveModelMapping("claude-3-7-sonnet")
	if err != nil {
		t.Fatalf("resolve model mapping failed: %v", err)
	}
	if got != "qwen-max" {
		t.Fatalf("expected qwen-max, got %q", got)
	}

	got, err = s.ResolveModelMapping("unknown-model")
	if err != nil {
		t.Fatalf("resolve fallback model mapping failed: %v", err)
	}
	if got != "fallback-model" {
		t.Fatalf("expected fallback-model, got %q", got)
	}
}

func TestStoreResolveVisionSupport(t *testing.T) {
	s := NewStore(RuntimeSettings{
		VisionSupportHints: map[string]bool{
			"gpt-3.5-*": false,
			"gpt-4o":    true,
		},
	})
	if supported, known := s.ResolveVisionSupport("gpt-3.5-turbo"); !known || supported {
		t.Fatalf("expected gpt-3.5-turbo known unsupported, got known=%v supported=%v", known, supported)
	}
	if supported, known := s.ResolveVisionSupport("gpt-4o"); !known || !supported {
		t.Fatalf("expected gpt-4o known supported, got known=%v supported=%v", known, supported)
	}
	if _, known := s.ResolveVisionSupport("unknown-model"); known {
		t.Fatalf("expected unknown-model not found")
	}
}

func TestStoreToolAliasesSanitizeAndClone(t *testing.T) {
	s := NewStore(RuntimeSettings{
		ToolAliases: map[string]string{
			" read_file ": " file_read ",
			"":            "ignored",
		},
	})
	cfg := s.Get()
	if got := cfg.ToolAliases["read_file"]; got != "file_read" {
		t.Fatalf("expected trimmed tool alias read_file=file_read, got %q", got)
	}
	if _, ok := cfg.ToolAliases[""]; ok {
		t.Fatalf("expected empty alias key to be dropped")
	}

	// Ensure Get returns a clone, not internal map reference.
	cfg.ToolAliases["read_file"] = "mutated"
	cfg2 := s.Get()
	if got := cfg2.ToolAliases["read_file"]; got != "file_read" {
		t.Fatalf("expected stored alias to remain file_read, got %q", got)
	}
}

func TestStoreToolLoopSanitize(t *testing.T) {
	s := NewStore(RuntimeSettings{
		ToolLoop: ToolLoopSettings{
			Mode:          "invalid-mode",
			MaxSteps:      0,
			EmulationMode: "invalid-emulation",
			PlannerModel:  "  planner-a  ",
		},
	})
	cfg := s.Get()
	if cfg.ToolLoop.Mode != "client_loop" {
		t.Fatalf("expected default tool loop mode client_loop, got %q", cfg.ToolLoop.Mode)
	}
	if cfg.ToolLoop.MaxSteps != 4 {
		t.Fatalf("expected default max steps 4, got %d", cfg.ToolLoop.MaxSteps)
	}
	if cfg.ToolLoop.EmulationMode != "native" {
		t.Fatalf("expected default emulation mode native, got %q", cfg.ToolLoop.EmulationMode)
	}
	if cfg.ToolLoop.PlannerModel != "planner-a" {
		t.Fatalf("expected planner model trimmed, got %q", cfg.ToolLoop.PlannerModel)
	}
}

func TestIntelligentDispatchDefault(t *testing.T) {
	cfg := DefaultRuntimeSettings()

	// Verify intelligent dispatch is enabled by default
	if !cfg.IntelligentDispatch.Enabled {
		t.Error("expected intelligent dispatch to be enabled by default")
	}
	if cfg.IntelligentDispatch.MinScoreDifference != 5.0 {
		t.Errorf("expected min score difference 5.0, got %f", cfg.IntelligentDispatch.MinScoreDifference)
	}
	if cfg.IntelligentDispatch.ReElectIntervalMS != 600000 {
		t.Errorf("expected re-elect interval 600000ms, got %d", cfg.IntelligentDispatch.ReElectIntervalMS)
	}
}

func TestIntelligentDispatchMerge(t *testing.T) {
	// Test merge with custom values via Store
	s := NewStore(RuntimeSettings{
		IntelligentDispatch: IntelligentDispatchSettings{
			Enabled:            false,
			MinScoreDifference: 15.0,
			ReElectIntervalMS:  900000,
		},
	})
	merged := s.Get()

	if merged.IntelligentDispatch.Enabled != false {
		t.Error("expected enabled to be false after merge")
	}
	if merged.IntelligentDispatch.MinScoreDifference != 15.0 {
		t.Errorf("expected min score difference 15.0, got %f", merged.IntelligentDispatch.MinScoreDifference)
	}
	if merged.IntelligentDispatch.ReElectIntervalMS != 900000 {
		t.Errorf("expected re-elect interval 900000ms, got %d", merged.IntelligentDispatch.ReElectIntervalMS)
	}
}

func TestIntelligentDispatchSanitize(t *testing.T) {
	// Test sanitization of invalid values
	s := NewStore(RuntimeSettings{
		IntelligentDispatch: IntelligentDispatchSettings{
			Enabled:            true,
			MinScoreDifference: -1, // invalid - should be sanitized to 5.0
			ReElectIntervalMS:  0,  // invalid - should be sanitized to 600000
		},
	})
	sanitized := s.Get()

	if sanitized.IntelligentDispatch.MinScoreDifference != 5.0 {
		t.Errorf("expected sanitized min score difference 5.0, got %f", sanitized.IntelligentDispatch.MinScoreDifference)
	}
	if sanitized.IntelligentDispatch.ReElectIntervalMS != 600000 {
		t.Errorf("expected sanitized re-elect interval 600000ms, got %d", sanitized.IntelligentDispatch.ReElectIntervalMS)
	}
}

func TestIntelligentDispatchModelPolicies(t *testing.T) {
	// Test model policies configuration
	s := NewStore(RuntimeSettings{
		IntelligentDispatch: IntelligentDispatchSettings{
			Enabled: true,
			ModelPolicies: map[string]ModelDispatchPolicy{
				"gpt-4*": {
					PreferredAdapter: "openai-adapter",
					ForceScheduler:   true,
					ComplexityLevel:  "high",
				},
				"claude-*": {
					PreferredAdapter: "anthropic-adapter",
					ForceScheduler:   false,
					ComplexityLevel:  "auto",
				},
			},
		},
	})
	cfg := s.Get()

	if len(cfg.IntelligentDispatch.ModelPolicies) != 2 {
		t.Errorf("expected 2 model policies, got %d", len(cfg.IntelligentDispatch.ModelPolicies))
	}

	if cfg.IntelligentDispatch.ModelPolicies["gpt-4*"].PreferredAdapter != "openai-adapter" {
		t.Error("expected gpt-4* to use openai-adapter")
	}
	if !cfg.IntelligentDispatch.ModelPolicies["gpt-4*"].ForceScheduler {
		t.Error("expected gpt-4* to force scheduler")
	}
}

func TestIntelligentDispatchComplexityThresholds(t *testing.T) {
	// Test complexity thresholds configuration
	s := NewStore(RuntimeSettings{
		IntelligentDispatch: IntelligentDispatchSettings{
			Enabled: true,
			ComplexityThresholds: ComplexityThresholds{
				LongContextChars:   8000,
				ToolCountThreshold: 3,
			},
		},
	})
	cfg := s.Get()

	if cfg.IntelligentDispatch.ComplexityThresholds.LongContextChars != 8000 {
		t.Errorf("expected long context chars 8000, got %d", cfg.IntelligentDispatch.ComplexityThresholds.LongContextChars)
	}
	if cfg.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold != 3 {
		t.Errorf("expected tool count threshold 3, got %d", cfg.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold)
	}
}

func TestIntelligentDispatchDefaults(t *testing.T) {
	cfg := DefaultRuntimeSettings()

	// Verify default thresholds
	if cfg.IntelligentDispatch.ComplexityThresholds.LongContextChars != 4000 {
		t.Errorf("expected default long context 4000, got %d", cfg.IntelligentDispatch.ComplexityThresholds.LongContextChars)
	}
	if cfg.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold != 1 {
		t.Errorf("expected default tool count threshold 1, got %d", cfg.IntelligentDispatch.ComplexityThresholds.ToolCountThreshold)
	}
	if cfg.IntelligentDispatch.FallbackToScheduler != true {
		t.Error("expected fallback to scheduler to be true by default")
	}
}

func TestNewFromEnvPreservesIntelligentDispatchBoolDefaultsWhenMissing(t *testing.T) {
	t.Setenv("RUNTIME_SETTINGS_JSON", `{
		"intelligent_dispatch":{
			"min_score_difference":12.5
		}
	}`)
	store, err := NewFromEnv()
	if err != nil {
		t.Fatalf("new from env: %v", err)
	}
	cfg := store.Get()
	if !cfg.IntelligentDispatch.Enabled {
		t.Fatalf("expected enabled=true when env key is missing")
	}
	if !cfg.IntelligentDispatch.FallbackToScheduler {
		t.Fatalf("expected fallback_to_scheduler=true when env key is missing")
	}
	if cfg.IntelligentDispatch.MinScoreDifference != 12.5 {
		t.Fatalf("expected min_score_difference=12.5, got %f", cfg.IntelligentDispatch.MinScoreDifference)
	}
}

func TestNewFromEnvRespectsExplicitIntelligentDispatchBoolOverrides(t *testing.T) {
	t.Setenv("RUNTIME_SETTINGS_JSON", `{
		"intelligent_dispatch":{
			"enabled":false,
			"fallback_to_scheduler":false
		}
	}`)
	store, err := NewFromEnv()
	if err != nil {
		t.Fatalf("new from env: %v", err)
	}
	cfg := store.Get()
	if cfg.IntelligentDispatch.Enabled {
		t.Fatalf("expected enabled=false from explicit env override")
	}
	if cfg.IntelligentDispatch.FallbackToScheduler {
		t.Fatalf("expected fallback_to_scheduler=false from explicit env override")
	}
}
