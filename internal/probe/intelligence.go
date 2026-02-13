package probe

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ccgateway/internal/orchestrator"
	"ccgateway/internal/upstream"
)

// IntelligenceResult holds the intelligence score for an adapter+model pair.
type IntelligenceResult struct {
	AdapterName string    `json:"adapter_name"`
	Model       string    `json:"model"`
	Score       float64   `json:"score"`   // 0-100
	Details     []QAScore `json:"details"` // per-question scores
	TestedAt    time.Time `json:"tested_at"`
	LatencyMS   int64     `json:"latency_ms"`
	Error       string    `json:"error,omitempty"`
}

// QAScore holds the result of a single intelligence question.
type QAScore struct {
	Category string  `json:"category"`
	Question string  `json:"question"`
	Score    float64 `json:"score"` // 0-20
	Answer   string  `json:"answer,omitempty"`
}

type intelligenceQuestion struct {
	Category string
	Question string
	Checker  func(answer string) float64 // returns 0-20
}

// defaultIntelligenceQuestions are 5 diverse questions to probe model intelligence.
var defaultIntelligenceQuestions = []intelligenceQuestion{
	{
		Category: "reasoning",
		Question: "A farmer has 17 sheep. All but 9 run away. How many sheep does the farmer have left? Answer with just the number.",
		Checker: func(answer string) float64 {
			a := strings.TrimSpace(answer)
			if strings.Contains(a, "9") && !strings.Contains(a, "8") && !strings.Contains(a, "17") {
				return 20
			}
			if strings.Contains(a, "9") {
				return 12
			}
			return 0
		},
	},
	{
		Category: "coding",
		Question: "Write a Python function called `fibonacci` that returns the nth Fibonacci number. Keep it short, just the function.",
		Checker: func(answer string) float64 {
			a := strings.ToLower(answer)
			score := 0.0
			if strings.Contains(a, "def fibonacci") || strings.Contains(a, "def fib") {
				score += 8
			}
			if strings.Contains(a, "return") {
				score += 4
			}
			if strings.Contains(a, "if") || strings.Contains(a, "while") || strings.Contains(a, "for") {
				score += 4
			}
			if strings.Contains(a, "n") && (strings.Contains(a, "n-1") || strings.Contains(a, "n - 1")) {
				score += 4
			}
			if score > 20 {
				score = 20
			}
			return score
		},
	},
	{
		Category: "math",
		Question: "What is 37 * 43? Answer with just the number.",
		Checker: func(answer string) float64 {
			a := strings.TrimSpace(answer)
			if strings.Contains(a, "1591") {
				return 20
			}
			// Partial credit for being close
			if strings.Contains(a, "159") {
				return 8
			}
			return 0
		},
	},
	{
		Category: "instruction_following",
		Question: "List exactly 3 colors, one per line. Do not add any other text, numbering, or explanation.",
		Checker: func(answer string) float64 {
			lines := strings.Split(strings.TrimSpace(answer), "\n")
			cleaned := make([]string, 0)
			for _, l := range lines {
				l = strings.TrimSpace(l)
				if l != "" {
					cleaned = append(cleaned, l)
				}
			}
			score := 0.0
			if len(cleaned) == 3 {
				score += 10
			} else if len(cleaned) >= 1 && len(cleaned) <= 5 {
				score += 4
			}
			colors := []string{"red", "blue", "green", "yellow", "purple", "orange", "pink", "black", "white", "brown", "gray", "grey", "cyan", "magenta", "violet", "indigo"}
			colorCount := 0
			for _, l := range cleaned {
				lower := strings.ToLower(strings.TrimLeft(l, "0123456789.-) "))
				for _, c := range colors {
					if strings.Contains(lower, c) {
						colorCount++
						break
					}
				}
			}
			score += float64(colorCount) * 10.0 / 3.0
			if score > 20 {
				score = 20
			}
			return score
		},
	},
	{
		Category: "summarization",
		Question: "Summarize in one sentence: 'The quick brown fox jumps over the lazy dog' is a pangram, meaning it contains every letter of the English alphabet at least once.",
		Checker: func(answer string) float64 {
			a := strings.ToLower(answer)
			score := 0.0
			if strings.Contains(a, "pangram") {
				score += 8
			}
			if strings.Contains(a, "every letter") || strings.Contains(a, "all letter") || strings.Contains(a, "all 26") || strings.Contains(a, "each letter") {
				score += 6
			}
			if strings.Contains(a, "alphabet") {
				score += 3
			}
			if strings.Contains(a, "sentence") || strings.Contains(a, "fox") {
				score += 3
			}
			if score > 20 {
				score = 20
			}
			return score
		},
	},
}

// ProbeIntelligence tests the intelligence of an adapter by sending benchmark questions.
func ProbeIntelligence(ctx context.Context, adapter upstream.Adapter, model string, timeout time.Duration) IntelligenceResult {
	started := time.Now()
	result := IntelligenceResult{
		AdapterName: adapter.Name(),
		Model:       model,
		TestedAt:    started,
		Details:     make([]QAScore, 0, len(defaultIntelligenceQuestions)),
	}

	totalScore := 0.0
	for _, q := range defaultIntelligenceQuestions {
		qCtx, cancel := context.WithTimeout(ctx, timeout)
		resp, err := adapter.Complete(qCtx, orchestrator.Request{
			Model:     model,
			MaxTokens: 256,
			System:    "Answer concisely and precisely. Follow instructions exactly.",
			Messages: []orchestrator.Message{
				{Role: "user", Content: q.Question},
			},
		})
		cancel()

		qs := QAScore{
			Category: q.Category,
			Question: q.Question,
		}

		if err != nil {
			qs.Score = 0
			qs.Answer = fmt.Sprintf("error: %s", err.Error())
		} else {
			answerText := extractResponseText(resp)
			qs.Answer = truncate(answerText, 500)
			qs.Score = q.Checker(answerText)
		}
		totalScore += qs.Score
		result.Details = append(result.Details, qs)
	}

	result.Score = totalScore
	result.LatencyMS = time.Since(started).Milliseconds()
	return result
}

func extractResponseText(resp orchestrator.Response) string {
	var sb strings.Builder
	for _, b := range resp.Blocks {
		if b.Type == "text" {
			if sb.Len() > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(b.Text)
		}
	}
	return sb.String()
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}
