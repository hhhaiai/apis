package memory

import (
	"context"
	"strings"
)

// IntentAnalyzer 意图分析器接口
type IntentAnalyzer interface {
	Analyze(ctx context.Context, message string) (Intent, float64, error)
}

// RuleBasedAnalyzer 基于规则的意图分析器（快速、免费）
type RuleBasedAnalyzer struct{}

// NewRuleBasedAnalyzer 创建规则分析器
func NewRuleBasedAnalyzer() *RuleBasedAnalyzer {
	return &RuleBasedAnalyzer{}
}

// Analyze 分析意图（按优先级顺序检查）
func (a *RuleBasedAnalyzer) Analyze(ctx context.Context, message string) (Intent, float64, error) {
	lower := strings.ToLower(message)

	// 优先级1: 调试（最高优先级，因为错误关键词很明确）
	debugKeywords := []string{
		"错误", "bug", "调试", "修复", "不工作", "失败", "报错",
		"error", "debug", "fix", "not working", "failed", "issue",
	}
	if containsAny(lower, debugKeywords) {
		return IntentDebug, 0.9, nil
	}

	// 优先级2: 代码重构
	refactorKeywords := []string{
		"重构", "优化", "改进", "重写",
		"refactor", "optimize", "improve", "rewrite",
	}
	if containsAny(lower, refactorKeywords) {
		return IntentCodeRefactor, 0.8, nil
	}

	// 优先级3: 架构设计
	architectureKeywords := []string{
		"架构", "设计", "方案", "规划",
		"architecture", "design", "plan", "structure",
	}
	if containsAny(lower, architectureKeywords) {
		return IntentArchitecture, 0.7, nil
	}

	// 优先级4: 文档（需要更精确的匹配）
	docKeywords := []string{
		"文档", "注释", "说明", "readme",
		"documentation", "comment", "doc",
	}
	// 排除"写一个"这种代码生成的关键词
	if containsAny(lower, docKeywords) && !strings.Contains(lower, "写一个") && !strings.Contains(lower, "write a") {
		return IntentDocumentation, 0.8, nil
	}

	// 优先级5: 代码生成
	codeGenKeywords := []string{
		"创建", "生成", "写一个", "新建", "添加",
		"create", "generate", "write", "add", "new",
	}
	if containsAny(lower, codeGenKeywords) {
		return IntentCodeGeneration, 0.8, nil
	}

	// 优先级6: 问题（最低优先级，因为"为什么"可能是调试的一部分）
	questionKeywords := []string{
		"什么", "如何", "怎么", "吗",
		"what", "how", "can", "is", "?",
	}
	// "为什么"单独处理，可能是调试
	if strings.Contains(lower, "为什么") || strings.Contains(lower, "why") {
		// 如果同时包含错误相关词，归类为调试
		if containsAny(lower, []string{"错误", "bug", "报错", "error", "failed"}) {
			return IntentDebug, 0.85, nil
		}
		return IntentQuestion, 0.7, nil
	}
	if containsAny(lower, questionKeywords) {
		return IntentQuestion, 0.7, nil
	}

	return IntentOther, 0.5, nil
}

// containsAny 检查字符串是否包含任意关键词
func containsAny(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}
