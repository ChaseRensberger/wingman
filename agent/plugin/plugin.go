// Package plugin defines the agent plugin model: a Plugin is a
// bundle of hook installations, custom tools, and custom Part type
// registrations, packaged behind a single activation call.
//
// The motivation is packaging related extension points together. A
// single plugin can register tool gates, lifecycle observers, custom
// tools, and compaction behavior. Without an aggregating
// abstraction, equivalents in Go would each be wired separately into
// loop config, tool slice, and a scoped part registry — easy to do
// once, painful to compose across many plugins, and impossible to
// opt-in at the session boundary as one unit.
//
// # Composition model
//
// The loop's Hooks struct allows exactly one function per seam (single
// call site, no surprise ordering). When multiple plugins want the same
// seam, the registry composes them in activation order:
//
//   - Pipeline seams (TransformHistory, TransformContext, BeforeToolCall,
//     AfterToolCall) chain: each hook receives the previous one's output.
//   - Sink subscribers run independently: every registered sink sees
//     every event.
//   - Tool registrations merge into the session's tool slice.
//   - Part registrations build a decoder generation scoped to this activation.
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
//	func (p *MyPlugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
//	    r.RegisterTransformHistory(p.transformHistory)
//	    r.RegisterTool(p.someTool)
//	    return nil, nil
//	}
//
// Plugins should keep their identity (Name) stable across versions so
// observability layers can attribute hook activity.
package plugin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/tool"
)

// Plugin is the aggregating abstraction. Implementations bundle hook
// installations, tools, and part registrations behind a single activation.
type Plugin interface {
	// Name is a stable identifier for the plugin. Used in error
	// messages and (later) observability. Must be unique among plugins
	// installed into the same session.
	Name() string

	// Activate registers this plugin's contributions and optionally returns
	// cleanup for resources acquired during activation.
	Activate(*Registry) (Cleanup, error)
}

// Cleanup releases plugin activation resources.
type Cleanup func(context.Context) error

// Registry collects plugin contributions during activation.
// Session uses Build to fold the registry into a run.Hooks value
// (with composed pipelines), a sink, and a merged tool slice.
//
// Registry is single-use: Build freezes it and subsequent registrations fail.
type Registry struct {
	beforeRun         []run.BeforeRunHook
	transformHistory  []run.TransformHistoryHook
	transformContext  []run.TransformContextHook
	beforeToolCall    []run.BeforeToolCallFunc
	afterToolCall     []run.AfterToolCallFunc
	afterRun          []run.AfterRunHook
	transformToolDefs []run.TransformToolDefsHook
	transformParams   []run.TransformParamsHook
	sinks             []sinkRegistration
	tools             []tool.Tool
	parts             []partRegistration
	built             bool
	owner             string
}

type partRegistration struct {
	owner    string
	typeName string
	decoder  models.PartUnmarshaler
}

type sinkRegistration struct {
	owner    string
	sink     run.Sink
	timeout  time.Duration
	inFlight *atomic.Bool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry { return &Registry{} }

// RegisterBeforeRun adds a BeforeRun hook. Hooks compose in activation
// order: each receives the accumulated history from prior hooks and
// returns the new accumulated history. Returning nil is a no-op.
//
// The canonical user is the storage plugin (rehydrate from disk);
// other plugins layer on top (resumption markers, header context).
func (r *Registry) RegisterBeforeRun(h run.BeforeRunHook) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.beforeRun = append(r.beforeRun, h)
	}
	return nil
}

// RegisterTransformHistory adds a TransformHistory hook to the pipeline. Hooks run
// in activation order; each receives the previous hook's output as
// info.Messages.
func (r *Registry) RegisterTransformHistory(h run.TransformHistoryHook) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.transformHistory = append(r.transformHistory, h)
	}
	return nil
}

// RegisterTransformContext adds a TransformContext hook to the
// per-turn ephemeral pipeline. Hooks run in activation order.
func (r *Registry) RegisterTransformContext(h run.TransformContextHook) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.transformContext = append(r.transformContext, h)
	}
	return nil
}

// RegisterBeforeToolCall adds a BeforeToolCall hook. Hooks run in
// activation order; the first hook to return ErrSkipTool short-circuits
// the chain.
func (r *Registry) RegisterBeforeToolCall(h run.BeforeToolCallFunc) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.beforeToolCall = append(r.beforeToolCall, h)
	}
	return nil
}

// RegisterAfterToolCall adds an AfterToolCall hook. Hooks run in
// activation order; each receives the previous hook's output.
func (r *Registry) RegisterAfterToolCall(h run.AfterToolCallFunc) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.afterToolCall = append(r.afterToolCall, h)
	}
	return nil
}

// RegisterAfterRun adds an AfterRun hook. Hooks run in activation order;
// every registered hook sees the same Result and errors are joined.
func (r *Registry) RegisterAfterRun(h run.AfterRunHook) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.afterRun = append(r.afterRun, h)
	}
	return nil
}

