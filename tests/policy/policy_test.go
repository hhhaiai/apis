package policy_test

import (
	. "ccgateway/internal/policy"
	"context"
	"testing"

	"ccgateway/internal/settings"
	"ccgateway/internal/toolcatalog"
)

func TestDynamicEngineToolPolicy(t *testing.T) {
	st := settings.NewStore(settings.RuntimeSettings{
		UseModeModelOverride:   false,
		ModeModels:             map[string]string{},
		PromptPrefixes:         map[string]string{},
		AllowExperimentalTools: false,
		AllowUnknownTools:      false,
		Routing: settings.RoutingSettings{
			Retries:          1,
			ReflectionPasses: 1,
			TimeoutMS:        30000,
			ModeRoutes:       map[string][]string{},
		},
	})
	catalog := toolcatalog.NewCatalog([]toolcatalog.ToolSpec{
		{Name: "safe_tool", Status: toolcatalog.StatusSupported},
		{Name: "beta_tool", Status: toolcatalog.StatusExperimental},
	})
	engine := NewDynamicEngine(st, catalog)

	if err := engine.Authorize(context.Background(), Action{
		Path:      "/v1/messages",
		Model:     "x",
		ToolNames: []string{"safe_tool"},
	}); err != nil {
		t.Fatalf("supported tool should pass: %v", err)
	}

	if err := engine.Authorize(context.Background(), Action{
		Path:      "/v1/messages",
		Model:     "x",
		ToolNames: []string{"beta_tool"},
	}); err == nil {
		t.Fatalf("experimental tool should fail when disabled")
	}

	st.Put(settings.RuntimeSettings{
		UseModeModelOverride:   false,
		ModeModels:             map[string]string{},
		PromptPrefixes:         map[string]string{},
		AllowExperimentalTools: true,
		AllowUnknownTools:      true,
		Routing: settings.RoutingSettings{
			Retries:          1,
			ReflectionPasses: 1,
			TimeoutMS:        30000,
			ModeRoutes:       map[string][]string{},
		},
	})
	if err := engine.Authorize(context.Background(), Action{
		Path:      "/v1/messages",
		Model:     "x",
		ToolNames: []string{"beta_tool"},
	}); err != nil {
		t.Fatalf("experimental tool should pass after enabling: %v", err)
	}
}
