package costtrack

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// Cost represents cost in USD.
type Cost struct {
	InputCost  float64 `json:"input_cost"`
	OutputCost float64 `json:"output_cost"`
	TotalCost  float64 `json:"total_cost"`
}

// ModelPricing defines per-token cost for a model.
type ModelPricing struct {
	InputPer1M  float64 `json:"input_per_1m"`  // cost per 1M input tokens
	OutputPer1M float64 `json:"output_per_1m"` // cost per 1M output tokens
}

// Tracker tracks costs per session/run and enforces budget limits.
type Tracker struct {
	mu      sync.RWMutex
	pricing map[string]ModelPricing
	costs   map[string]Cost // key: session or run ID
	budget  float64         // max total cost (0 = unlimited)
}

// New creates a new cost tracker.
func New(pricing map[string]ModelPricing, budgetUSD float64) *Tracker {
	if pricing == nil {
		pricing = DefaultPricing()
	}
	return &Tracker{
		pricing: pricing,
		costs:   make(map[string]Cost),
		budget:  budgetUSD,
	}
}

// NewFromEnv creates a Tracker from environment variables:
//   - MODEL_PRICING_JSON: JSON map of model->pricing (optional, uses defaults)
//   - BUDGET_LIMIT_USD: max total cost in USD (0 = unlimited)
func NewFromEnv() *Tracker {
	pricing := DefaultPricing()
	if raw := strings.TrimSpace(os.Getenv("MODEL_PRICING_JSON")); raw != "" {
		var custom map[string]ModelPricing
		if err := json.Unmarshal([]byte(raw), &custom); err == nil {
			for k, v := range custom {
				pricing[k] = v
			}
		}
	}
	budget := 0.0
	if raw := strings.TrimSpace(os.Getenv("BUDGET_LIMIT_USD")); raw != "" {
		_, _ = fmt.Sscanf(raw, "%f", &budget)
	}
	return New(pricing, budget)
}

// DefaultPricing returns common model pricing (as of 2025-2026).
func DefaultPricing() map[string]ModelPricing {
	return map[string]ModelPricing{
		"claude-sonnet-4-20250514": {InputPer1M: 3.0, OutputPer1M: 15.0},
		"claude-3-5-sonnet":        {InputPer1M: 3.0, OutputPer1M: 15.0},
		"claude-3-haiku":           {InputPer1M: 0.25, OutputPer1M: 1.25},
		"claude-3-opus":            {InputPer1M: 15.0, OutputPer1M: 75.0},
		"gpt-4o":                   {InputPer1M: 2.5, OutputPer1M: 10.0},
		"gpt-4o-mini":              {InputPer1M: 0.15, OutputPer1M: 0.6},
		"deepseek-chat":            {InputPer1M: 0.27, OutputPer1M: 1.10},
		"*":                        {InputPer1M: 3.0, OutputPer1M: 15.0}, // fallback
	}
}

// Record records token usage for a model under a given key (session/run ID).
func (t *Tracker) Record(key, model string, inputTokens, outputTokens int) Cost {
	t.mu.Lock()
	defer t.mu.Unlock()

	p, ok := t.pricing[model]
	if !ok {
		p = t.pricing["*"]
	}

	c := Cost{
		InputCost:  float64(inputTokens) / 1_000_000 * p.InputPer1M,
		OutputCost: float64(outputTokens) / 1_000_000 * p.OutputPer1M,
	}
	c.TotalCost = c.InputCost + c.OutputCost

	existing := t.costs[key]
	existing.InputCost += c.InputCost
	existing.OutputCost += c.OutputCost
	existing.TotalCost += c.TotalCost
	t.costs[key] = existing

	return c
}

// Total returns the accumulated cost for a given key.
func (t *Tracker) Total(key string) Cost {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.costs[key]
}

// GlobalTotal returns the total cost across all keys.
func (t *Tracker) GlobalTotal() Cost {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total Cost
	for _, c := range t.costs {
		total.InputCost += c.InputCost
		total.OutputCost += c.OutputCost
		total.TotalCost += c.TotalCost
	}
	return total
}

// CheckBudget returns an error if the total cost exceeds the budget.
func (t *Tracker) CheckBudget(key string) error {
	if t.budget <= 0 {
		return nil // no budget limit
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	total := 0.0
	for _, c := range t.costs {
		total += c.TotalCost
	}
	if total >= t.budget {
		return fmt.Errorf("budget exceeded: total $%.4f >= limit $%.4f", total, t.budget)
	}
	return nil
}

// Snapshot returns a copy of all tracked costs.
func (t *Tracker) Snapshot() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	perKey := make(map[string]Cost, len(t.costs))
	var total Cost
	for k, v := range t.costs {
		perKey[k] = v
		total.InputCost += v.InputCost
		total.OutputCost += v.OutputCost
		total.TotalCost += v.TotalCost
	}

	return map[string]any{
		"total_cost_usd":   total.TotalCost,
		"budget_limit_usd": t.budget,
		"per_model":        perKey,
	}
}
