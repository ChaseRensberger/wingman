// Package tool defines the executor-bearing tool contract used by the
// agent loop and the built-in tool implementations.
//
// The loop and storage layer also reference models.ToolDef, which is
// the wire-format schema (description + JSON Schema) sent to the model
// provider. The split is intentional:
//
//   - models.ToolDef is data that travels to the LLM. It has no
//     execute method and no idea how to run.
//   - tool.Tool is the runtime contract the loop uses to actually execute
//     a call. It owns the executor function, work-dir context, and any
//     tool-specific state.
//
// A Tool produces a models.ToolDef via Definition(). The loop
// translates [Definition] to ToolDef when building each provider request.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/chaserensberger/wingman/models"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Tool is the executor contract every agent tool implements. The loop
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
	Execute(ctx context.Context, inv Invocation) (Result, error)
}

// Result is the structured outcome a tool returns. Text is model-facing output;
// Structured and Metadata are optional client-facing data for richer renderers.
type Result struct {
	Text       string         `json:"text"`
	Structured any            `json:"structured,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Progress is the callback a tool may use to stream intermediate output
// and JSON-serializable metadata updates during execution.
type Progress struct {
	report func(delta string, metadata map[string]any)
}

// NewProgress returns a Progress that delegates to the supplied callback.
// Nil callbacks are no-ops.
func NewProgress(report func(delta string, metadata map[string]any)) *Progress {
	return &Progress{report: report}
}

// Report emits an output delta and/or metadata update. Safe to call from
// any goroutine; nil receivers are no-ops.
func (p *Progress) Report(delta string, metadata map[string]any) {
	if p != nil && p.report != nil {
		p.report(delta, cloneProgressMetadata(metadata))
	}
}

func cloneProgressMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		return nil
	}
	var cloned map[string]any
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil
	}
	return cloned
}

// Invocation carries everything a tool needs to execute a single call.
type Invocation struct {
	Input       map[string]any
	WorkDir     string
	SessionID   string
	RunID       string
	AgentID     string
	ToolUseID   string
	CallID      string
	MessageID   string
	PartID      string
	ModelCallID string
	Progress    *Progress
}

// SequentialTool is an optional interface a Tool can implement to force
// the loop into sequential execution mode for any batch it appears in.
//
// Tools that don't implement this interface are treated as parallel-safe.
type SequentialTool interface {
	Tool
	Sequential() bool
}

// PermissionCheck describes the action/resources a tool call needs before it
// executes. Tools with non-trivial inputs, such as apply_patch touching multiple
// files, implement PermissionedTool to avoid coarse fallback matching.
type PermissionCheck struct {
	Action    string
	Resources []string
	Save      []string
}

// PermissionedTool is an optional interface for tools that can describe their
// own permission target from validated input parameters.
type PermissionedTool interface {
	Tool
	Permission(inv Invocation) (PermissionCheck, error)
}

// DirectoryScopedTool is a marker interface for tools that operate on the
// local filesystem. The session start path validates that if any allowed tool
// implements this marker, the session has a non-empty working directory.
type DirectoryScopedTool interface {
	Tool
	DirectoryScoped()
}

// PermissionTarget declares the permission action and input fields that
// identify resources for a tool invocation.
type PermissionTarget struct {
	Action         string   `json:"action"`
	ResourceFields []string `json:"resource_fields,omitempty"`
}

// Definition is the JSON-Schema shaped declaration the loop sends to the
// model. It mirrors models.ToolDef but uses a typed schema struct so
// builtin tools can write definitions without wrestling with map[string]any.
//
// The loop converts Definition into models.ToolDef by reflating the
// nested schema into the open-ended map shape providers consume.
type Definition struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"input_schema"`
	// RawInputSchema carries schemas from external protocols such as MCP that
	// exceed Wingman's small typed schema subset. If set, it takes precedence
	// when building the model-facing tool definition.
	RawInputSchema map[string]any `json:"-"`
	// OutputSchema validates Result.Structured after a successful execution.
	OutputSchema map[string]any `json:"output_schema,omitempty"`
	// Sequential requests sequential execution when the tool does not implement
	// SequentialTool.
	Sequential bool `json:"sequential,omitempty"`
	// DirectoryScoped requires a working directory when the tool does not
	// implement DirectoryScopedTool.
	DirectoryScoped bool `json:"directory_scoped,omitempty"`
	// Permission declares a permission target when the tool does not implement
	// PermissionedTool.
	Permission *PermissionTarget `json:"permission,omitempty"`
}

