// Package tool defines the executor-bearing tool contract used by the
// wingharness loop and the built-in tool implementations.
//
// The loop and storage layer also reference wingmodels.ToolDef, which is
// the wire-format schema (description + JSON Schema) sent to the model
// provider. The split is intentional:
//
//   - wingmodels.ToolDef is data that travels to the LLM. It has no
//     execute method and no idea how to run.
//   - tool.Tool is the runtime contract the loop uses to actually execute
//     a call. It owns the executor function, work-dir context, and any
//     tool-specific state.
//
// A Tool produces a wingmodels.ToolDef via Definition(). The loop
// translates [Definition] to ToolDef when building each provider request.
package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/wingmodels"
)

// Tool is the executor contract every wingharness tool implements. The loop
// dispatches tool calls by looking up Tool instances in a Registry.
//
// Sequential is consulted per tool: if any tool in a batch returns true,
// the loop runs the entire batch sequentially. Otherwise tools execute
// in parallel. Tools that mutate shared resources (e.g., the file system
// in non-idempotent ways, or the same external service with rate limits)
// should opt into sequential execution.
type Tool interface {
	Name() string
	Description() string
	Definition() Definition
	Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}

// SequentialTool is an optional interface a Tool can implement to force
// the loop into sequential execution mode for any batch it appears in.
//
// Tools that don't implement this interface are treated as parallel-safe.
// This matches pi-mono's "sequential trumps parallel per-batch" rule:
// a single sequential tool poisons the whole batch.
type SequentialTool interface {
	Tool
	Sequential() bool
}

// Definition is the JSON-Schema shaped declaration the loop sends to the
// model. It mirrors wingmodels.ToolDef but uses a typed schema struct so
// builtin tools can write definitions without wrestling with map[string]any.
//
// The loop converts Definition into wingmodels.ToolDef by reflating the
// nested schema into the open-ended map shape providers consume.
type Definition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"input_schema"`
}

// InputSchema is the JSON Schema for a tool's input. Wingman v0.1 only
// supports object-shaped inputs (the universal LLM tool-input shape).
type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

// Property describes a single field on a tool's input schema. The minimal
// JSON Schema subset (type + description + enum) covers every wingman
// builtin; more exotic schemas (anyOf, nested objects, refs) are out of
// scope for v0.1 and would require switching to a free-form map.
type Property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// AsModelToolDef converts a Definition into the open-ended ToolDef the
// model layer expects. Centralizing the conversion here keeps providers
// from having to know about the typed schema shape.
func (d Definition) AsModelToolDef() wingmodels.ToolDef {
	props := make(map[string]any, len(d.InputSchema.Properties))
	for name, p := range d.InputSchema.Properties {
		obj := map[string]any{"type": p.Type}
		if p.Description != "" {
			obj["description"] = p.Description
		}
		if len(p.Enum) > 0 {
			obj["enum"] = p.Enum
		}
		props[name] = obj
	}
	schema := map[string]any{
		"type":       d.InputSchema.Type,
		"properties": props,
	}
	if len(d.InputSchema.Required) > 0 {
		schema["required"] = d.InputSchema.Required
	}
	return wingmodels.ToolDef{
		Name:        d.Name,
		Description: d.Description,
		InputSchema: schema,
	}
}

// Registry is a thread-safe map of tool name to Tool. The loop builds a
// Registry per Run from the configured tool list; callers can also build
// one ahead of time and reuse it across runs.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds (or replaces) a tool by Name. Last write wins.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the named tool or an error wrapping ErrToolNotFound.
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrToolNotFound, name)
	}
	return t, nil
}

// List returns a snapshot of all registered tools in unspecified order.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// Definitions returns the JSON Schema declarations for every tool.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]Definition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

// ErrToolNotFound is wrapped by Registry.Get when a tool name has no
// matching registration. The loop converts this into an immediate tool
// result with isError=true rather than failing the whole turn.
var ErrToolNotFound = fmt.Errorf("tool not found")

// BaseTool is an embeddable struct that satisfies the descriptive part
// of Tool (Name/Description/Definition). Custom tools embed BaseTool and
// only have to implement Execute.
type BaseTool struct {
	name        string
	description string
	definition  Definition
}

// NewBaseTool constructs a BaseTool with the given identity and schema.
func NewBaseTool(name, description string, def Definition) BaseTool {
	return BaseTool{name: name, description: description, definition: def}
}

func (b BaseTool) Name() string           { return b.name }
func (b BaseTool) Description() string    { return b.description }
func (b BaseTool) Definition() Definition { return b.definition }

// FuncTool wraps a plain function as a Tool. Useful for one-off tools
// defined inline in user code without declaring a new type.
type FuncTool struct {
	BaseTool
	fn func(ctx context.Context, params map[string]any, workDir string) (string, error)
}

// NewFuncTool returns a FuncTool. The Definition's Name should match the
// passed name to avoid a mismatch between the LLM's tool schema view and
// the registry's lookup key.
func NewFuncTool(name, description string, def Definition,
	fn func(ctx context.Context, params map[string]any, workDir string) (string, error),
) *FuncTool {
	return &FuncTool{BaseTool: NewBaseTool(name, description, def), fn: fn}
}

// Execute delegates to the wrapped function.
func (f *FuncTool) Execute(ctx context.Context, params map[string]any, workDir string) (string, error) {
	return f.fn(ctx, params, workDir)
}

// Compile-time conformance checks for built-in tools. Adding a new
// builtin? Add it here so a signature drift fails the build instead of
// being caught at runtime by Registry.Register accepting any value
// satisfying the (then-mutated) Tool interface.
var (
	_ Tool = (*BashTool)(nil)
	_ Tool = (*EditTool)(nil)
	_ Tool = (*GlobTool)(nil)
	_ Tool = (*GrepTool)(nil)
	_ Tool = (*PerplexityTool)(nil)
	_ Tool = (*ReadTool)(nil)
	_ Tool = (*WebFetchTool)(nil)
	_ Tool = (*WriteTool)(nil)
	_ Tool = (*FuncTool)(nil)
)
