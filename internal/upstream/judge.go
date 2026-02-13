package upstream

import (
	"context"
	"strings"
	"time"

	"ccgateway/internal/orchestrator"
)

type CandidateJudge interface {
	Select(ctx context.Context, req orchestrator.Request, candidates []JudgedCandidate) (int, error)
}

type JudgedCandidate struct {
	AdapterName string
	Response    orchestrator.Response
	Latency     time.Duration
	Order       int
}

type HeuristicJudge struct{}

func NewHeuristicJudge() *HeuristicJudge {
	return &HeuristicJudge{}
}

func (j *HeuristicJudge) Select(_ context.Context, req orchestrator.Request, candidates []JudgedCandidate) (int, error) {
	if len(candidates) == 0 {
		return -1, nil
	}
	best := 0
	bestScore := j.score(req, candidates[0])
	for i := 1; i < len(candidates); i++ {
		score := j.score(req, candidates[i])
		if score > bestScore {
			best = i
			bestScore = score
			continue
		}
		if score == bestScore {
			if candidates[i].Latency < candidates[best].Latency {
				best = i
				bestScore = score
				continue
			}
			if candidates[i].Latency == candidates[best].Latency && candidates[i].Order < candidates[best].Order {
				best = i
				bestScore = score
			}
		}
	}
	return best, nil
}

func (j *HeuristicJudge) score(req orchestrator.Request, candidate JudgedCandidate) float64 {
	score := 0.0
	textLen := 0
	toolCount := 0
	for _, b := range candidate.Response.Blocks {
		switch b.Type {
		case "text":
			textLen += len(strings.TrimSpace(b.Text))
		case "tool_use":
			toolCount++
		}
	}

	if textLen > 0 {
		score += minFloat(float64(textLen)/24.0, 18)
	}
	if strings.TrimSpace(candidate.Response.StopReason) == "end_turn" {
		score += 6
	}
	if strings.TrimSpace(candidate.Response.StopReason) == "tool_use" {
		score += 2
	}
	if candidate.Response.Usage.OutputTokens > 0 {
		score += minFloat(float64(candidate.Response.Usage.OutputTokens)/20.0, 8)
	}

	expectTools := len(req.Tools) > 0
	if expectTools {
		if toolCount > 0 {
			score += 10
		} else {
			score -= 8
		}
	} else if toolCount > 0 {
		score -= 2
	}

	latPenalty := minFloat(float64(candidate.Latency.Milliseconds())/250.0, 6)
	score -= latPenalty
	return score
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
