package upstream

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

func ParseRoutesFromEnv() (map[string][]string, error) {
	raw := strings.TrimSpace(os.Getenv("UPSTREAM_MODEL_ROUTES_JSON"))
	if raw == "" {
		return map[string][]string{}, nil
	}
	var routes map[string][]string
	if err := json.Unmarshal([]byte(raw), &routes); err != nil {
		return nil, fmt.Errorf("invalid UPSTREAM_MODEL_ROUTES_JSON: %w", err)
	}
	for model, list := range routes {
		clean := make([]string, 0, len(list))
		for _, item := range list {
			item = strings.TrimSpace(item)
			if item != "" {
				clean = append(clean, item)
			}
		}
		routes[model] = clean
	}
	return routes, nil
}

func ParseAdaptersFromEnv() ([]Adapter, error) {
	specs, err := ParseAdapterSpecsFromEnv()
	if err != nil {
		return nil, err
	}
	if len(specs) == 0 {
		return nil, nil
	}
	return BuildAdaptersFromSpecs(specs)
}

func ParseDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	if d <= 0 {
		return fallback
	}
	return d
}

func ParseIntEnv(key string, fallback int) int {
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

func ParseBoolEnv(key string, fallback bool) bool {
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

func ParseListEnv(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), fallback...)
	}
	return out
}
