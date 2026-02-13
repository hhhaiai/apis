package upstream

import (
	"context"
	"fmt"
	"strings"

	"ccgateway/internal/orchestrator"
)

// reflectionCriticPrompt asks the model to review its own response.
const reflectionCriticPrompt = `Review the following response for accuracy, completeness, and quality.
List any issues, errors, omissions, or areas for improvement. Be specific and concise.

Response to review:
%s`

// reflectionFixPrompt asks the model to fix based on a critique.
const reflectionFixPrompt = `Here is a response that was reviewed and issues were found.

Original response:
%s

Issues identified:
%s

Provide a corrected and improved response that addresses all identified issues. Output only the improved response, nothing else.`

// applyReflectionLoop runs a real multi-pass reflection: draft → critique → fix.
// Each pass sends the response back through the adapter for self-evaluation and correction.
// The method accumulates token usage across all passes and updates the trace.
func (s *RouterService) applyReflectionLoop(ctx context.Context, resp orchestrator.Response, req orchestrator.Request, passes int) orchestrator.Response {
	if passes <= 0 {
		return resp
	}

	currentText := extractTextFromBlocks(resp.Blocks)
	totalUsage := resp.Usage

	for pass := 0; pass < passes; pass++ {
		// Step 1: Critique — ask the model to review its own response
		critiqueReq := orchestrator.Request{
			Model:     req.Model,
			MaxTokens: req.MaxTokens,
			System:    "You are a critical reviewer. Identify issues in the response below.",
			Messages: []orchestrator.Message{
				{Role: "user", Content: fmt.Sprintf(reflectionCriticPrompt, currentText)},
			},
			Metadata: map[string]any{
				"reflection_pass":    pass + 1,
				"reflection_phase":   "critique",
				"reflection_passes":  0, // prevent recursive reflection
			},
			Headers: req.Headers,
		}

		critiqueResp, err := s.completeOnce(ctx, critiqueReq)
		if err != nil {
			// If critique fails, return what we have with partial trace
			resp.Trace.ReflectionPasses = pass
			resp.Usage = totalUsage
			return resp
		}
		totalUsage.InputTokens += critiqueResp.Usage.InputTokens
		totalUsage.OutputTokens += critiqueResp.Usage.OutputTokens

		critique := extractTextFromBlocks(critiqueResp.Blocks)
		if strings.TrimSpace(critique) == "" {
			// No issues found — stop early
			break
		}

		// Step 2: Fix — ask the model to produce a corrected response
		fixReq := orchestrator.Request{
			Model:     req.Model,
			MaxTokens: req.MaxTokens,
			System:    req.System,
			Messages: []orchestrator.Message{
				{Role: "user", Content: fmt.Sprintf(reflectionFixPrompt, currentText, critique)},
			},
			Metadata: map[string]any{
				"reflection_pass":    pass + 1,
				"reflection_phase":   "fix",
				"reflection_passes":  0, // prevent recursive reflection
			},
			Headers: req.Headers,
		}

		fixResp, err := s.completeOnce(ctx, fixReq)
		if err != nil {
			resp.Trace.ReflectionPasses = pass + 1
			resp.Usage = totalUsage
			return resp
		}
		totalUsage.InputTokens += fixResp.Usage.InputTokens
		totalUsage.OutputTokens += fixResp.Usage.OutputTokens

		fixedText := extractTextFromBlocks(fixResp.Blocks)
		if strings.TrimSpace(fixedText) != "" {
			currentText = fixedText
			// Update response blocks with the fixed content
			resp.Blocks = fixResp.Blocks
		}
	}

	resp.Trace.ReflectionPasses = passes
	resp.Usage = totalUsage
	return resp
}

// completeOnce performs a single completion without reflection or parallel candidates.
func (s *RouterService) completeOnce(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error) {
	candidates := s.routeForRequest(req)
	if s.selector != nil {
		candidates = s.selector.Order(req, candidates, false)
	}
	if len(candidates) == 0 {
		return orchestrator.Response{}, fmt.Errorf("no upstream adapter available for reflection")
	}

	retries := s.retries
	timeout := s.timeout

	for _, name := range candidates {
		r := s.runCandidate(ctx, req, name, 0, retries, timeout)
		if r.err == nil {
			return r.resp, nil
		}
	}
	return orchestrator.Response{}, fmt.Errorf("all adapters failed during reflection")
}

// extractTextFromBlocks concatenates all text blocks from a response.
func extractTextFromBlocks(blocks []orchestrator.AssistantBlock) string {
	var sb strings.Builder
	for _, b := range blocks {
		if b.Type == "text" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}
