package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// EvalResult holds the evaluation scores and analysis.
type EvalResult struct {
	Score    float64            `json:"score"`
	Criteria map[string]float64 `json:"criteria"`
	Analysis string             `json:"analysis"`
	Model    string             `json:"model"`
	Duration time.Duration      `json:"duration_ms"`
}

// Completer is the interface for calling a model (satisfied by orchestrator.Service).
type Completer interface {
	CompleteSimple(ctx context.Context, model, system, user string) (string, error)
}

// Evaluator performs automatic quality evaluation on model responses.
type Evaluator struct {
	completer  Completer
	judgeModel string
}

// NewEvaluator creates a new evaluator with a judge model.
func NewEvaluator(completer Completer, judgeModel string) *Evaluator {
	if judgeModel == "" {
		judgeModel = "claude-3-5-sonnet-20241022"
	}
	return &Evaluator{
		completer:  completer,
		judgeModel: judgeModel,
	}
}

const evalSystemPrompt = `You are an expert AI response evaluator. Score the given response on these criteria (each 0-10):

1. accuracy: factual correctness
2. completeness: covers all aspects of the prompt
3. reasoning: logical depth and quality
4. code_quality: if code is present, its correctness and style (score 7 if no code)
5. instruction_following: how well the response follows the prompt's instructions

Return ONLY valid JSON:
{"accuracy":N,"completeness":N,"reasoning":N,"code_quality":N,"instruction_following":N,"analysis":"brief 2-3 sentence analysis"}`

// Evaluate scores a model's response quality using a judge model.
func (e *Evaluator) Evaluate(ctx context.Context, model, prompt, response string) (EvalResult, error) {
	if e.completer == nil {
		return EvalResult{}, fmt.Errorf("no completer configured")
	}

	start := time.Now()

	userMsg := fmt.Sprintf("## Prompt\n%s\n\n## Response\n%s", prompt, response)

	raw, err := e.completer.CompleteSimple(ctx, e.judgeModel, evalSystemPrompt, userMsg)
	if err != nil {
		return EvalResult{}, fmt.Errorf("judge model failed: %w", err)
	}

	result, err := parseEvalOutput(raw)
	if err != nil {
		// If parsing fails, return a basic result
		return EvalResult{
			Score:    5.0,
			Criteria: map[string]float64{},
			Analysis: "Judge model output could not be parsed: " + raw,
			Model:    model,
			Duration: time.Since(start),
		}, nil
	}

	result.Model = model
	result.Duration = time.Since(start)
	return result, nil
}

// EvaluateWithGeneration first generates a response then evaluates it.
func (e *Evaluator) EvaluateWithGeneration(ctx context.Context, model, prompt string) (EvalResult, string, error) {
	if e.completer == nil {
		return EvalResult{}, "", fmt.Errorf("no completer configured")
	}

	// Step 1: Generate response from target model
	response, err := e.completer.CompleteSimple(ctx, model, "", prompt)
	if err != nil {
		return EvalResult{}, "", fmt.Errorf("target model failed: %w", err)
	}

	// Step 2: Evaluate the response
	result, err := e.Evaluate(ctx, model, prompt, response)
	if err != nil {
		return EvalResult{}, response, err
	}

	return result, response, nil
}

func parseEvalOutput(raw string) (EvalResult, error) {
	raw = strings.TrimSpace(raw)

	// Try to find JSON in the output
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < 0 || end <= start {
		return EvalResult{}, fmt.Errorf("no JSON found in output")
	}
	jsonStr := raw[start : end+1]

	var parsed struct {
		Accuracy             float64 `json:"accuracy"`
		Completeness         float64 `json:"completeness"`
		Reasoning            float64 `json:"reasoning"`
		CodeQuality          float64 `json:"code_quality"`
		InstructionFollowing float64 `json:"instruction_following"`
		Analysis             string  `json:"analysis"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		return EvalResult{}, fmt.Errorf("JSON parse error: %w", err)
	}

	criteria := map[string]float64{
		"accuracy":              clamp(parsed.Accuracy),
		"completeness":          clamp(parsed.Completeness),
		"reasoning":             clamp(parsed.Reasoning),
		"code_quality":          clamp(parsed.CodeQuality),
		"instruction_following": clamp(parsed.InstructionFollowing),
	}

	// Weighted average
	total := 0.0
	weights := map[string]float64{
		"accuracy": 0.25, "completeness": 0.2, "reasoning": 0.25,
		"code_quality": 0.1, "instruction_following": 0.2,
	}
	for k, w := range weights {
		total += criteria[k] * w
	}

	return EvalResult{
		Score:    math.Round(total*10) / 10,
		Criteria: criteria,
		Analysis: parsed.Analysis,
	}, nil
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return math.Round(v*10) / 10
}
