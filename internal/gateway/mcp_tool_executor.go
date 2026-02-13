package gateway

import (
	"context"
	"errors"

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
	remote, err := e.mcp.CallToolAny(ctx, call.Name, call.Input)
	if err != nil {
		return toolruntime.Result{}, err
	}
	return toolruntime.Result{
		Content: remote.Content,
		IsError: remote.IsError,
	}, nil
}
