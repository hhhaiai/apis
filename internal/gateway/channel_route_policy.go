package gateway

import (
	"context"
	"strings"

	"ccgateway/internal/token"
	"ccgateway/internal/upstream"
)

const defaultChannelGroup = "default"

func (s *server) applyChannelRoutePolicy(ctx context.Context, metadata map[string]any, model string) map[string]any {
	route := s.resolveChannelRoute(ctx, model)
	if len(route) == 0 {
		return metadata
	}
	out := make(map[string]any, len(metadata)+2)
	for k, v := range metadata {
		out[k] = v
	}
	out["routing_adapter_route"] = route
	out["routing_route_source"] = "channel"
	return out
}

func (s *server) resolveChannelRoute(ctx context.Context, model string) []string {
	model = strings.TrimSpace(model)
	if model == "" || s.channelStore == nil {
		return nil
	}

	for _, group := range channelCandidateGroups(s.resolveUserGroup(ctx)) {
		ch, ok := s.channelStore.GetChannelByGroupAndModel(group, model)
		if !ok || ch == nil {
			continue
		}
		adapterName := strings.TrimSpace(ch.Name)
		if adapterName == "" || !s.isKnownAdapterName(adapterName) {
			continue
		}
		return []string{adapterName}
	}
	return nil
}

func (s *server) resolveUserGroup(ctx context.Context) string {
	if ctx == nil || s.authService == nil {
		return defaultChannelGroup
	}
	tk, ok := ctx.Value(tokenContextKey).(*token.Token)
	if !ok || tk == nil {
		return defaultChannelGroup
	}
	userID := strings.TrimSpace(tk.UserID)
	if userID == "" {
		return defaultChannelGroup
	}
	user, err := s.authService.Get(userID)
	if err != nil {
		return defaultChannelGroup
	}
	group := strings.TrimSpace(user.Group)
	if group == "" {
		return defaultChannelGroup
	}
	return group
}

func channelCandidateGroups(primary string) []string {
	primary = strings.TrimSpace(primary)
	if primary == "" || strings.EqualFold(primary, defaultChannelGroup) {
		return []string{defaultChannelGroup}
	}
	return []string{primary, defaultChannelGroup}
}

func (s *server) isKnownAdapterName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || s.orchestrator == nil {
		return false
	}
	provider, ok := s.orchestrator.(interface {
		GetUpstreamConfig() upstream.UpstreamAdminConfig
	})
	if !ok {
		return true
	}
	for _, spec := range provider.GetUpstreamConfig().Adapters {
		if strings.EqualFold(strings.TrimSpace(spec.Name), name) {
			return true
		}
	}
	return false
}
