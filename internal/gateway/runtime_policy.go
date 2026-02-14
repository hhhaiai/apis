package gateway

import (
	"fmt"
	"net/http"
	"strings"
)

func requestMode(r *http.Request, metadata map[string]any) string {
	mode := strings.ToLower(strings.TrimSpace(r.Header.Get("x-cc-mode")))
	if mode != "" {
		return mode
	}
	if metadata != nil {
		if v, ok := metadata["cc_mode"].(string); ok {
			mode = strings.ToLower(strings.TrimSpace(v))
			if mode != "" {
				return mode
			}
		}
	}
	return "chat"
}

func (s *server) resolveModelByMode(mode, requested string) string {
	requested = strings.TrimSpace(requested)
	if s.settings == nil {
		return requested
	}
	return s.settings.ResolveModel(mode, requested)
}

func (s *server) resolveUpstreamModel(mode, clientModel string) (string, string, error) {
	requested := s.resolveModelByMode(mode, clientModel)
	mapped := requested

	if s.settings != nil {
		m, err := s.settings.ResolveModelMapping(requested)
		if err != nil {
			return requested, "", err
		}
		mapped = strings.TrimSpace(m)
	}
	if strings.TrimSpace(mapped) == "" {
		return requested, "", fmt.Errorf("model is required")
	}
	if s.modelMapper != nil {
		finalMapped, err := s.modelMapper.Resolve(mapped)
		if err != nil {
			return requested, "", err
		}
		mapped = finalMapped
	}
	return requested, mapped, nil
}

func (s *server) applySystemPromptPrefix(mode string, system any) any {
	if s.settings == nil {
		return system
	}
	prefix := strings.TrimSpace(s.settings.PromptPrefix(mode))
	if prefix == "" {
		return system
	}
	existing := strings.TrimSpace(systemToText(system))
	if existing == "" {
		return prefix
	}
	return prefix + "\n\n" + existing
}

func (s *server) applyRoutingPolicy(mode string, metadata map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range metadata {
		out[k] = v
	}
	if s.settings == nil {
		return out
	}
	cfg := s.settings.Get()
	out["routing_retries"] = cfg.Routing.Retries
	out["routing_timeout_ms"] = cfg.Routing.TimeoutMS
	out["reflection_passes"] = cfg.Routing.ReflectionPasses
	out["parallel_candidates"] = cfg.Routing.ParallelCandidates
	out["enable_response_judge"] = cfg.Routing.EnableResponseJudge
	out["tool_loop_mode"] = cfg.ToolLoop.Mode
	out["tool_loop_max_steps"] = cfg.ToolLoop.MaxSteps
	if route := s.settings.ModeRoute(mode); len(route) > 0 {
		out["routing_adapter_route"] = route
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func systemToText(system any) string {
	switch s := system.(type) {
	case nil:
		return ""
	case string:
		return s
	case []any:
		var parts []string
		for _, item := range s {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if block["type"] == "text" {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}
