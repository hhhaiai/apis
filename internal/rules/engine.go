package rules

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
)

// Action represents a policy action.
type Action string

const (
	ActionAllow Action = "allow"
	ActionAsk   Action = "ask"
	ActionDeny  Action = "deny"
)

// Rule defines a policy rule with pattern matching.
type Rule struct {
	ID          string `json:"id"`
	Pattern     string `json:"pattern"`  // glob pattern for tool/command name
	Action      Action `json:"action"`   // allow/ask/deny
	Scope       string `json:"scope"`    // "tool", "command", "file", "*"
	Priority    int    `json:"priority"` // higher = evaluated first
	Description string `json:"description,omitempty"`
}

// Engine evaluates rules against tool calls and commands.
type Engine struct {
	mu    sync.RWMutex
	rules []Rule
	idSeq int
}

// NewEngine creates a new rules engine.
func NewEngine() *Engine {
	return &Engine{
		rules: []Rule{},
	}
}

// AddRule adds a new rule to the engine.
func (e *Engine) AddRule(r Rule) error {
	if strings.TrimSpace(r.Pattern) == "" {
		return fmt.Errorf("rule pattern is required")
	}
	if r.Action != ActionAllow && r.Action != ActionAsk && r.Action != ActionDeny {
		return fmt.Errorf("invalid action %q, must be allow/ask/deny", r.Action)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.idSeq++
	r.ID = fmt.Sprintf("rule_%d", e.idSeq)
	if r.Scope == "" {
		r.Scope = "*"
	}
	e.rules = append(e.rules, r)

	// Re-sort by priority (higher priority first)
	sort.Slice(e.rules, func(i, j int) bool {
		return e.rules[i].Priority > e.rules[j].Priority
	})

	return nil
}

// RemoveRule removes a rule by ID.
func (e *Engine) RemoveRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i, r := range e.rules {
		if r.ID == id {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("rule %q not found", id)
}

// ListRules returns all rules.
func (e *Engine) ListRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]Rule, len(e.rules))
	copy(out, e.rules)
	return out
}

// Evaluate checks a tool/command name against all rules and returns the appropriate action.
// Rules are evaluated in priority order; first matching rule wins.
// If no rule matches, returns ActionAllow (default permissive).
func (e *Engine) Evaluate(name string, scope string) Action {
	e.mu.RLock()
	defer e.mu.RUnlock()

	name = strings.TrimSpace(name)
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "tool"
	}

	for _, r := range e.rules {
		// Check scope match
		if r.Scope != "*" && r.Scope != scope {
			continue
		}

		// Check pattern match
		matched, err := path.Match(r.Pattern, name)
		if err != nil {
			continue
		}
		if matched {
			return r.Action
		}
	}

	return ActionAllow // default: allow
}

// EvaluateWithContext evaluates with additional context metadata.
func (e *Engine) EvaluateWithContext(name, scope string, context map[string]any) Action {
	// For now, delegate to basic Evaluate.
	// Context-aware evaluation can be extended later.
	return e.Evaluate(name, scope)
}
