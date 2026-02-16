package memory

import (
	"context"
	"fmt"
	"strings"

	"ccgateway/internal/orchestrator"
)

// UpstreamClient defines the interface for making completion requests
type UpstreamClient interface {
	Complete(ctx context.Context, req orchestrator.Request) (orchestrator.Response, error)
}

// Summarizer defines the interface for summarizing conversations
type Summarizer interface {
	// SummarizeRecent summarizes the recent conversation history
	SummarizeRecent(ctx context.Context, messages []Message) (string, error)

	// SummarizeSession summarizes the entire session
	SummarizeSession(ctx context.Context, sessionID string) (*SessionMemory, error)

	// ExtractKeyInfo extracts key information from messages
	ExtractKeyInfo(ctx context.Context, messages []Message) (map[string]interface{}, error)
}

// LLMSummarizer implements Summarizer using an LLM
type LLMSummarizer struct {
	upstreamClient UpstreamClient
	model          string // Model to use for summarization
}

// NewLLMSummarizer creates a new LLMSummarizer
func NewLLMSummarizer(client UpstreamClient, model string) *LLMSummarizer {
	return &LLMSummarizer{
		upstreamClient: client,
		model:          model,
	}
}

// SummarizeRecent summarizes the recent conversation history
func (s *LLMSummarizer) SummarizeRecent(ctx context.Context, messages []Message) (string, error) {
	prompt := buildSummaryPrompt(messages)

	orchMessages := []orchestrator.Message{
		{Role: "system", Content: []any{map[string]any{"type": "text", "text": summarySystemPrompt}}},
		{Role: "user", Content: []any{map[string]any{"type": "text", "text": prompt}}},
	}

	req := orchestrator.Request{
		Model:     s.model,
		Messages:  orchMessages,
		MaxTokens: 500,
	}

	resp, err := s.upstreamClient.Complete(ctx, req)
	if err != nil {
		return "", fmt.Errorf("summarize failed: %w", err)
	}

	if len(resp.Blocks) > 0 {
		return resp.Blocks[0].Text, nil
	}
	return "", fmt.Errorf("empty summary response")
}

// SummarizeSession is a placeholder for now
func (s *LLMSummarizer) SummarizeSession(ctx context.Context, sessionID string) (*SessionMemory, error) {
	return nil, fmt.Errorf("not implemented")
}

// ExtractKeyInfo is a placeholder for now
func (s *LLMSummarizer) ExtractKeyInfo(ctx context.Context, messages []Message) (map[string]interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

const summarySystemPrompt = `You are a professional conversation summarization assistant. Please summarize the conversation between the user and the AI, extracting key information:
1. User's main goals and needs
2. Operations and modifications completed
3. Problems encountered and solutions
4. Pending items and next steps

Requirements:
- Be concise and clear, keep within 200 words
- Retain key filenames, variable names, and error messages
- Use a structured format (Goals/Progress/Issues/Plan)`

func buildSummaryPrompt(messages []Message) string {
	var sb strings.Builder
	sb.WriteString("Please summarize the following conversation:\n\n")
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("%s: %s\n\n", msg.Role, msg.Content))
	}
	return sb.String()
}
