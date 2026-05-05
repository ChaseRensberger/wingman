// Package plugin defines the wingagent plugin model: a Plugin is a
// bundle of hook installations, custom tools, and custom Part type
// registrations, packaged behind a single Install call.
//
// The motivation is opencode parity. Opencode's plugin system lets a
// single npm package register tool gates, lifecycle observers, custom
// tools, and compaction behavior together. Without an aggregating
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
//   - Pipeline seams (BeforeStep, TransformContext, BeforeToolCall,
//     AfterToolCall) chain: each hook receives the previous one's output.
//   - Sink subscribers run independently: every registered sink sees
//     every event.
//   - Tool registrations merge into the session's tool slice.
//   - Part registrations call wingmodels.RegisterPart directly (the
//     part registry is process-global; idempotent across re-installs).
//
// # Loading model
//
// v0.1 plugins are compile-time only: a Plugin is a Go value the
// program builds and passes to session.WithPlugin. Future versions may
// add MCP-style external plugins (for tools) and Yaegi-script plugins
// (for hooks), matching opencode's npm/local file loading.
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
//	    r.RegisterBeforeStep(p.beforeStep)
//	    r.RegisterTool(p.someTool)
//	    return nil
//	}
//
// Plugins should keep their identity (Name) stable across versions so
// observability layers can attribute hook activity.
package plugin

import (
	"context"
	"fmt"

	"github.com/chaserensberger/wingman/wingagent/loop"
	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels"
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
	beforeRun        []loop.BeforeRunHook
	beforeStep       []loop.BeforeStepHook
	transformContext []loop.TransformContextHook
	beforeToolCall   []loop.BeforeToolCallFunc
	afterToolCall    []loop.AfterToolCallFunc
	sinks            []loop.Sink
	tools            []tool.Tool
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

// RegisterBeforeStep adds a BeforeStep hook to the pipeline. Hooks run
// in install order; each receives the previous hook's output as
// info.Messages.
func (r *Registry) RegisterBeforeStep(h loop.BeforeStepHook) {
	if h != nil {
		r.beforeStep = append(r.beforeStep, h)
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
// process-global wingmodels part registry. Plugins typically call this
// from Install so loaded sessions decode their custom parts correctly.
//
// Re-registering an existing name overwrites; safe to call across
// re-installs of the same plugin.
func (r *Registry) RegisterPart(typeName string, fn wingmodels.PartUnmarshaler) {
	wingmodels.RegisterPart(typeName, fn)
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

	switch len(r.beforeStep) {
	case 0:
		// no-op
	case 1:
		hooks.BeforeStep = r.beforeStep[0]
	default:
		hooks.BeforeStep = composeBeforeStep(r.beforeStep)
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
	return func(ctx context.Context, current []wingmodels.Message) ([]wingmodels.Message, error) {
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

// composeBeforeStep chains BeforeStep hooks: each one's output messages
// become the next one's input. Errors short-circuit the chain.
func composeBeforeStep(hooks []loop.BeforeStepHook) loop.BeforeStepHook {
	return func(ctx context.Context, info loop.BeforeStepInfo) ([]wingmodels.Message, error) {
		msgs := info.Messages
		for i, h := range hooks {
			next := info
			next.Messages = msgs
			out, err := h(ctx, next)
			if err != nil {
				return nil, fmt.Errorf("before_step[%d]: %w", i, err)
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
	return func(ctx context.Context, info loop.TransformContextInfo) ([]wingmodels.Message, error) {
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

// multiSink fans an event out to multiple sinks. Each sink runs
// synchronously in install order; a slow sink slows the loop. Sinks
// that need concurrency should fire-and-forget into their own goroutines.
type multiSink []loop.Sink

func (m multiSink) OnEvent(e loop.Event) {
	for _, s := range m {
		s.OnEvent(e)
	}
}
