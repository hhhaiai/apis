package memory_test

import (
	"context"
	"testing"

	"ccgateway/internal/memory"
)

func TestRuleBasedAnalyzer_CodeGeneration(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []struct {
		message  string
		expected memory.Intent
	}{
		{"创建一个新的API接口", memory.IntentCodeGeneration},
		{"生成一个用户管理模块", memory.IntentCodeGeneration},
		{"写一个排序算法", memory.IntentCodeGeneration},
		{"Create a new function", memory.IntentCodeGeneration},
		{"Generate a REST API", memory.IntentCodeGeneration},
	}

	for _, tc := range testCases {
		intent, confidence, err := analyzer.Analyze(ctx, tc.message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", tc.message, err)
		}
		if intent != tc.expected {
			t.Errorf("For message '%s', expected intent %s, got %s", tc.message, tc.expected, intent)
		}
		if confidence < 0.5 {
			t.Errorf("For message '%s', confidence too low: %f", tc.message, confidence)
		}
	}
}

func TestRuleBasedAnalyzer_Debug(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []struct {
		message  string
		expected memory.Intent
	}{
		{"这个代码有bug", memory.IntentDebug},
		{"为什么会报错？", memory.IntentDebug},
		{"修复这个问题", memory.IntentDebug},
		{"Fix this error", memory.IntentDebug},
		{"Debug this issue", memory.IntentDebug},
		{"The code is not working", memory.IntentDebug},
	}

	for _, tc := range testCases {
		intent, confidence, err := analyzer.Analyze(ctx, tc.message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", tc.message, err)
		}
		if intent != tc.expected {
			t.Errorf("For message '%s', expected intent %s, got %s", tc.message, tc.expected, intent)
		}
		if confidence < 0.7 {
			t.Errorf("For message '%s', confidence too low: %f", tc.message, confidence)
		}
	}
}

func TestRuleBasedAnalyzer_Refactor(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []struct {
		message  string
		expected memory.Intent
	}{
		{"重构这段代码", memory.IntentCodeRefactor},
		{"优化性能", memory.IntentCodeRefactor},
		{"改进代码质量", memory.IntentCodeRefactor},
		{"Refactor this function", memory.IntentCodeRefactor},
		{"Optimize the algorithm", memory.IntentCodeRefactor},
	}

	for _, tc := range testCases {
		intent, _, err := analyzer.Analyze(ctx, tc.message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", tc.message, err)
		}
		if intent != tc.expected {
			t.Errorf("For message '%s', expected intent %s, got %s", tc.message, tc.expected, intent)
		}
	}
}

func TestRuleBasedAnalyzer_Architecture(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []struct {
		message  string
		expected memory.Intent
	}{
		{"设计一个微服务架构", memory.IntentArchitecture},
		{"规划系统架构", memory.IntentArchitecture},
		{"Design the system architecture", memory.IntentArchitecture},
		{"Plan the project structure", memory.IntentArchitecture},
	}

	for _, tc := range testCases {
		intent, _, err := analyzer.Analyze(ctx, tc.message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", tc.message, err)
		}
		if intent != tc.expected {
			t.Errorf("For message '%s', expected intent %s, got %s", tc.message, tc.expected, intent)
		}
	}
}

func TestRuleBasedAnalyzer_Documentation(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []struct {
		message  string
		expected memory.Intent
	}{
		// "写一个"会被识别为代码生成，这是合理的
		{"README文档", memory.IntentDocumentation},
		{"注释这段代码", memory.IntentDocumentation}, // "注释"是文档关键词
		{"documentation for this project", memory.IntentDocumentation},
		{"comment this code", memory.IntentDocumentation},
	}

	for _, tc := range testCases {
		intent, _, err := analyzer.Analyze(ctx, tc.message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", tc.message, err)
		}
		if intent != tc.expected {
			t.Errorf("For message '%s', expected intent %s, got %s", tc.message, tc.expected, intent)
		}
	}
}

func TestRuleBasedAnalyzer_Question(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []struct {
		message  string
		expected memory.Intent
	}{
		{"什么是微服务？", memory.IntentQuestion},
		{"为什么要用Go语言？", memory.IntentQuestion},
		{"如何实现单例模式？", memory.IntentQuestion},
		{"What is REST API?", memory.IntentQuestion},
		// "Why" 单独会被识别为 question
		{"Why is this happening?", memory.IntentQuestion},
		{"How to deploy?", memory.IntentQuestion},
	}

	for _, tc := range testCases {
		intent, _, err := analyzer.Analyze(ctx, tc.message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", tc.message, err)
		}
		if intent != tc.expected {
			t.Errorf("For message '%s', expected intent %s, got %s", tc.message, tc.expected, intent)
		}
	}
}

func TestRuleBasedAnalyzer_Other(t *testing.T) {
	analyzer := memory.NewRuleBasedAnalyzer()
	ctx := context.Background()

	testCases := []string{
		"Hello",
		"Thanks",
		"OK",
		"继续",
	}

	for _, message := range testCases {
		intent, confidence, err := analyzer.Analyze(ctx, message)
		if err != nil {
			t.Fatalf("Analyze failed for '%s': %v", message, err)
		}
		if intent != memory.IntentOther {
			t.Errorf("For message '%s', expected intent %s, got %s", message, memory.IntentOther, intent)
		}
		if confidence > 0.6 {
			t.Errorf("For message '%s', confidence too high for 'other': %f", message, confidence)
		}
	}
}

func TestGetStrategyByIntent(t *testing.T) {
	testCases := []struct {
		intent               memory.Intent
		expectedMaxWorking   int
		expectedUseExpensive bool
	}{
		{memory.IntentCodeGeneration, 3, false},
		{memory.IntentDebug, 10, false},
		{memory.IntentArchitecture, 5, true},
		{memory.IntentCodeRefactor, 5, false},
		{memory.IntentDocumentation, 5, false},
		{memory.IntentQuestion, 5, false},
		{memory.IntentOther, 5, false},
	}

	for _, tc := range testCases {
		strategy := memory.GetStrategyByIntent(tc.intent)
		if strategy.MaxWorkingMemory != tc.expectedMaxWorking {
			t.Errorf("For intent %s, expected MaxWorkingMemory %d, got %d",
				tc.intent, tc.expectedMaxWorking, strategy.MaxWorkingMemory)
		}
		if strategy.UseExpensiveModel != tc.expectedUseExpensive {
			t.Errorf("For intent %s, expected UseExpensiveModel %v, got %v",
				tc.intent, tc.expectedUseExpensive, strategy.UseExpensiveModel)
		}
		if strategy.CompressionRatio <= 0 || strategy.CompressionRatio > 1 {
			t.Errorf("For intent %s, invalid CompressionRatio: %f", tc.intent, strategy.CompressionRatio)
		}
		if strategy.SummaryFrequency <= 0 {
			t.Errorf("For intent %s, invalid SummaryFrequency: %d", tc.intent, strategy.SummaryFrequency)
		}
	}
}
