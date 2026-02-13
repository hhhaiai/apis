package scheduler

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func NewFromEnv(adapterNames []string) (*Engine, error) {
	cfg := Config{
		FailureThreshold:   envInt("SCHEDULER_FAILURE_THRESHOLD", 3),
		Cooldown:           envDuration("SCHEDULER_COOLDOWN", 30*time.Second),
		StrictProbeGate:    envBool("SCHEDULER_STRICT_PROBE_GATE", false),
		RequireStreamProbe: envBool("SCHEDULER_REQUIRE_STREAM_PROBE", false),
		RequireToolProbe:   envBool("SCHEDULER_REQUIRE_TOOL_PROBE", false),
	}
	if cfg.FailureThreshold <= 0 {
		return nil, fmt.Errorf("SCHEDULER_FAILURE_THRESHOLD must be > 0")
	}
	if cfg.Cooldown <= 0 {
		return nil, fmt.Errorf("SCHEDULER_COOLDOWN must be > 0")
	}
	return NewEngine(cfg, adapterNames), nil
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	var n int
	_, err := fmt.Sscanf(raw, "%d", &n)
	if err != nil {
		return fallback
	}
	return n
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}