// IsSequential reports whether t requires sequential execution. SequentialTool
// takes precedence over Definition.Sequential.
func IsSequential(t Tool) bool {
	if sequential, ok := t.(SequentialTool); ok {
		return sequential.Sequential()
	}
	return t.Definition().Sequential
}

// IsDirectoryScoped reports whether t requires a working directory.
// DirectoryScopedTool takes precedence over Definition.DirectoryScoped.
func IsDirectoryScoped(t Tool) bool {
	if _, ok := t.(DirectoryScopedTool); ok {
		return true
	}
	return t.Definition().DirectoryScoped
}

// PermissionFor derives a permission check from t and inv. It reports false
// when neither the optional interface nor Definition declares a target.
// PermissionedTool takes precedence over Definition.Permission.
func PermissionFor(t Tool, inv Invocation) (PermissionCheck, bool, error) {
	if permissioned, ok := t.(PermissionedTool); ok {
		check, err := permissioned.Permission(inv)
		return check, true, err
	}
	target := t.Definition().Permission
	if target == nil {
		return PermissionCheck{}, false, nil
	}
	resources := make([]string, 0, len(target.ResourceFields))
	for _, field := range target.ResourceFields {
		switch value := inv.Input[field].(type) {
		case string:
			if value != "" {
				resources = append(resources, value)
			}
		case []string:
			for _, resource := range value {
				if resource != "" {
					resources = append(resources, resource)
				}
			}
		case []any:
			for _, resource := range value {
				if resource, ok := resource.(string); ok && resource != "" {
					resources = append(resources, resource)
				}
			}
		}
	}
	if len(resources) == 0 {
		resources = []string{"*"}
	}
	return PermissionCheck{Action: target.Action, Resources: resources}, true, nil
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
func (d Definition) AsModelToolDef() models.ToolDef {
	if d.RawInputSchema != nil {
		return models.ToolDef{
			Name:        d.Name,
			Description: d.Description,
			InputSchema: d.RawInputSchema,
		}
	}
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
	return models.ToolDef{
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

// Register adds t to the registry. It rejects invalid and duplicate tools.
func (r *Registry) Register(t Tool) error {
	if err := Validate(t); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[t.Name()]; exists {
		return &ValidationError{Kind: "duplicate", Name: t.Name(), Err: ErrDuplicateTool}
	}
	r.tools[t.Name()] = t
	return nil
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

// List returns a snapshot of all registered tools ordered by name.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	return tools
}

// Definitions returns the JSON Schema declarations for every tool.
func (r *Registry) Definitions() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := r.listLocked()
	defs := make([]Definition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, t.Definition())
	}
	return defs
}

func (r *Registry) listLocked() []Tool {
	tools := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name() < tools[j].Name() })
	return tools
}

// ErrToolNotFound is wrapped by Registry.Get when a tool name has no
// matching registration. The loop converts this into an immediate tool
// result with isError=true rather than failing the whole turn.
var ErrToolNotFound = fmt.Errorf("tool not found")

// ErrInvalidTool identifies a nil, unnamed, or inconsistently declared tool.
var ErrInvalidTool = errors.New("invalid tool")

// ErrDuplicateTool identifies an attempt to register the same name twice.
var ErrDuplicateTool = errors.New("duplicate tool")

// ErrResultValidation identifies structured output that violates a tool's
// declared output schema.
var ErrResultValidation = errors.New("tool result validation")

// ValidationError describes a tool registration failure.
type ValidationError struct {
	Kind string
	Name string
	Err  error
}

