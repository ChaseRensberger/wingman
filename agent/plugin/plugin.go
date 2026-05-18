// Package plugin defines the agent plugin model: a Plugin is a
// bundle of hook installations, custom tools, and custom Part type
// registrations, packaged behind a single Install call.
//
// The motivation is packaging related extension points together. A
// single plugin can register tool gates, lifecycle observers, custom
// tools, and compaction behavior. Without an aggregating
// abstraction, equivalents in Go would each be wired separately into
// loop config, tool slice, and the global part registry — easy to do
// once, painful to compose across many plugins, and impossible to
// opt-in at the session boundary as one unit.
//
// # Composition model
//
// The loop's Hooks struct allows exactly one function per seam (single
// call site, no surprise ordering). When multiple plugins want the same
// seam, the registry composes them in install order:
//
//   - Pipeline seams (TransformHistory, TransformContext, BeforeToolCall,
//     AfterToolCall) chain: each hook receives the previous one's output.
//   - Sink subscribers run independently: every registered sink sees
//     every event.
//   - Tool registrations merge into the session's tool slice.
//   - Part registrations call models.RegisterPart directly (the
//     part registry is process-global; idempotent across re-installs).
//
// # Loading model
//
// v0.1 plugins are compile-time only: a Plugin is a Go value the
// program builds and passes to session.WithPlugin. Future versions may
// add MCP-style external plugins (for tools) and Yaegi-script plugins
// (for hooks), loaded from local files or package distributions.
//
// # Authoring
//
//	type MyPlugin struct{ /* options */ }
//
//	func New(opts ...Option) *MyPlugin { ... }
//
//	func (p *MyPlugin) Name() string { return "my-plugin" }
//
//	func (p *MyPlugin) Install(r *plugin.Registry) error {
//	    r.RegisterTransformHistory(p.transformHistory)
//	    r.RegisterTool(p.someTool)
//	    return nil
//	}
//
// Plugins should keep their identity (Name) stable across versions so
// observability layers can attribute hook activity.
package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/tool"
	"github.com/chaserensberger/wingman/models"
)

// Plugin is the aggregating abstraction. Implementations bundle hook
// installations, tools, and part registrations behind a single Install.
type Plugin interface {
	// Name is a stable identifier for the plugin. Used in error
	// messages and (later) observability. Must be unique among plugins
	// installed into the same session.
	Name() string

	// Install registers the plugin's contributions with the registry.
	// Called exactly once per session.New invocation. Errors fail
	// session construction.
	Install(*Registry) error
}

// Registry collects plugin contributions during the install phase.
// Session uses Build to fold the registry into a loop.Hooks value
// (with composed pipelines), a sink, and a merged tool slice.
//
// Registry is single-use: once Build is called, further Register* calls
// have undefined effect. Sessions construct a fresh Registry per
// activation.
type Registry struct {
	beforeRun         []loop.BeforeRunHook
	transformHistory  []loop.TransformHistoryHook
	transformContext  []loop.TransformContextHook
	beforeToolCall    []loop.BeforeToolCallFunc
	afterToolCall     []loop.AfterToolCallFunc
	afterRun          []loop.AfterRunHook
	transformToolDefs []loop.TransformToolDefsHook
	transformParams   []loop.TransformParamsHook
	sinks             []loop.Sink
	tools             []tool.Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{} }

// RegisterBeforeRun adds a BeforeRun hook. Hooks compose in install
// order: each receives the accumulated history from prior hooks and
// returns the new accumulated history. Returning nil is a no-op.
//
// The canonical user is the storage plugin (rehydrate from disk);
// other plugins layer on top (resumption markers, header context).
func (r *Registry) RegisterBeforeRun(h loop.BeforeRunHook) {
	if h != nil {
		r.beforeRun = append(r.beforeRun, h)
	}
}

// RegisterTransformHistory adds a TransformHistory hook to the pipeline. Hooks run
// in install order; each receives the previous hook's output as
// info.Messages.
func (r *Registry) RegisterTransformHistory(h loop.TransformHistoryHook) {
	if h != nil {
		r.transformHistory = append(r.transformHistory, h)
	}
}

// RegisterTransformContext adds a TransformContext hook to the
// per-turn ephemeral pipeline. Hooks run in install order.
func (r *Registry) RegisterTransformContext(h loop.TransformContextHook) {
	if h != nil {
		r.transformContext = append(r.transformContext, h)
	}
}

// RegisterBeforeToolCall adds a BeforeToolCall hook. Hooks run in
// install order; the first hook to return ErrSkipTool short-circuits
// the chain.
func (r *Registry) RegisterBeforeToolCall(h loop.BeforeToolCallFunc) {
	if h != nil {
		r.beforeToolCall = append(r.beforeToolCall, h)
	}
}

// RegisterAfterToolCall adds an AfterToolCall hook. Hooks run in
// install order; each receives the previous hook's output.
func (r *Registry) RegisterAfterToolCall(h loop.AfterToolCallFunc) {
	if h != nil {
		r.afterToolCall = append(r.afterToolCall, h)
	}
}

