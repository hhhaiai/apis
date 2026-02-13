package gateway

import (
	"net/http"
	"strings"

	"ccgateway/internal/ccevent"
)

func (s *server) appendEvent(in ccevent.AppendInput) {
	if s.eventStore == nil {
		return
	}
	_, _ = s.eventStore.Append(in)
}

func requestSessionID(r *http.Request, metadata map[string]any) string {
	if r != nil {
		if v := strings.TrimSpace(r.Header.Get("x-cc-session-id")); v != "" {
			return v
		}
	}
	for _, key := range []string{"session_id", "cc_session_id", "sessionId"} {
		if v, ok := metadata[key].(string); ok {
			if text := strings.TrimSpace(v); text != "" {
				return text
			}
		}
	}
	return ""
}