func (e *ValidationError) Error() string {
	if e.Name == "" {
		return fmt.Sprintf("tool %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("tool %s %q: %v", e.Kind, e.Name, e.Err)
}

func (e *ValidationError) Unwrap() error { return e.Err }

// ResultValidationError describes a successful execution whose structured
// result is missing or invalid for the tool's output schema.
type ResultValidationError struct {
	Tool string
	Err  error
}

func (e *ResultValidationError) Error() string {
	return fmt.Sprintf("tool %q result validation error: %v", e.Tool, e.Err)
}

func (e *ResultValidationError) Unwrap() error { return errors.Join(ErrResultValidation, e.Err) }

// Validate checks that t has a non-empty, consistent runtime and declared name.
func Validate(t Tool) error {
	if t == nil || isNilTool(t) {
		return &ValidationError{Kind: "nil", Err: ErrInvalidTool}
	}
	name := t.Name()
	if name == "" {
		return &ValidationError{Kind: "empty name", Err: ErrInvalidTool}
	}
	definition := t.Definition()
	if definition.Name != name {
		return &ValidationError{Kind: "name mismatch", Name: name, Err: ErrInvalidTool}
	}
	if definition.Permission != nil {
		if definition.Permission.Action == "" {
			return &ValidationError{Kind: "permission action", Name: name, Err: ErrInvalidTool}
		}
		for _, field := range definition.Permission.ResourceFields {
			if field == "" {
				return &ValidationError{Kind: "permission resource field", Name: name, Err: ErrInvalidTool}
			}
		}
	}
	if schema := definition.AsModelToolDef().InputSchema; definition.RawInputSchema != nil || definition.InputSchema.Type != "" {
		if err := compileSchema(name+"-input.json", schema); err != nil {
			return &ValidationError{Kind: "input schema", Name: name, Err: errors.Join(ErrInvalidTool, err)}
		}
	}
	if len(definition.OutputSchema) > 0 {
		if err := compileSchema(name+"-output.json", definition.OutputSchema); err != nil {
			return &ValidationError{Kind: "output schema", Name: name, Err: errors.Join(ErrInvalidTool, err)}
		}
	}
	return nil
}

func compileSchema(resource string, schema map[string]any) error {
	b, err := json.Marshal(schema)
	if err != nil {
		return err
	}
	var normalized map[string]any
	if err := json.Unmarshal(b, &normalized); err != nil {
		return err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(resource, normalized); err != nil {
		return err
	}
	_, err = c.Compile(resource)
	return err
}

func isNilTool(t Tool) bool {
	v := reflect.ValueOf(t)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

// Compose validates and registers tools as one deterministic catalog.
func Compose(tools []Tool) (*Registry, error) {
	registry := NewRegistry()
	for _, t := range tools {
		if err := registry.Register(t); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

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
	fn func(ctx context.Context, inv Invocation) (Result, error)
}

// NewFuncTool returns a FuncTool. The Definition's Name should match the
// passed name to avoid a mismatch between the LLM's tool schema view and
// the registry's lookup key.
func NewFuncTool(name, description string, def Definition,
	fn func(ctx context.Context, inv Invocation) (Result, error),
) *FuncTool {
	return &FuncTool{BaseTool: NewBaseTool(name, description, def), fn: fn}
}

// Execute delegates to the wrapped function.
func (f *FuncTool) Execute(ctx context.Context, inv Invocation) (Result, error) {
	return f.fn(ctx, inv)
}

// Compile-time conformance checks for built-in tools. Adding a new
// builtin? Add it here so a signature drift fails the build instead of
// being caught at runtime by Registry.Register accepting any value
// satisfying the (then-mutated) Tool interface.
var (
	_ Tool                = (*BashTool)(nil)
	_ Tool                = (*ApplyPatchTool)(nil)
	_ Tool                = (*EditTool)(nil)
	_ Tool                = (*GlobTool)(nil)
	_ Tool                = (*GrepTool)(nil)
	_ Tool                = (*ReadTool)(nil)
	_ Tool                = (*WebFetchTool)(nil)
	_ Tool                = (*WriteTool)(nil)
	_ Tool                = (*FuncTool)(nil)
	_ DirectoryScopedTool = (*ApplyPatchTool)(nil)
	_ DirectoryScopedTool = (*BashTool)(nil)
	_ DirectoryScopedTool = (*EditTool)(nil)
	_ DirectoryScopedTool = (*GlobTool)(nil)
	_ DirectoryScopedTool = (*GrepTool)(nil)
	_ DirectoryScopedTool = (*ReadTool)(nil)
	_ DirectoryScopedTool = (*WriteTool)(nil)
)
