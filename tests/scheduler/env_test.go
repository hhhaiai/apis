package scheduler_test

import (
	. "ccgateway/internal/scheduler"
	"testing"

	"ccgateway/internal/orchestrator"
)

func TestNewFromEnv(t *testing.T) {
	t.Setenv("SCHEDULER_FAILURE_THRESHOLD", "4")
	t.Setenv("SCHEDULER_COOLDOWN", "45s")
	t.Setenv("SCHEDULER_STRICT_PROBE_GATE", "true")
	t.Setenv("SCHEDULER_REQUIRE_STREAM_PROBE", "true")
	t.Setenv("SCHEDULER_REQUIRE_TOOL_PROBE", "false")

	engine, err := NewFromEnv([]string{"a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ordered := engine.Order(orchestrator.Request{Model: "m1"}, []string{"a1"}, true)
	if len(ordered) != 1 || ordered[0] != "a1" {
		t.Fatalf("unexpected order result: %v", ordered)
	}
}
