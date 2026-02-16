package upstream_test

import (
	"context"
	"testing"

	. "ccgateway/internal/upstream"
	"ccgateway/internal/orchestrator"
)

func TestTaskClassifier_ClassifyTask(t *testing.T) {
	classifier := NewTaskClassifier()

	tests := []struct {
		name     string
		messages []orchestrator.Message
		expected TaskComplexity
	}{
		{
			name:     "empty messages",
			messages: []orchestrator.Message{},
			expected: ComplexityLow,
		},
		{
			name: "very high complexity - architecture design",
			messages: []orchestrator.Message{
				{Role: "user", Content: "设计一个分布式系统架构"},
			},
			expected: ComplexityVeryHigh,
		},
		{
			name: "high complexity - write code",
			messages: []orchestrator.Message{
				{Role: "user", Content: "帮我写一个排序算法"},
			},
			expected: ComplexityHigh,
		},
		{
			name: "medium complexity - explain",
			messages: []orchestrator.Message{
				{Role: "user", Content: "解释一下这段代码"},
			},
			expected: ComplexityMedium,
		},
		{
			name: "low complexity - simple greeting",
			messages: []orchestrator.Message{
				{Role: "user", Content: "你好"},
			},
			expected: ComplexityLow,
		},
		{
			name: "high complexity - create new system",
			messages: []orchestrator.Message{
				{Role: "user", Content: "create a new system from scratch"},
			},
			expected: ComplexityVeryHigh,
		},
		{
			name: "high complexity - refactor",
			messages: []orchestrator.Message{
				{Role: "user", Content: "帮我重构这段代码"},
			},
			expected: ComplexityHigh,
		},
		{
			name: "high complexity - debug",
			messages: []orchestrator.Message{
				{Role: "user", Content: "帮我debug这个问题"},
			},
			expected: ComplexityHigh,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifier.ClassifyTask(context.Background(), tt.messages)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestTaskClassifier_ExtractText(t *testing.T) {
	classifier := NewTaskClassifier()

	// Test string content
	msg1 := orchestrator.Message{
		Role:    "user",
		Content: "hello world",
	}
	result1 := classifier.ClassifyTask(context.Background(), []orchestrator.Message{msg1})
	if result1 != ComplexityLow {
		t.Errorf("expected low for simple string, got %s", result1)
	}

	// Test array content (like vision messages)
	msg2 := orchestrator.Message{
		Role: "user",
		Content: []any{
			map[string]any{"type": "text", "text": "what's in this image"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:..."}},
		},
	}
	result2 := classifier.ClassifyTask(context.Background(), []orchestrator.Message{msg2})
	// Should extract text and classify
	if result2 != ComplexityLow {
		t.Errorf("expected low for vision message, got %s", result2)
	}
}

func TestShouldEmulateTools(t *testing.T) {
	tests := []struct {
		model               string
		upstreamSupportsTools bool
		expected            bool
	}{
		{"claude-3-5-sonnet-20241022", true, false},
		{"gpt-4o", true, false},
		{"ernie-4", false, true},
		{"glm-4-flash", false, true},
		{"qwen-turbo", false, true},
		{"gpt-3.5-turbo", false, true},
		{"claude-3-5-sonnet-20241022", false, false}, // not in noToolPrefixes
	}

	for _, tt := range tests {
		result := ShouldEmulateTools(tt.model, tt.upstreamSupportsTools)
		if result != tt.expected {
			t.Errorf("ShouldEmulateTools(%s, %v) = %v, expected %v",
				tt.model, tt.upstreamSupportsTools, result, tt.expected)
		}
	}
}

func TestGetEmulationMode(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"claude-3-5-sonnet-20241022", "native"},
		{"gpt-4o", "native"},
		{"qwen-plus", "native"},
		{"moonshot-v1-8k", "native"},
		{"glm-4", "native"},
		{"unknown-model", "hybrid"},
		{"some-random-model", "hybrid"},
	}

	for _, tt := range tests {
		result := GetEmulationMode(tt.model)
		if result != tt.expected {
			t.Errorf("GetEmulationMode(%s) = %s, expected %s",
				tt.model, result, tt.expected)
		}
	}
}
