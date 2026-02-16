package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ccgateway/internal/mcpregistry"
	"ccgateway/internal/requestctx"
	"ccgateway/internal/toolruntime"
)

type mcpAwareExecutor struct {
	local toolruntime.Executor
	mcp   MCPRegistry
}

func newMCPAwareExecutor(local toolruntime.Executor, mcp MCPRegistry) toolruntime.Executor {
	return &mcpAwareExecutor{
		local: local,
		mcp:   mcp,
	}
}

func (e *mcpAwareExecutor) Execute(ctx context.Context, call toolruntime.Call) (toolruntime.Result, error) {
	if e.local != nil {
		out, err := e.local.Execute(ctx, call)
		if err == nil {
			return out, nil
		}
		if !errors.Is(err, toolruntime.ErrToolNotImplemented) {
			return toolruntime.Result{}, err
		}
	}
	if e.mcp == nil {
		return toolruntime.Result{}, toolruntime.ErrToolNotImplemented
	}
	remote, err := callScopedMCPToolAny(ctx, e.mcp, call.Name, call.Input)
	if err != nil {
		return toolruntime.Result{}, err
	}
	return toolruntime.Result{
		Content: remote.Content,
		IsError: remote.IsError,
	}, nil
}

func callScopedMCPToolAny(ctx context.Context, registry MCPRegistry, name string, input map[string]any) (mcpregistry.ToolCallResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return mcpregistry.ToolCallResult{}, fmt.Errorf("tool name is required")
	}
	projectID := requestctx.ProjectID(ctx)
	servers := registry.List(0)
	var lastErr error
	for _, server := range servers {
		if !server.Enabled || !mcpServerBelongsToProject(projectID, server) {
			continue
		}
		tools, err := registry.ListTools(ctx, server.ID)
		if err != nil {
			lastErr = err
			continue
		}
		if !hasMCPToolName(tools, name) {
			continue
		}
		out, err := registry.CallTool(ctx, server.ID, name, input)
		if err != nil {
			lastErr = err
			continue
		}
		return out, nil
	}
	if lastErr != nil {
		return mcpregistry.ToolCallResult{}, lastErr
	}
	return mcpregistry.ToolCallResult{}, fmt.Errorf("%w: %s", mcpregistry.ErrToolNotFound, name)
}

func hasMCPToolName(tools []mcpregistry.Tool, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return false
	}
	for _, item := range tools {
		if strings.ToLower(strings.TrimSpace(item.Name)) == name {
			return true
		}
	}
	return false
}
