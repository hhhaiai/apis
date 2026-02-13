package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"ccgateway/internal/orchestrator"
)

var firstIntPattern = regexp.MustCompile(`-?\d+`)

type LLMJudgeConfig struct {
	Route        []string
	Model        string
	Timeout      time.Duration
	Retries      int
	MaxTokens    int
	SystemPrompt string
}

type LLMJudge struct {
	adapters map[string]Adapter
	cfg      LLMJudgeConfig
}

func NewLLMJudge(cfg LLMJudgeConfig, adapters []Adapter) (*LLMJudge, error) {
	cfg = sanitizeLLMJudgeConfig(cfg)
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("judge model is required")
	}
	if len(cfg.Route) == 0 {
		return nil, fmt.Errorf("judge route is required")
	}

	adapterMap := map[string]Adapter{}
	for _, a := range adapters {
		if a == nil {
			continue
		}
		name := strings.TrimSpace(a.Name())
		if name == "" {
			continue
		}
		adapterMap[name] = a
	}
	if len(adapterMap) == 0 {
		return nil, fmt.Errorf("judge adapters are empty")
	}
	return &LLMJudge{
		adapters: adapterMap,
		cfg:      cfg,
	}, nil
}

func (j *LLMJudge) Select(ctx context.Context, req orchestrator.Request, candidates []JudgedCandidate) (int, error) {
	if len(candidates) == 0 {
		return -1, nil
	}
	if len(candidates) == 1 {
		return 0, nil
	}

	prompt, err := j.buildPrompt(req, candidates)
	if err != nil {
		return -1, err
	}
	var lastErr error
	for _, adapterName := range j.cfg.Route {
		adapterName = strings.TrimSpace(adapterName)
		if adapterName == "" {
			continue
		}
		adapter, ok := j.adapters[adapterName]
		if !ok {
			lastErr = fmt.Errorf("judge adapter %q not registered", adapterName)
			continue
		}
		for attempt := 0; attempt <= j.cfg.Retries; attempt++ {
			attemptCtx, cancel := context.WithTimeout(ctx, j.cfg.Timeout)
			resp, err := adapter.Complete(attemptCtx, orchestrator.Request{
				Model:     j.cfg.Model,
				MaxTokens: j.cfg.MaxTokens,
				System:    j.cfg.SystemPrompt,
				Messages: []orchestrator.Message{
					{Role: "user", Content: prompt},
				},
			})
			cancel()
			if err != nil {
				lastErr = err
				continue
			}
			idx, err := parseJudgeIndex(resp, len(candidates))
			if err != nil {
				lastErr = err
				continue
			}
			return idx, nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("judge failed with no available adapter")
	}
	return -1, lastErr
}

func sanitizeLLMJudgeConfig(cfg LLMJudgeConfig) LLMJudgeConfig {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 64
	}
	cfg.Model = strings.TrimSpace(cfg.Model)
	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		cfg.SystemPrompt = "You are a strict judge. Select the best candidate index for quality and task fit. Reply with ONLY the integer index."
	}
	cleanRoute := make([]string, 0, len(cfg.Route))
	for _, item := range cfg.Route {
		item = strings.TrimSpace(item)
		if item != "" {
			cleanRoute = append(cleanRoute, item)
		}
	}
	cfg.Route = cleanRoute
	return cfg
}

func (j *LLMJudge) buildPrompt(req orchestrator.Request, candidates []JudgedCandidate) (string, error) {
	type candidatePayload struct {
		Index      int      `json:"index"`
		Adapter    string   `json:"adapter"`
		LatencyMS  int64    `json:"latency_ms"`
		StopReason string   `json:"stop_reason"`
		Text       string   `json:"text"`
		Tools      []string `json:"tools"`
	}
	payload := struct {
		TaskModel   string             `json:"task_model"`
		ExpectTools bool               `json:"expect_tools"`
		Candidates  []candidatePayload `json:"candidates"`
	}{
		TaskModel:   strings.TrimSpace(req.Model),
		ExpectTools: len(req.Tools) > 0,
		Candidates:  make([]candidatePayload, 0, len(candidates)),
	}

	for i, c := range candidates {
		text, tools := summarizeBlocks(c.Response.Blocks)
		payload.Candidates = append(payload.Candidates, candidatePayload{
			Index:      i,
			Adapter:    c.AdapterName,
			LatencyMS:  c.Latency.Milliseconds(),
			StopReason: strings.TrimSpace(c.Response.StopReason),
			Text:       text,
			Tools:      tools,
		})
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return "Select one candidate index from JSON and return integer only:\n" + string(raw), nil
}

func summarizeBlocks(blocks []orchestrator.AssistantBlock) (string, []string) {
	text := strings.Builder{}
	tools := map[string]struct{}{}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			part := strings.TrimSpace(b.Text)
			if part == "" {
				continue
			}
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(part)
		case "tool_use":
			name := strings.TrimSpace(b.Name)
			if name != "" {
				tools[name] = struct{}{}
			}
		}
	}
	toolList := make([]string, 0, len(tools))
	for k := range tools {
		toolList = append(toolList, k)
	}
	sort.Strings(toolList)
	return trimTo(text.String(), 800), toolList
}

func trimTo(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func parseJudgeIndex(resp orchestrator.Response, count int) (int, error) {
	if count <= 0 {
		return -1, fmt.Errorf("candidate count must be > 0")
	}
	text := responseText(resp)
	text = strings.TrimSpace(text)
	if text == "" {
		return -1, fmt.Errorf("empty judge response")
	}

	var obj struct {
		Index int `json:"index"`
	}
	if err := json.Unmarshal([]byte(text), &obj); err == nil {
		if obj.Index >= 0 && obj.Index < count {
			return obj.Index, nil
		}
	}

	match := firstIntPattern.FindString(text)
	if strings.TrimSpace(match) == "" {
		return -1, fmt.Errorf("judge response missing index: %q", text)
	}
	var idx int
	if _, err := fmt.Sscanf(match, "%d", &idx); err != nil {
		return -1, fmt.Errorf("judge index parse failed: %w", err)
	}
	if idx < 0 || idx >= count {
		return -1, fmt.Errorf("judge index out of range: %d", idx)
	}
	return idx, nil
}

func responseText(resp orchestrator.Response) string {
	parts := make([]string, 0, len(resp.Blocks))
	for _, b := range resp.Blocks {
		if b.Type != "text" {
			continue
		}
		part := strings.TrimSpace(b.Text)
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "\n")
}
