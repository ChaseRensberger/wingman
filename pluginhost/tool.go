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
		Name:        t.spec.Name,
		Description: t.spec.Description,
		InputSchema: t.spec.InputSchema,
	}
}

func (t *rpcTool) Execute(ctx context.Context, params map[string]any, workDir string) (tool.Result, error) {
	text, metadata, err := t.manager.executeToolResult(ctx, t.pluginID, t.spec.Name, params, workDir)
	return tool.Result{Text: text, Metadata: metadata}, err
}
