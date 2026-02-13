package probe_test

import (
	. "ccgateway/internal/probe"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("PROBE_ENABLED", "true")
	t.Setenv("PROBE_INTERVAL", "30s")
	t.Setenv("PROBE_TIMEOUT", "5s")
	t.Setenv("PROBE_MODELS", "m1,m2")
	t.Setenv("PROBE_MODELS_JSON", `{"a1":["x1","x2"]}`)
	t.Setenv("PROBE_STREAM_SMOKE", "true")
	t.Setenv("PROBE_TOOL_SMOKE", "false")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Enabled {
		t.Fatalf("expected enabled=true")
	}
	if cfg.Interval.String() != "30s" {
		t.Fatalf("expected 30s interval, got %v", cfg.Interval)
	}
	if len(cfg.DefaultModels) != 2 {
		t.Fatalf("expected 2 default models, got %d", len(cfg.DefaultModels))
	}
	if len(cfg.ModelsByAdapter["a1"]) != 2 {
		t.Fatalf("expected 2 adapter models, got %+v", cfg.ModelsByAdapter["a1"])
	}
	if cfg.ToolSmoke {
		t.Fatalf("expected tool smoke disabled")
	}
}