// RegisterTransformToolDefs adds a TransformToolDefs hook to the
// per-turn pipeline. Hooks run in activation order; each receives the
// previous hook's output.
func (r *Registry) RegisterTransformToolDefs(h run.TransformToolDefsHook) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.transformToolDefs = append(r.transformToolDefs, h)
	}
	return nil
}

// RegisterTransformParams adds a TransformParams hook to the per-turn
// pipeline. Hooks run in activation order; each receives the previous
// hook's output.
func (r *Registry) RegisterTransformParams(h run.TransformParamsHook) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if h != nil {
		r.transformParams = append(r.transformParams, h)
	}
	return nil
}

// RegisterSink adds an event observer. All registered sinks receive
// every event, in activation order.
func (r *Registry) RegisterSink(s run.Sink) error {
	return r.RegisterSinkTimeout(s, DefaultSinkTimeout)
}

// DefaultSinkTimeout bounds how long event dispatch waits for a plugin sink.
const DefaultSinkTimeout = time.Second

// RegisterSinkTimeout adds an event observer with an explicit dispatch timeout.
func (r *Registry) RegisterSinkTimeout(s run.Sink, timeout time.Duration) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if s == nil {
		return nil
	}
	if timeout <= 0 {
		return errors.New("plugin: sink timeout must be positive")
	}
	r.sinks = append(r.sinks, sinkRegistration{owner: r.owner, sink: s, timeout: timeout, inFlight: new(atomic.Bool)})
	return nil
}

// RegisterTool adds a tool to the session's tool list. The session's strict
// catalog composition rejects collisions with session or other plugin tools.
func (r *Registry) RegisterTool(t tool.Tool) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if t != nil {
		r.tools = append(r.tools, t)
	}
	return nil
}

// RegisterPart registers a Part discriminator + decoder with the
// scoped decoder generation. Duplicate custom discriminators are rejected
// when the generation is built.
func (r *Registry) RegisterPart(typeName string, fn models.PartUnmarshaler) error {
	if err := r.mutable(); err != nil {
		return err
	}
	if strings.TrimSpace(typeName) == "" {
		return errors.New("plugin: part type is empty")
	}
	if fn == nil {
		return fmt.Errorf("plugin: part decoder %q is nil", typeName)
	}
	r.parts = append(r.parts, partRegistration{owner: r.owner, typeName: typeName, decoder: fn})
	return nil
}

// Built bundles the composed hooks, merged tool slice, and aggregated
// sink that a session feeds to run.Run. Construct via Registry.Build.
type Built struct {
	Hooks run.Hooks
	Tools []tool.Tool
	// Sink is non-nil when at least one plugin registered a sink. The
	// session combines this with its own internal sink.
	Sink run.Sink
}

// Generation is one immutable plugin activation. Its runtime contributions
// and PartDecoders are scoped to this generation.
type Generation struct {
	built        Built
	partDecoders models.PartDecoders
	cleanups     []Cleanup
	closeOnce    sync.Once
	closeErr     error
}

// Runtime returns a snapshot of this generation's runtime contributions.
func (g *Generation) Runtime() Built {
	if g == nil {
		return Built{}
	}
	built := g.built
	built.Tools = append([]tool.Tool(nil), built.Tools...)
	return built
}

// Parts returns this generation's immutable part decoder set.
func (g *Generation) Parts() models.PartDecoders {
	if g == nil {
		return models.BuiltinPartDecoders()
	}
	return g.partDecoders
}

// Close releases activated plugins in reverse activation order. It is safe to
// call concurrently and returns the same joined error on every call.
func (g *Generation) Close(ctx context.Context) error {
	if g == nil {
		return nil
	}
	g.closeOnce.Do(func() {
		var errs []error
		for i := len(g.cleanups) - 1; i >= 0; i-- {
			if g.cleanups[i] != nil {
				errs = append(errs, g.cleanups[i](ctx))
			}
		}
		g.closeErr = errors.Join(errs...)
	})
	return g.closeErr
}

