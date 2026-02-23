// Package tool defines the Tool interface (re-exported from core), the tool
// Registry, and all built-in tool constructors.
package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/core"
)

// Tool is the interface every Wingman tool must implement.
// Re-exported from core for convenience.
type Tool = core.Tool

// Registry is a thread-safe map of tool name → Tool implementation.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]core.Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]core.Tool),
	}
}

// Register adds a tool to the registry, keyed by its Name().
func (r *Registry) Register(t core.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the tool with the given name, or an error if not found.
func (r *Registry) Get(name string) (core.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return t, nil
}

// List returns all registered tools.
func (r *Registry) List() []core.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]core.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// Definitions returns the ToolDefinition for each registered tool, suitable
// for including in an InferenceRequest.
func (r *Registry) Definitions() []core.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]core.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// ============================================================
//  BaseTool provides a convenience base for implementing tools.
// ============================================================

// BaseTool can be embedded to satisfy the name/description/definition parts of
// the Tool interface, leaving only Execute for the implementer.
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

// ExecuteFunc is a functional tool implementation — embed BaseTool and provide
// the execution logic as a closure. Useful for simple tools and testing.
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
