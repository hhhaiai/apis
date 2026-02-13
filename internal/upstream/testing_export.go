package upstream

import (
	"context"

	"ccgateway/internal/orchestrator"
)

// Export unexported functions for external tests.
var ParseJudgeIndex = parseJudgeIndex
var ExtractTextFromBlocks = extractTextFromBlocks

// ApplyReflectionLoop exports the unexported method for testing.
func (s *RouterService) ApplyReflectionLoop(ctx context.Context, resp orchestrator.Response, req orchestrator.Request, passes int) orchestrator.Response {
	return s.applyReflectionLoop(ctx, resp, req, passes)
}
