package probe

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Enabled:     envBool("PROBE_ENABLED", true),
		Interval:    envDuration("PROBE_INTERVAL", 45*time.Second),
		Timeout:     envDuration("PROBE_TIMEOUT", 8*time.Second),
		StreamSmoke: envBool("PROBE_STREAM_SMOKE", true),
		ToolSmoke:   envBool("PROBE_TOOL_SMOKE", true),
	}
	cfg.DefaultModels = parseListEnv("PROBE_MODELS")
	modelMapRaw := strings.TrimSpace(os.Getenv("PROBE_MODELS_JSON"))
	if modelMapRaw != "" {
		var modelMap map[string][]string
		if err := json.Unmarshal([]byte(modelMapRaw), &modelMap); err != nil {
			return Config{}, fmt.Errorf("invalid PROBE_MODELS_JSON: %w", err)
		}
		cfg.ModelsByAdapter = sanitizeModelMap(modelMap)
	} else {
		cfg.ModelsByAdapter = map[string][]string{}
	}
	return cfg, nil
}

func sanitizeModelMap(in map[string][]string) map[string][]string {
	out := map[string][]string{}
	for k, models := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		clean := make([]string, 0, len(models))
		for _, m := range models {
			m = strings.TrimSpace(m)
			if m != "" {
				clean = append(clean, m)
			}
		}
		out[k] = clean
	}
	return out
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

func parseListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
