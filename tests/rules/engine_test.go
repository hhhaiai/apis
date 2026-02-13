package rules_test

import (
	. "ccgateway/internal/rules"
	"testing"
)

func TestEngine_AddAndEvaluate(t *testing.T) {
	e := NewEngine()
	_ = e.AddRule(Rule{Pattern: "web_search", Action: ActionAllow, Priority: 1})
	_ = e.AddRule(Rule{Pattern: "rm_*", Action: ActionDeny, Priority: 10})
	_ = e.AddRule(Rule{Pattern: "sudo_*", Action: ActionAsk, Priority: 5})

	tests := []struct {
		name string
		want Action
	}{
		{"web_search", ActionAllow},
		{"rm_rf", ActionDeny},
		{"rm_file", ActionDeny},
		{"sudo_install", ActionAsk},
		{"unknown_tool", ActionAllow}, // default
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.Evaluate(tt.name, "tool")
			if got != tt.want {
				t.Errorf("Evaluate(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestEngine_PriorityOrder(t *testing.T) {
	e := NewEngine()
	// Lower priority: allow all
	_ = e.AddRule(Rule{Pattern: "*", Action: ActionAllow, Priority: 1})
	// Higher priority: deny dangerous
	_ = e.AddRule(Rule{Pattern: "dangerous_*", Action: ActionDeny, Priority: 10})

	if got := e.Evaluate("dangerous_tool", "tool"); got != ActionDeny {
		t.Fatalf("higher priority rule should win, got %q", got)
	}
	if got := e.Evaluate("safe_tool", "tool"); got != ActionAllow {
		t.Fatalf("safe tool should be allowed, got %q", got)
	}
}

func TestEngine_ScopeFiltering(t *testing.T) {
	e := NewEngine()
	_ = e.AddRule(Rule{Pattern: "write_file", Action: ActionAsk, Scope: "file"})

	// Should not match "tool" scope
	if got := e.Evaluate("write_file", "tool"); got != ActionAllow {
		t.Fatalf("should not match different scope, got %q", got)
	}
	// Should match "file" scope
	if got := e.Evaluate("write_file", "file"); got != ActionAsk {
		t.Fatalf("should match file scope, got %q", got)
	}
}

func TestEngine_InvalidAction(t *testing.T) {
	e := NewEngine()
	err := e.AddRule(Rule{Pattern: "test", Action: "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestEngine_RemoveRule(t *testing.T) {
	e := NewEngine()
	_ = e.AddRule(Rule{Pattern: "test", Action: ActionDeny})
	rules := e.ListRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}
	_ = e.RemoveRule(rules[0].ID)
	if len(e.ListRules()) != 0 {
		t.Fatal("expected 0 rules after remove")
	}
}
