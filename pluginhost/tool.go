package pluginhost

import (
	"context"

	"github.com/chaserensberger/wingman/tool"
)

type rpcTool struct {
	manager  *Manager
	pluginID string
	spec     ToolSpec
}

func (t *rpcTool) Name() string { return t.spec.Name }

func (t *rpcTool) Description() string { return t.spec.Description }

func (t *rpcTool) Definition() tool.Definition {
	return tool.Definition{
		Name:            t.spec.Name,
		Description:     t.spec.Description,
		InputSchema:     t.spec.InputSchema,
		OutputSchema:    t.spec.OutputSchema,
		Sequential:      t.spec.Sequential,
		DirectoryScoped: t.spec.DirectoryScoped,
		Permission:      t.spec.Permission,
	}
}

func (t *rpcTool) Execute(ctx context.Context, inv tool.Invocation) (tool.Result, error) {
	return t.manager.executeToolResult(ctx, t.pluginID, t.spec.Name, inv.Input, inv.WorkDir)
}