// ActivateAll stages the supplied plugins into one scoped generation. If an
// activation or build fails, already-acquired resources are cleaned up in
// reverse order and no process-global state is changed.
func ActivateAll(plugins ...Plugin) (*Generation, error) {
	if len(plugins) == 0 {
		return nil, errors.New("plugin: no plugins")
	}
	r := NewRegistry()
	seen := make(map[string]struct{}, len(plugins))
	cleanups := make([]Cleanup, 0, len(plugins))
	rollback := func() error {
		var errs []error
		for i := len(cleanups) - 1; i >= 0; i-- {
			if cleanups[i] != nil {
				errs = append(errs, cleanups[i](context.Background()))
			}
		}
		return errors.Join(errs...)
	}
	for i, p := range plugins {
		if isNilPlugin(p) {
			return nil, errors.Join(fmt.Errorf("plugin: plugin[%d] is nil", i), rollback())
		}
		name := strings.TrimSpace(p.Name())
		if name == "" {
			return nil, errors.Join(fmt.Errorf("plugin: plugin[%d] has an empty name", i), rollback())
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, errors.Join(fmt.Errorf("plugin: duplicate plugin %q", name), rollback())
		}
		seen[name] = struct{}{}
		r.owner = name
		cleanup, err := p.Activate(r)
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
		if err != nil {
			return nil, errors.Join(fmt.Errorf("plugin %q: %w", name, err), rollback())
		}
	}
	built, decoders, err := r.build()
	if err != nil {
		return nil, errors.Join(err, rollback())
	}
	return &Generation{built: built, partDecoders: decoders, cleanups: cleanups}, nil
}

func isNilPlugin(p Plugin) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

func (r *Registry) mutable() error {
	if r == nil {
		return errors.New("plugin: nil registry")
	}
	if r.built {
		return errors.New("plugin: registry is already built")
	}
	return nil
}

func (r *Registry) build() (Built, models.PartDecoders, error) {
	if err := r.mutable(); err != nil {
		return Built{}, models.PartDecoders{}, err
	}
	r.built = true
	partRegistry := models.NewPartRegistry()
	for _, part := range r.parts {
		if err := partRegistry.Register(part.typeName, part.decoder); err != nil {
			return Built{}, models.PartDecoders{}, fmt.Errorf("plugin %q: %w", part.owner, err)
		}
	}
	decoders, err := partRegistry.Build()
	if err != nil {
		return Built{}, models.PartDecoders{}, err
	}
	hooks := run.Hooks{}

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

	var sink run.Sink
	if len(r.sinks) > 0 {
		sink = multiSink(append([]sinkRegistration(nil), r.sinks...))
	}

	tools := append([]tool.Tool(nil), r.tools...)

	return Built{Hooks: hooks, Tools: tools, Sink: sink}, decoders, nil
}

// composeBeforeRun chains BeforeRun hooks. Each receives the
// accumulated history from prior hooks and may return a new
// accumulated history. nil returns leave the accumulator unchanged.
// Errors short-circuit the chain.
func composeBeforeRun(hooks []run.BeforeRunHook) run.BeforeRunHook {
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
func composeTransformHistory(hooks []run.TransformHistoryHook) run.TransformHistoryHook {
	return func(ctx context.Context, info run.TransformHistoryInfo) ([]models.Message, error) {
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
func composeTransformContext(hooks []run.TransformContextHook) run.TransformContextHook {
	return func(ctx context.Context, info run.TransformContextInfo) ([]models.Message, error) {
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
func composeBeforeToolCall(hooks []run.BeforeToolCallFunc) run.BeforeToolCallFunc {
	return func(ctx context.Context, call run.ToolCall) (map[string]any, error) {
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
// previous hook's structured result.
func composeAfterToolCall(hooks []run.AfterToolCallFunc) run.AfterToolCallFunc {
	return func(ctx context.Context, call run.ToolCall, result run.ToolResult) (run.ToolResult, error) {
		out := result
		for i, h := range hooks {
			newOut, err := h(ctx, call, out)
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
func composeAfterRun(hooks []run.AfterRunHook) run.AfterRunHook {
	return func(ctx context.Context, info run.AfterRunInfo) error {
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
func composeTransformToolDefs(hooks []run.TransformToolDefsHook) run.TransformToolDefsHook {
	return func(ctx context.Context, info run.TransformToolDefsInfo) ([]models.ToolDef, error) {
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
func composeTransformParams(hooks []run.TransformParamsHook) run.TransformParamsHook {
	return func(ctx context.Context, info run.TransformParamsInfo) (run.TransformParamsResult, error) {
		params := info.Params
		for i, h := range hooks {
			next := info
			next.Params = params
			out, err := h(ctx, next)
			if err != nil {
				return run.TransformParamsResult{}, fmt.Errorf("transform_params[%d]: %w", i, err)
			}
			params = out.Params
		}
		return run.TransformParamsResult{Params: params}, nil
	}
}

// multiSink fans events out in registration order. Each sink has at most one
// callback in flight. A timed-out callback keeps its slot until it returns,
// so later events are dropped rather than growing goroutines without bound.
type multiSink []sinkRegistration

func (m multiSink) OnEvent(e run.Event) {
	for _, registration := range m {
		registration.dispatch(e)
	}
}

func (r sinkRegistration) dispatch(e run.Event) {
	if !r.inFlight.CompareAndSwap(false, true) {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer r.inFlight.Store(false)
		r.sink.OnEvent(e)
	}()
	select {
	case <-done:
	case <-time.After(r.timeout):
	}
}
