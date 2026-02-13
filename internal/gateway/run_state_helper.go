package gateway

import (
	"net/http"

	"ccgateway/internal/ccrun"
)

func runListFilterFromRequest(r *http.Request, limit int) ccrun.ListFilter {
	return ccrun.ListFilter{
		Limit:     limit,
		SessionID: r.URL.Query().Get("session_id"),
		Status:    r.URL.Query().Get("status"),
		Path:      r.URL.Query().Get("path"),
	}
}

func (s *server) createRunIfConfigured(in ccrun.CreateInput) {
	if s.runStore == nil {
		return
	}
	_, _ = s.runStore.Create(in)
}

func (s *server) completeRunIfConfigured(runID string, statusCode int, errText string) {
	if s.runStore == nil {
		return
	}
	_, _ = s.runStore.Complete(runID, ccrun.CompleteInput{
		StatusCode: statusCode,
		Error:      errText,
	})
}
