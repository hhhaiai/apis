package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ccgateway/internal/toolruntime"
)

// MCPToolAdapter 将 MCP 服务器的工具转换为 Claude Code Tools 格式
type MCPToolAdapter struct {
	mcpRegistry MCPRegistry
	executor    toolruntime.Executor
}

// NewMCPToolAdapter creates a new MCP tool adapter
func NewMCPToolAdapter(mcpRegistry MCPRegistry, executor toolruntime.Executor) *MCPToolAdapter {
	return &MCPToolAdapter{
		mcpRegistry: mcpRegistry,
		executor:    executor,
	}
}

// ToolDef 定义工具
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// GetToolsForRequest 获取可用于请求的 Tools（合并本地 + MCP）
func (a *MCPToolAdapter) GetToolsForRequest(ctx context.Context) ([]ToolDef, error) {
	var allTools []ToolDef

	// 添加 MCP 工具
	if a.mcpRegistry != nil {
		mcpTools, err := a.getMCPTools(ctx)
		if err != nil {
			return nil, err
		}
		allTools = append(allTools, mcpTools...)
	}

	return allTools, nil
}

// getMCPTools 获取所有 MCP 服务器的工具
func (a *MCPToolAdapter) getMCPTools(ctx context.Context) ([]ToolDef, error) {
	if a.mcpRegistry == nil {
		return nil, nil
	}

	servers := a.mcpRegistry.List(100)
	var tools []ToolDef

	projectID := projectIDFromContext(ctx)
	for _, server := range servers {
		if !mcpServerBelongsToProject(projectID, server) {
			continue
		}
		serverTools, err := a.mcpRegistry.ListTools(ctx, server.ID)
		if err != nil {
			continue // 跳过错误的服务器
		}

		for _, t := range serverTools {
			tool := ToolDef{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}
			tools = append(tools, tool)
		}
	}

	return tools, nil
}

// ExecuteTool 执行工具（优先本地，然后 MCP）
func (a *MCPToolAdapter) ExecuteTool(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
	// 尝试本地执行
	if a.executor != nil {
		result, err := a.executor.Execute(ctx, call)
		if err == nil {
			return result, nil
		}
		// 如果是"未实现"错误，继续尝试 MCP
		if !isToolNotImplementedError(err) {
			return result, err
		}
	}

	// 尝试 MCP 执行
	if a.mcpRegistry != nil {
		result, err := callScopedMCPToolAny(ctx, a.mcpRegistry, call.Name, call.Input)
		if err == nil {
			return toolruntime.Result{
				Content: result.Content,
				IsError: result.IsError,
			}, nil
		}
		return toolruntime.Result{}, err
	}

	return toolruntime.Result{}, fmt.Errorf("tool not found: %s", call.Name)
}

func isToolNotImplementedError(err error) bool {
	return strings.Contains(err.Error(), "not implemented") ||
		strings.Contains(err.Error(), "not found")
}

// MCPToolExecutor 专门执行 MCP 工具的执行器
type MCPToolExecutor struct {
	mcpRegistry MCPRegistry
}

// NewMCPToolExecutor creates a new MCP tool executor
func NewMCPToolExecutor(mcpRegistry MCPRegistry) *MCPToolExecutor {
	return &MCPToolExecutor{
		mcpRegistry: mcpRegistry,
	}
}

// Execute 执行 MCP 工具
func (e *MCPToolExecutor) Execute(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
	if e.mcpRegistry == nil {
		return toolruntime.Result{}, fmt.Errorf("MCP registry not configured")
	}

	result, err := callScopedMCPToolAny(ctx, e.mcpRegistry, call.Name, call.Input)
	if err != nil {
		return toolruntime.Result{}, err
	}

	return toolruntime.Result{
		Content: result.Content,
		IsError: result.IsError,
	}, nil
}

// ToolCatalogMerger 合并多个工具目录
type ToolCatalogMerger struct {
	localCatalog *LocalToolCatalog
	mcpAdapter   *MCPToolAdapter
}

// NewToolCatalogMerger creates a new tool catalog merger
func NewToolCatalogMerger(mcpRegistry MCPRegistry, executor toolruntime.Executor) *ToolCatalogMerger {
	return &ToolCatalogMerger{
		localCatalog: DefaultLocalTools(),
		mcpAdapter:   NewMCPToolAdapter(mcpRegistry, executor),
	}
}

// GetAllTools 获取所有工具（本地 + MCP）
func (m *ToolCatalogMerger) GetAllTools(ctx context.Context) ([]ToolDef, error) {
	var allTools []ToolDef

	// 本地工具
	if m.localCatalog != nil {
		localTools := m.localCatalog.Snapshot()
		allTools = append(allTools, localTools...)
	}

	// MCP 工具
	if m.mcpAdapter != nil {
		mcpTools, err := m.mcpAdapter.getMCPTools(ctx)
		if err == nil {
			allTools = append(allTools, mcpTools...)
		}
	}

	return allTools, nil
}

// LocalToolCatalog 本地工具目录
type LocalToolCatalog struct {
	tools map[string]ToolDef
}

// NewLocalToolCatalog creates a new local tool catalog
func NewLocalToolCatalog() *LocalToolCatalog {
	return &LocalToolCatalog{
		tools: make(map[string]ToolDef),
	}
}

// Register 注册本地工具
func (c *LocalToolCatalog) Register(def ToolDef) {
	if def.Name != "" {
		c.tools[def.Name] = def
	}
}

// Get 获取工具定义
func (c *LocalToolCatalog) Get(name string) (ToolDef, bool) {
	def, ok := c.tools[name]
	return def, ok
}

// List 列出所有工具
func (c *LocalToolCatalog) List() []ToolDef {
	result := make([]ToolDef, 0, len(c.tools))
	for _, t := range c.tools {
		result = append(result, t)
	}
	return result
}

// Snapshot 获取所有工具快照
func (c *LocalToolCatalog) Snapshot() []ToolDef {
	return c.List()
}

// DefaultLocalTools 注册默认的本地工具
func DefaultLocalTools() *LocalToolCatalog {
	catalog := NewLocalToolCatalog()

	// 文件操作工具
	catalog.Register(ToolDef{
		Name:        "file_read",
		Description: "Read the contents of a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The path to the file to read",
				},
			},
			"required": []string{"path"},
		},
	})

	catalog.Register(ToolDef{
		Name:        "file_write",
		Description: "Write content to a file",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The path to the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
	})

	catalog.Register(ToolDef{
		Name:        "file_list",
		Description: "List files in a directory",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The directory path to list",
				},
			},
			"required": []string{"path"},
		},
	})

	// 网络工具
	catalog.Register(ToolDef{
		Name:        "web_search",
		Description: "Search the web for information",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "The search query",
				},
			},
			"required": []string{"query"},
		},
	})

	// 天气工具
	catalog.Register(ToolDef{
		Name:        "get_weather",
		Description: "Get weather information for a location",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"city": map[string]any{
					"type":        "string",
					"description": "The city to get weather for",
				},
			},
			"required": []string{"city"},
		},
	})

	return catalog
}

// convertJSONToMap 将 JSON 转换为 map
func convertJSONToMap(jsonStr string) (map[string]any, error) {
	var result map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, err
	}
	return result, nil
}
