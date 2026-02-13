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
			Mode:     "server_loop",
			MaxSteps: 5,
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
}
