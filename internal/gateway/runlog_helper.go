package gateway

import "ccgateway/internal/runlog"

func (s *server) logRun(entry runlog.Entry) {
	if s.runLogger == nil {
		return
	}
	_ = s.runLogger.Log(entry)
}