// RegisterAfterRun adds an AfterRun hook. Hooks run in install order;
// every registered hook sees the same Result and errors are joined.
func (r *Registry) RegisterAfterRun(h loop.AfterRunHook) {
	if h != nil {
		r.afterRun = append(r.afterRun, h)
	}
}

// RegisterTransformToolDefs adds a TransformToolDefs hook to the
// per-turn pipeline. Hooks run in install order; each receives the
// previous hook's output.
func (r *Registry) RegisterTransformToolDefs(h loop.TransformToolDefsHook) {
	if h != nil {
		r.transformToolDefs = append(r.transformToolDefs, h)
	}
}

// RegisterTransformParams adds a TransformParams hook to the per-turn
// pipeline. Hooks run in install order; each receives the previous
// hook's output.
func (r *Registry) RegisterTransformParams(h loop.TransformParamsHook) {
	if h != nil {
		r.transformParams = append(r.transformParams, h)
	}
}

// RegisterSink adds an event observer. All registered sinks receive
// every event, in install order.
func (r *Registry) RegisterSink(s loop.Sink) {
	if s != nil {
		r.sinks = append(r.sinks, s)
	}
}

// RegisterTool adds a tool to the session's tool list. Plugins may
// override built-in tools by registering with the same name; the
// session's tool slice already contains user-supplied tools when the
// registry is built, so plugin tools are appended (later wins on
// name clashes inside the loop's tool registry).
func (r *Registry) RegisterTool(t tool.Tool) {
	if t != nil {
		r.tools = append(r.tools, t)
	}
}

// RegisterPart registers a Part discriminator + decoder with the
// process-global models part registry. Plugins typically call this
// from Install so loaded sessions decode their custom parts correctly.
//
// Re-registering an existing name overwrites; safe to call across
// re-installs of the same plugin.
func (r *Registry) RegisterPart(typeName string, fn models.PartUnmarshaler) {
	models.RegisterPart(typeName, fn)
}

// Built bundles the composed hooks, merged tool slice, and aggregated
// sink that a session feeds to loop.Run. Construct via Registry.Build.
type Built struct {
	Hooks loop.Hooks
	Tools []tool.Tool
	// Sink is non-nil when at least one plugin registered a sink. The
	// session combines this with its own internal sink.
	Sink loop.Sink
}

// Build folds the registry's contributions into a Built value. The
// returned Built is independent of the registry; further mutations to
// the registry don't affect it.
func (r *Registry) Build() Built {
	hooks := loop.Hooks{}

	switch len(r.beforeRun) {
	case 0:
		// no-op
	case 1:
		hooks.BeforeRun = r.beforeRun[0]
	default:
		hooks.BeforeRun = composeBeforeRun(r.beforeRun)
	}

	switch len(r.transformHistory) {
	case 0:
		// no-op
	case 1:
		hooks.TransformHistory = r.transformHistory[0]
	default:
		hooks.TransformHistory = composeTransformHistory(r.transformHistory)
	}

	switch len(r.transformContext) {
	case 0:
	case 1:
		hooks.TransformContext = r.transformContext[0]
	default:
		hooks.TransformContext = composeTransformContext(r.transformContext)
	}

	switch len(r.beforeToolCall) {
	case 0:
	case 1:
		hooks.BeforeToolCall = r.beforeToolCall[0]
	default:
		hooks.BeforeToolCall = composeBeforeToolCall(r.beforeToolCall)
	}

	switch len(r.afterToolCall) {
	case 0:
	case 1:
		hooks.AfterToolCall = r.afterToolCall[0]
	default:
		hooks.AfterToolCall = composeAfterToolCall(r.afterToolCall)
	}

	switch len(r.afterRun) {
	case 0:
	case 1:
		hooks.AfterRun = r.afterRun[0]
	default:
		hooks.AfterRun = composeAfterRun(r.afterRun)
	}

	switch len(r.transformToolDefs) {
	case 0:
	case 1:
		hooks.TransformToolDefs = r.transformToolDefs[0]
	default:
		hooks.TransformToolDefs = composeTransformToolDefs(r.transformToolDefs)
	}

	switch len(r.transformParams) {
	case 0:
	case 1:
		hooks.TransformParams = r.transformParams[0]
	default:
		hooks.TransformParams = composeTransformParams(r.transformParams)
	}

	var sink loop.Sink
	if len(r.sinks) > 0 {
		sink = multiSink(append([]loop.Sink(nil), r.sinks...))
	}

	tools := append([]tool.Tool(nil), r.tools...)

	return Built{Hooks: hooks, Tools: tools, Sink: sink}
}

