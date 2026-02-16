package gateway

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"

	"ccgateway/internal/ccevent"
	"ccgateway/internal/orchestrator"
	"ccgateway/internal/toolruntime"
	"ccgateway/internal/upstream"
)

type visionMessageRefs struct {
	Index int
	URLs  []string
}

func (s *server) applyVisionFallback(ctx context.Context, req orchestrator.Request) orchestrator.Request {
	if !s.shouldApplyVisionFallback(req) {
		return req
	}

	refs := collectVisionMessageRefs(req.Messages)
	if len(refs) == 0 {
		return req
	}

	out := req
	out.Messages = append([]orchestrator.Message(nil), req.Messages...)
	usedURLs := make([]string, 0, 8)
	summaries := map[string]string{}

	for _, ref := range refs {
		report := s.buildVisionReport(ctx, ref.URLs, summaries, &usedURLs)
		if strings.TrimSpace(report) == "" {
			continue
		}
		msg := out.Messages[ref.Index]
		msg.Content = stripImagesAndAppendVisionReport(msg.Content, report)
		out.Messages[ref.Index] = msg
	}

	if len(usedURLs) == 0 {
		return out
	}

	meta := map[string]any{}
	for k, v := range out.Metadata {
		meta[k] = v
	}
	meta["vision_fallback_applied"] = true
	meta["vision_fallback_image_count"] = len(usedURLs)
	out.Metadata = meta

	s.appendVisionFallbackEvent(out, usedURLs)
	return out
}

func (s *server) shouldApplyVisionFallback(req orchestrator.Request) bool {
	mode := strings.ToLower(strings.TrimSpace(stringFromAny(req.Metadata["vision_fallback_mode"])))
	switch mode {
	case "off", "disabled", "none":
		return false
	case "force", "on", "always":
		return true
	}
	if v, ok := boolFromAny(req.Metadata["upstream_supports_vision"]); ok {
		return !v
	}
	visionSupport := strings.ToLower(strings.TrimSpace(stringFromAny(req.Metadata["upstream_vision_support"])))
	switch visionSupport {
	case "supported", "yes", "true":
		return false
	case "unsupported", "no", "false":
		return true
	}
	if supported, known := s.resolveVisionSupportFromUpstream(req); known {
		return !supported
	}
	if s.settings != nil {
		if supported, known := s.settings.ResolveVisionSupport(req.Model); known {
			return !supported
		}
	}
	return modelLikelyNoVision(req.Model)
}

func boolFromAny(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func modelLikelyNoVision(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return false
	}
	if strings.Contains(m, "gpt-3.5") {
		return true
	}
	if strings.HasPrefix(m, "text-") {
		return true
	}
	if strings.Contains(m, "claude-2") || strings.Contains(m, "claude-instant") {
		return true
	}
	if strings.Contains(m, "deepseek-chat") || strings.Contains(m, "deepseek-coder") {
		return true
	}
	return false
}

