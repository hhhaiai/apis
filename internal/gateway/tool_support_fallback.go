package gateway

import (
	"path"
	"sort"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/upstream"
)

func (s *server) applyToolSupportFallback(req orchestrator.Request) orchestrator.Request {
	if len(req.Tools) == 0 {
		return req
	}

	mode := strings.ToLower(strings.TrimSpace(stringFromAny(req.Metadata["tool_fallback_mode"])))
	switch mode {
	case "off", "disabled", "none":
		return req
	}

	force := mode == "force" || mode == "on" || mode == "always"
	if !force && hasServerToolLoopMode(stringFromAny(req.Metadata["tool_loop_mode"])) {
		return req
	}

	supported, known := s.resolveToolsSupport(req)
	if !force {
		if !known || supported {
			return req
		}
	}

	out := req
	meta := map[string]any{}
	for k, v := range req.Metadata {
		meta[k] = v
	}
	if !hasServerToolLoopMode(stringFromAny(meta["tool_loop_mode"])) {
		meta["tool_loop_mode"] = "server_loop"
	}
	emu := strings.ToLower(strings.TrimSpace(stringFromAny(meta["tool_emulation_mode"])))
	if emu == "" || emu == "native" {
		meta["tool_emulation_mode"] = "hybrid"
	}

	reason := "forced"
	if !force {
		reason = "upstream_tools_unsupported"
	}
	meta["tool_fallback_applied"] = true
	meta["tool_fallback_reason"] = reason
	out.Metadata = meta
	s.appendToolFallbackEvent(out, reason)
	return out
}

func hasServerToolLoopMode(mode string) bool {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "server", "server_loop", "native", "json", "react", "hybrid":
		return true
	default:
		return false
	}
}

func (s *server) resolveToolsSupport(req orchestrator.Request) (supported bool, known bool) {
	if v, ok := boolFromAny(req.Metadata["upstream_supports_tools"]); ok {
		return v, true
	}
	support := strings.ToLower(strings.TrimSpace(stringFromAny(req.Metadata["upstream_tool_support"])))
	switch support {
	case "supported", "yes", "true":
		return true, true
	case "unsupported", "no", "false":
		return false, true
	}
	if supported, known := s.resolveToolSupportFromUpstream(req); known {
		return supported, true
	}
	return false, false
}

func (s *server) resolveToolSupportFromUpstream(req orchestrator.Request) (supported bool, known bool) {
	type upstreamConfigProvider interface {
		GetUpstreamConfig() upstream.UpstreamAdminConfig
	}
	provider, ok := s.orchestrator.(upstreamConfigProvider)
	if !ok {
		return false, false
	}
	cfg := provider.GetUpstreamConfig()
	if len(cfg.Adapters) == 0 {
		return false, false
	}

	route := routeFromMetadataLocal(req.Metadata)
	if len(route) == 0 {
		route = resolveRouteByModel(cfg, req.Model)
	}
	if len(route) == 0 {
		return false, false
	}

	specByName := make(map[string]upstream.AdapterSpec, len(cfg.Adapters))
	for _, spec := range cfg.Adapters {
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			continue
		}
		specByName[name] = spec
	}

	anyKnown := false
	anySupported := false
	anyUnsupported := false
	for _, name := range route {
		spec, ok := specByName[strings.TrimSpace(name)]
		if !ok || spec.SupportsTools == nil {
			continue
		}
		anyKnown = true
		if *spec.SupportsTools {
			anySupported = true
		} else {
			anyUnsupported = true
		}
	}
	if anySupported {
		return true, true
	}
	if anyKnown && anyUnsupported {
		return false, true
	}
	return false, false
}

func (s *server) appendToolFallbackEvent(req orchestrator.Request, reason string) {
	sessionID := ""
	mode := ""
	pathValue := ""
	if req.Metadata != nil {
		sessionID = stringFromAny(req.Metadata["session_id"])
		mode = stringFromAny(req.Metadata["mode"])
		pathValue = stringFromAny(req.Metadata["request_path"])
	}
	if strings.TrimSpace(pathValue) == "" {
		pathValue = "/v1/messages"
	}

	tools := make([]string, 0, len(req.Tools))
	for _, tool := range req.Tools {
		name := strings.TrimSpace(tool.Name)
		if name != "" {
			tools = append(tools, name)
		}
	}
	sort.Strings(tools)

	s.appendEvent(ccevent.AppendInput{
		EventType: "tool.fallback_applied",
		SessionID: sessionID,
		RunID:     req.RunID,
		Data: map[string]any{
			"path":   path.Clean(pathValue),
			"mode":   mode,
			"reason": strings.TrimSpace(reason),
			"count":  len(tools),
			"tools":  tools,
		},
	})
}