// composeBeforeRun chains BeforeRun hooks. Each receives the
// accumulated history from prior hooks and may return a new
// accumulated history. nil returns leave the accumulator unchanged.
// Errors short-circuit the chain.
func composeBeforeRun(hooks []loop.BeforeRunHook) loop.BeforeRunHook {
	return func(ctx context.Context, current []models.Message) ([]models.Message, error) {
		acc := current
		for i, h := range hooks {
			out, err := h(ctx, acc)
			if err != nil {
				return nil, fmt.Errorf("before_run[%d]: %w", i, err)
			}
			if out != nil {
				acc = out
			}
		}
		return acc, nil
	}
}

// composeTransformHistory chains TransformHistory hooks: each one's output messages
// become the next one's input. Errors short-circuit the chain.
func composeTransformHistory(hooks []loop.TransformHistoryHook) loop.TransformHistoryHook {
	return func(ctx context.Context, info loop.TransformHistoryInfo) ([]models.Message, error) {
		msgs := info.Messages
		for i, h := range hooks {
			next := info
			next.Messages = msgs
			out, err := h(ctx, next)
			if err != nil {
				return nil, fmt.Errorf("transform_history[%d]: %w", i, err)
			}
			if out != nil {
				msgs = out
			}
		}
		return msgs, nil
	}
}

// composeTransformContext chains TransformContext hooks similarly.
func composeTransformContext(hooks []loop.TransformContextHook) loop.TransformContextHook {
	return func(ctx context.Context, info loop.TransformContextInfo) ([]models.Message, error) {
		msgs := info.Messages
		for i, h := range hooks {
			next := info
			next.Messages = msgs
			out, err := h(ctx, next)
			if err != nil {
				return nil, fmt.Errorf("transform_context[%d]: %w", i, err)
			}
			if out != nil {
				msgs = out
			}
		}
		return msgs, nil
	}
}

// composeBeforeToolCall chains BeforeToolCall hooks. Each receives the
// previous hook's args (rewritten via the hook's newArgs return). The
// first hook to return any error (including ErrSkipTool) terminates
// the chain — ErrSkipTool propagates to the loop unchanged.
func composeBeforeToolCall(hooks []loop.BeforeToolCallFunc) loop.BeforeToolCallFunc {
	return func(ctx context.Context, call loop.ToolCall) (map[string]any, error) {
		args := call.Args
		for i, h := range hooks {
			next := call
			next.Args = args
			newArgs, err := h(ctx, next)
			if err != nil {
				return newArgs, fmt.Errorf("before_tool_call[%d]: %w", i, err)
			}
			if newArgs != nil {
				args = newArgs
			}
		}
		return args, nil
	}
}

// composeAfterToolCall chains AfterToolCall hooks: each receives the
// previous hook's output string and isError flag.
func composeAfterToolCall(hooks []loop.AfterToolCallFunc) loop.AfterToolCallFunc {
	return func(ctx context.Context, call loop.ToolCall, result string, isError bool) (string, error) {
		out := result
		for i, h := range hooks {
			newOut, err := h(ctx, call, out, isError)
			if err != nil {
				return out, fmt.Errorf("after_tool_call[%d]: %w", i, err)
			}
			out = newOut
		}
		return out, nil
	}
}

// composeAfterRun runs all AfterRun hooks; every hook sees the same
// Result and errors are joined.
func composeAfterRun(hooks []loop.AfterRunHook) loop.AfterRunHook {
	return func(ctx context.Context, info loop.AfterRunInfo) error {
		var errs []error
		for i, h := range hooks {
			if err := h(ctx, info); err != nil {
				errs = append(errs, fmt.Errorf("after_run[%d]: %w", i, err))
			}
		}
		return errors.Join(errs...)
	}
}

// composeTransformToolDefs chains TransformToolDefs hooks: each receives
// the previous hook's output. Errors short-circuit the chain.
func composeTransformToolDefs(hooks []loop.TransformToolDefsHook) loop.TransformToolDefsHook {
	return func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
		tools := info.Tools
		for i, h := range hooks {
			next := info
			next.Tools = tools
			out, err := h(ctx, next)
			if err != nil {
				return nil, fmt.Errorf("transform_tool_defs[%d]: %w", i, err)
			}
			tools = out
		}
		return tools, nil
	}
}

// composeTransformParams chains TransformParams hooks: each receives
// the previous hook's output. Errors short-circuit the chain.
func composeTransformParams(hooks []loop.TransformParamsHook) loop.TransformParamsHook {
	return func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
		params := info.Params
		for i, h := range hooks {
			next := info
			next.Params = params
			out, err := h(ctx, next)
			if err != nil {
				return loop.TransformParamsResult{}, fmt.Errorf("transform_params[%d]: %w", i, err)
			}
			params = out.Params
		}
		return loop.TransformParamsResult{Params: params}, nil
	}
}

// multiSink fans an event out to multiple sinks. Each sink runs
// synchronously in install order; a slow sink slows the loop. Sinks
// that need concurrency should fire-and-forget into their own goroutines.
type multiSink []loop.Sink

func (m multiSink) OnEvent(e loop.Event) {
	for _, s := range m {
		s.OnEvent(e)
	}
}
