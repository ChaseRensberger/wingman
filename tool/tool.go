package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/core"
)

type Tool = core.Tool

type Registry struct {
	mu    sync.RWMutex
	tools map[string]core.Tool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]core.Tool),
	}
}

func (r *Registry) Register(t core.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (core.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return t, nil
}

func (r *Registry) List() []core.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]core.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

func (r *Registry) Definitions() []core.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]core.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

type BaseTool struct {
	name        string
	description string
	definition  core.ToolDefinition
}

func NewBaseTool(name, description string, def core.ToolDefinition) BaseTool {
	return BaseTool{name: name, description: description, definition: def}
}

func (b BaseTool) Name() string                    { return b.name }
func (b BaseTool) Description() string             { return b.description }
func (b BaseTool) Definition() core.ToolDefinition { return b.definition }

type ExecuteFunc struct {
	BaseTool
	fn func(ctx context.Context, params map[string]any, workDir string) (string, error)
}

func NewFuncTool(name, description string, def core.ToolDefinition,
	fn func(ctx context.Context, params map[string]any, workDir string) (string, error),
) *ExecuteFunc {
	return &ExecuteFunc{BaseTool: NewBaseTool(name, description, def), fn: fn}
}

func (e *ExecuteFunc) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	return e.fn(ctx, params, workDir)
}
