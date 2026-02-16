package memory

import "time"

// WorkingMemory 工作记忆：最近的完整对话
type WorkingMemory struct {
	SessionID  string
	Messages   []Message
	LastUpdate time.Time
	TokenCount int
}

// SessionMemory 会话记忆：当前任务的结构化信息
type SessionMemory struct {
	SessionID      string
	ProjectMeta    map[string]interface{} // 项目元数据
	FileOperations []FileOp               // 文件操作历史
	UserPrefs      map[string]string      // 用户偏好
	Summary        string                 // 会话摘要
	LastUpdate     time.Time
	TokenCount     int
}

// LongTermMemory 长期记忆：跨会话知识
type LongTermMemory struct {
	UserID         string
	CodingStyle    string           // 编码风格
	TechStack      []string         // 技术栈偏好
	ProjectHistory []ProjectSummary // 历史项目
	Embeddings     []float32        // 向量表示
	CreatedAt      time.Time
}

// Message 消息结构
type Message struct {
	Role       string
	Content    string
	Timestamp  time.Time
	TokenCount int
}

// FileOp 文件操作记录
type FileOp struct {
	Action    string // create, modify, delete
	Path      string
	Timestamp time.Time
}

// ProjectSummary 项目摘要
type ProjectSummary struct {
	Name        string
	Description string
	TechStack   []string
	Timestamp   time.Time
}

// Intent 意图类型
type Intent string

const (
	IntentCodeGeneration Intent = "code_generation"
	IntentCodeRefactor   Intent = "code_refactor"
	IntentDebug          Intent = "debug"
	IntentArchitecture   Intent = "architecture"
	IntentDocumentation  Intent = "documentation"
	IntentQuestion       Intent = "question"
	IntentOther          Intent = "other"
)

// ContextStrategy 上下文策略
type ContextStrategy struct {
	MaxWorkingMemory  int     // 工作记忆最大轮数
	CompressionRatio  float64 // 压缩比例
	SummaryFrequency  int     // 总结频率（每N轮总结一次）
	UseExpensiveModel bool    // 是否使用贵模型
}

// GetStrategyByIntent 根据意图获取策略
func GetStrategyByIntent(intent Intent) ContextStrategy {
	switch intent {
	case IntentCodeGeneration:
		return ContextStrategy{
			MaxWorkingMemory:  3,
			CompressionRatio:  0.8,
			SummaryFrequency:  5,
			UseExpensiveModel: false,
		}

	case IntentDebug:
		return ContextStrategy{
			MaxWorkingMemory:  10,
			CompressionRatio:  0.3,
			SummaryFrequency:  10,
			UseExpensiveModel: false,
		}

	case IntentArchitecture:
		return ContextStrategy{
			MaxWorkingMemory:  5,
			CompressionRatio:  0.5,
			SummaryFrequency:  7,
			UseExpensiveModel: true,
		}

	default:
		return ContextStrategy{
			MaxWorkingMemory:  5,
			CompressionRatio:  0.6,
			SummaryFrequency:  7,
			UseExpensiveModel: false,
		}
	}
}
