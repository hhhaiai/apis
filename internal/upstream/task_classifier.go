package upstream

import (
	"context"
	"strings"

	"ccgateway/internal/orchestrator"
)

// ========== 任务复杂度分类 ==========

// TaskComplexity 任务复杂度
type TaskComplexity int

const (
	ComplexityUnknown TaskComplexity = iota
	ComplexityLow                  // 简单任务
	ComplexityMedium               // 中等任务
	ComplexityHigh                 // 复杂任务
	ComplexityVeryHigh             // 极高任务
)

// String 实现 Stringer 接口
func (c TaskComplexity) String() string {
	switch c {
	case ComplexityLow:
		return "low"
	case ComplexityMedium:
		return "medium"
	case ComplexityHigh:
		return "high"
	case ComplexityVeryHigh:
		return "very_high"
	default:
		return "unknown"
	}
}

// TaskClassifier 任务分类器
type TaskClassifier struct{}

// NewTaskClassifier 创建任务分类器
func NewTaskClassifier() *TaskClassifier {
	return &TaskClassifier{}
}

// ClassifyTask 分类任务复杂度
func (c *TaskClassifier) ClassifyTask(ctx context.Context, messages []orchestrator.Message) TaskComplexity {
	if len(messages) == 0 {
		return ComplexityLow
	}

	// 获取最后一条用户消息
	lastUserMsg := ""
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserMsg = c.extractText(messages[i].Content)
			break
		}
	}

	lowerMsg := strings.ToLower(lastUserMsg)

	// 极高复杂度关键词
	veryHighKeywords := []string{
		"设计一个", "架构", "从头实现", "全新系统", "重新设计",
		"design architecture", "create a new system", "implement from scratch",
	}
	for _, kw := range veryHighKeywords {
		if strings.Contains(lowerMsg, strings.ToLower(kw)) {
			return ComplexityVeryHigh
		}
	}

	// 高复杂度关键词
	highKeywords := []string{
		"帮我写", "创建一个", "实现功能", "重构", "优化",
		"帮我debug", "修复bug", "写一个完整的",
		"write code", "create a", "implement", "refactor", "debug",
	}
	for _, kw := range highKeywords {
		if strings.Contains(lowerMsg, strings.ToLower(kw)) {
			return ComplexityHigh
		}
	}

	// 中复杂度关键词
	mediumKeywords := []string{
		"解释一下", "分析一下", "这个代码", "什么意思", "怎么实现",
		"explain", "analyze", "what does this code", "how to",
	}
	for _, kw := range mediumKeywords {
		if strings.Contains(lowerMsg, strings.ToLower(kw)) {
			return ComplexityMedium
		}
	}

	return ComplexityLow
}

// extractText 从消息内容中提取文本
func (c *TaskClassifier) extractText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var sb strings.Builder
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				if text, ok := m["text"].(string); ok {
					sb.WriteString(text)
				}
			}
		}
		return sb.String()
	}
	return ""
}

// ModelCapability 模型能力
type ModelCapability struct {
	Name            string `json:"name"`
	Intelligence    int    `json:"intelligence"`    // 0-100
	CostLevel       int    `json:"cost_level"`     // 1-5, 1=最便宜
	SpeedLevel      int    `json:"speed_level"`     // 1-5, 5=最快
	SupportsTools   bool   `json:"supports_tools"`
	SupportsVision  bool   `json:"supports_vision"`
}

// ShouldEmulateTools 检查是否应该对指定模型启用工具模拟
func ShouldEmulateTools(model string, upstreamSupportsTools bool) bool {
	if upstreamSupportsTools {
		return false
	}

	model = strings.ToLower(model)

	// 已知不支持工具的模型前缀
	noToolPrefixes := []string{
		"ernie-", "glm-4-flash", "glm-4v-", "spark-", "hunyuan-",
		"baichuan", "abab6", "yi-", "qwen-turbo", "qwen-max", "qwen-long",
		"gpt-3.5",
	}

	for _, prefix := range noToolPrefixes {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}

	return false
}

// GetEmulationMode 获取指定模型的模拟模式
func GetEmulationMode(model string) string {
	model = strings.ToLower(model)

	// 支持工具的模型使用原生模式
	supportToolsPrefixes := []string{
		"qwen-plus", "glm-4", "moonshot-", "claude-", "gpt-4",
	}

	for _, prefix := range supportToolsPrefixes {
		if strings.HasPrefix(model, prefix) {
			return "native"
		}
	}

	// 其他默认使用混合模式
	return "hybrid"
}