func (s *server) resolveVisionSupportFromUpstream(req orchestrator.Request) (supported bool, known bool) {
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
		if !ok || spec.SupportsVision == nil {
			continue
		}
		anyKnown = true
		if *spec.SupportsVision {
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

func routeFromMetadataLocal(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}
	raw, ok := metadata["routing_adapter_route"]
	if !ok {
		return nil
	}
	switch route := raw.(type) {
	case []string:
		out := make([]string, 0, len(route))
		for _, item := range route {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(route))
		for _, item := range route {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func resolveRouteByModel(cfg upstream.UpstreamAdminConfig, model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return cleanRouteLocal(cfg.DefaultRoute)
	}
	routes := cfg.ModelRoutes
	if len(routes) == 0 {
		return cleanRouteLocal(cfg.DefaultRoute)
	}
	if seq := cleanRouteLocal(routes[model]); len(seq) > 0 {
		return seq
	}

	type patternRoute struct {
		pattern     string
		specificity int
		route       []string
	}
	patterns := make([]patternRoute, 0, len(routes))
	for p, r := range routes {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" || !strings.Contains(p, "*") {
			continue
		}
		patterns = append(patterns, patternRoute{
			pattern:     p,
			specificity: len(strings.ReplaceAll(p, "*", "")),
			route:       cleanRouteLocal(r),
		})
	}
	sort.Slice(patterns, func(i, j int) bool {
		if patterns[i].specificity == patterns[j].specificity {
			return patterns[i].pattern < patterns[j].pattern
		}
		return patterns[i].specificity > patterns[j].specificity
	})
	for _, p := range patterns {
		if len(p.route) == 0 {
			continue
		}
		matched, err := path.Match(p.pattern, model)
		if err != nil || !matched {
			continue
		}
		return p.route
	}
	if seq := cleanRouteLocal(routes["*"]); len(seq) > 0 {
		return seq
	}
	return cleanRouteLocal(cfg.DefaultRoute)
}

func cleanRouteLocal(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func collectVisionMessageRefs(messages []orchestrator.Message) []visionMessageRefs {
	out := make([]visionMessageRefs, 0, len(messages))
	for i, msg := range messages {
		if !strings.EqualFold(strings.TrimSpace(msg.Role), "user") {
			continue
		}
		urls := extractImageURLsFromContent(msg.Content)
		if len(urls) == 0 {
			continue
		}
		out = append(out, visionMessageRefs{
			Index: i,
			URLs:  uniqueStrings(urls),
		})
	}
	return out
}

func extractImageURLsFromContent(content any) []string {
	switch c := content.(type) {
	case []any:
		out := make([]string, 0, len(c))
		for _, item := range c {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if u, ok := extractImageURLFromBlock(block); ok {
				out = append(out, u)
			}
		}
		return out
	case map[string]any:
		if u, ok := extractImageURLFromBlock(c); ok {
			return []string{u}
		}
		return nil
	default:
		return nil
	}
}

func extractImageURLFromBlock(block map[string]any) (string, bool) {
	typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(block["type"])))
	switch typ {
	case "image_url":
		if raw, ok := block["image_url"]; ok {
			switch v := raw.(type) {
			case string:
				v = strings.TrimSpace(v)
				return v, v != ""
			case map[string]any:
				if u, ok := v["url"].(string); ok {
					u = strings.TrimSpace(u)
					return u, u != ""
				}
			}
		}
	case "image":
		source, _ := block["source"].(map[string]any)
		if source == nil {
			return "", false
		}
		srcType := strings.ToLower(strings.TrimSpace(fmt.Sprint(source["type"])))
		switch srcType {
		case "url":
			if u, ok := source["url"].(string); ok {
				u = strings.TrimSpace(u)
				return u, u != ""
			}
		case "base64":
			data, _ := source["data"].(string)
			data = strings.TrimSpace(data)
			if data == "" {
				return "", false
			}
			mime, _ := source["media_type"].(string)
			mime = strings.TrimSpace(mime)
			if mime == "" {
				mime = "image/png"
			}
			return "data:" + mime + ";base64," + data, true
		}
	}
	return "", false
}

func (s *server) buildVisionReport(ctx context.Context, urls []string, cache map[string]string, used *[]string) string {
	lines := make([]string, 0, len(urls)+1)
	for i, raw := range urls {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		if _, ok := cache[url]; !ok {
			summary := s.runImageRecognition(ctx, url)
			cache[url] = summary
			*used = append(*used, url)
		}
		lines = append(lines, fmt.Sprintf("Image %d: %s", i+1, cache[url]))
	}
	if len(lines) == 0 {
		return ""
	}
	header := "[Vision fallback context] The upstream model may not support images. Use the recognized details below."
	return header + "\n" + strings.Join(lines, "\n")
}

func (s *server) runImageRecognition(ctx context.Context, imageURL string) string {
	if s.toolExecutor == nil {
		return "image recognition unavailable (tool executor not configured)"
	}
	result, err := s.toolExecutor.Execute(ctx, toolruntime.Call{
		Name: "image_recognition",
		Input: map[string]any{
			"image_url": imageURL,
		},
	})
	if err != nil {
		return "image recognition failed: " + truncateText(strings.TrimSpace(err.Error()), 120)
	}
	text := strings.TrimSpace(renderToolResultContent(result.Content))
	if text == "" {
		return "image recognized but summary is empty"
	}
	return truncateText(text, 600)
}

func stripImagesAndAppendVisionReport(content any, report string) any {
	report = strings.TrimSpace(report)
	if report == "" {
		return content
	}
	switch c := content.(type) {
	case []any:
		out := make([]any, 0, len(c)+1)
		removed := false
		for _, item := range c {
			block, ok := item.(map[string]any)
			if !ok {
				out = append(out, item)
				continue
			}
			if isImageBlock(block) {
				removed = true
				continue
			}
			out = append(out, item)
		}
		if !removed {
			return content
		}
		out = append(out, map[string]any{
			"type": "text",
			"text": report,
		})
		return out
	case string:
		c = strings.TrimSpace(c)
		if c == "" {
			return report
		}
		return c + "\n\n" + report
	case nil:
		return report
	default:
		return []any{
			map[string]any{
				"type": "text",
				"text": report,
			},
		}
	}
}

func isImageBlock(block map[string]any) bool {
	typ := strings.ToLower(strings.TrimSpace(fmt.Sprint(block["type"])))
	return typ == "image" || typ == "image_url"
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func (s *server) appendVisionFallbackEvent(req orchestrator.Request, imageURLs []string) {
	sessionID := ""
	mode := ""
	path := ""
	if req.Metadata != nil {
		sessionID = stringFromAny(req.Metadata["session_id"])
		mode = stringFromAny(req.Metadata["mode"])
		path = stringFromAny(req.Metadata["request_path"])
	}
	if strings.TrimSpace(path) == "" {
		path = "/v1/messages"
	}
	out := make([]string, 0, len(imageURLs))
	for _, item := range imageURLs {
		out = append(out, truncateText(strings.TrimSpace(item), 220))
	}
	sort.Strings(out)
	s.appendEvent(ccevent.AppendInput{
		EventType: "vision.fallback_applied",
		SessionID: sessionID,
		RunID:     req.RunID,
		Data: map[string]any{
			"path":   path,
			"mode":   mode,
			"count":  len(out),
			"images": out,
		},
	})
}
