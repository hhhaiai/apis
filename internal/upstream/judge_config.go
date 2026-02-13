package upstream

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func NewJudgeFromEnv(adapters []Adapter, defaultRoute []string) (CandidateJudge, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("JUDGE_MODE")))
	if mode == "" || mode == "heuristic" {
		return NewHeuristicJudge(), nil
	}
	if mode != "llm" {
		return nil, fmt.Errorf("unsupported JUDGE_MODE %q", mode)
	}

	route := ParseListEnv("JUDGE_ROUTE", defaultRoute)
	model := strings.TrimSpace(os.Getenv("JUDGE_MODEL"))
	judge, err := NewLLMJudge(LLMJudgeConfig{
		Route:        route,
		Model:        model,
		Timeout:      ParseDurationEnv("JUDGE_TIMEOUT", ParseDurationEnv("UPSTREAM_TIMEOUT", 30*time.Second)),
		Retries:      ParseIntEnv("JUDGE_RETRIES", 0),
		MaxTokens:    ParseIntEnv("JUDGE_MAX_TOKENS", 64),
		SystemPrompt: strings.TrimSpace(os.Getenv("JUDGE_SYSTEM_PROMPT")),
	}, adapters)
	if err != nil {
		return nil, err
	}
	return judge, nil
}
