// Package session is a thin stateful wrapper over agent/loop.
//
// A Session owns:
//   - an identifier (ULID)
//   - a working directory passed to tool executions
//   - a models.Model + system prompt + tool registry
//   - the running message history
//   - optional lifecycle hooks (BeforeStep / TransformContext)
//
// Session itself is concurrency-safe (mu-guarded). Run and RunStream
// drive a single inference loop turn batch and append both the user
// message and any new assistant/tool messages produced by the loop into
// the session's running history.
//
// Plugins (agent/plugin) are opt-in: nothing is installed by
// default. Pass WithPlugin(compaction.New()) to enable summarization;
// pass any other plugin to extend behavior at the BeforeStep,
// TransformContext, BeforeToolCall, AfterToolCall, Sink, Tool, or
// Part-registry seams. WithBeforeStep / WithTransformContext remain
// available for power users who want to install one-off hooks without
// the plugin bundle.
//
// Session is deliberately minimal: it owns no persistence and no
// transport. The caller (typically server) wires those in by
// reading History() after Run returns or by attaching its own sink via
// a plugin.
package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
	"github.com/chaserensberger/wingman/models"
)

// Session is a single conversation. Construct with New.
type Session struct {
	id      string
	workDir string
	model   models.Model
	system  string
	tools   []tool.Tool

	// Plugins installed via WithPlugin. Composed into Built at Run
	// time so the session sees the model that was set most recently
	// (model can change via SetModel between turns).
	plugins []plugin.Plugin

	// Raw hook overrides installed via WithBeforeStep / WithTransformContext.
	// These run *after* plugin-contributed hooks (last wins for transform
	// pipelines), so a user-supplied hook always has the final word.
	beforeStep       loop.BeforeStepHook
	transformContext loop.TransformContextHook

	// messageSink, if non-nil, is invoked for every loop MessageEvent
	// (including plugin-injected messages such as compaction markers
	// emitted via info.Sink). Servers wire this to store.AppendMessage
	// for incremental persistence.
	messageSink func(models.Message)

	// outputSchema, if non-nil, constrains the assistant's reply on every
	// loop turn to a JSON document conforming to the schema. See
	// WithOutputSchema for details.
	outputSchema *models.OutputSchema

	history []models.Message
	mu      sync.RWMutex
}

// Option configures a new Session.
type Option func(*Session)

// New returns a Session with a freshly minted KSUID (ses_ prefix) and
// the supplied options applied. A new Session has an empty history and
// no model; Run/RunStream will return ErrNoModel until WithModel (or
// SetModel) is applied.
//
// Plugins are opt-in. A bare New() session runs the loop with no
// hooks, no extra tools, and no extra sinks. Use WithPlugin to install
// behavior bundles such as compaction.New().
func New(opts ...Option) *Session {
	s := &Session{
		id:      store.NewID(store.PrefixSession),
		history: []models.Message{},
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// WithWorkDir sets the working directory tools will see.
func WithWorkDir(dir string) Option {
	return func(s *Session) { s.workDir = dir }
}

// WithModel sets the models.Model used for inference.
func WithModel(m models.Model) Option {
	return func(s *Session) { s.model = m }
}

// WithSystem sets the system prompt sent on every turn.
func WithSystem(prompt string) Option {
	return func(s *Session) { s.system = prompt }
}

// WithTools registers the tools the model may call.
func WithTools(tools ...tool.Tool) Option {
	return func(s *Session) { s.tools = append(s.tools, tools...) }
}

// WithBeforeStep installs a raw hook that runs before each loop step
// and may persistently mutate the message slice (compaction-shaped).
// Composed *after* any plugin-contributed BeforeStep hooks; receives
// the post-plugin slice. Prefer WithPlugin for reusable behavior;
// reserve this for one-off ad-hoc hooks.
func WithBeforeStep(h loop.BeforeStepHook) Option {
	return func(s *Session) { s.beforeStep = h }
}

// WithTransformContext installs a raw ephemeral per-turn hook that may
// rewrite the message slice sent to the provider without affecting
// session history. Composed *after* any plugin-contributed
// TransformContext hooks (sees the post-plugin slice). Useful for
// redaction or per-turn context injection.
func WithTransformContext(h loop.TransformContextHook) Option {
	return func(s *Session) { s.transformContext = h }
}

// WithPlugin installs one or more plugins. Plugins contribute hooks,
// tools, sinks, and Part-type decoders. Hook composition order is
// install order (the first plugin's hook sees the raw slice; later
// plugins see the previous plugin's output). Tool name collisions
// resolve last-wins; sinks fan out to all installed plugins.
//
// Nothing is installed by default; bare New() sessions run with an
// empty plugin set.
func WithPlugin(plugins ...plugin.Plugin) Option {
	return func(s *Session) { s.plugins = append(s.plugins, plugins...) }
}

// WithMessageSink installs a callback fired for every complete
// message added to history during a Run — including plugin-injected
// messages (e.g. compaction markers) when the plugin emits a
// MessageEvent through the loop sink. Use this to persist messages
// incrementally as they're produced rather than batching at end of
// turn. Calls are synchronous on the loop goroutine; the callback
// must not block.
func WithMessageSink(fn func(models.Message)) Option {
	return func(s *Session) { s.messageSink = fn }
}

// WithOutputSchema constrains the assistant's reply on every loop turn
// to a JSON document conforming to the supplied schema. Providers that
// do not support native structured output silently ignore this; consult
// Model.Info().Capabilities.StructuredOutput to detect support.
//
// When the session has tools configured, the schema is sent on every
// turn including tool-calling turns. Providers that disallow tools and
// structured output simultaneously will surface an error from the
// underlying model.
func WithOutputSchema(schema *models.OutputSchema) Option {
	return func(s *Session) { s.outputSchema = schema }
}

// SetOutputSchema swaps the active output schema. Pass nil to clear.
func (s *Session) SetOutputSchema(schema *models.OutputSchema) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputSchema = schema
}

// OutputSchema returns the currently configured output schema, or nil.
func (s *Session) OutputSchema() *models.OutputSchema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.outputSchema
}

// ID returns the session identifier.
func (s *Session) ID() string { return s.id }

// WorkDir returns the configured working directory.
func (s *Session) WorkDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workDir
}

// SetModel swaps the active model. Useful for handlers that build the
// model lazily after constructing the session.
func (s *Session) SetModel(m models.Model) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = m
}

// SetSystem replaces the system prompt.
func (s *Session) SetSystem(prompt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = prompt
}

// SetTools replaces the tool registry.
func (s *Session) SetTools(tools []tool.Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tools = tools
}

// History returns a snapshot copy of the running message history.
func (s *Session) History() []models.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]models.Message, len(s.history))
	copy(out, s.history)
	return out
}

// AddMessage appends a message to the history without invoking the
// model. Handlers use this to rehydrate a session from persistent
// storage before calling Run.
func (s *Session) AddMessage(msg models.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, msg)
}

// SetHistory replaces the entire history. The slice is copied; later
// mutations of msgs do not affect the session.
func (s *Session) SetHistory(msgs []models.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append([]models.Message(nil), msgs...)
}

// Clear empties the history.
func (s *Session) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []models.Message{}
}

// Result is the terminal value of a Run / RunStream invocation.
type Result struct {
	// Response is the concatenated text content of the final assistant
	// message. Empty if the loop terminated without producing a
	// tool-call-free turn.
	Response string

	// ToolCalls is the per-call summary of every tool invocation across
	// every turn of this Run, in source order (the order the assistant
	// emitted the tool calls in within each turn, with turns in
	// execution order).
	ToolCalls []ToolCallResult

	// Usage is the cumulative token usage reported by the provider.
	Usage models.Usage

	// Steps is the number of assistant turns the loop ran.
	Steps int

	// StopReason tells callers why the loop terminated. Mirrors
	// loop.StopReason exactly; re-exported here so callers don't import
	// the loop package just for the constants.
	StopReason loop.StopReason
}

// ToolCallResult is a serialization-friendly view of one tool call.
// Wire format: handlers JSON-encode this into HTTP responses, so the
// field names matter.
type ToolCallResult struct {
	ToolName string `json:"tool_name"`
	Input    any    `json:"input,omitempty"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`
}

// Sentinel errors. ErrNoModel is returned when Run is called before a
// model has been configured.
var (
	ErrNoModel = errors.New("session: no model configured")
)

// Run drives one user message through the loop synchronously.
//
// On return, the session's history contains the input user message plus
// every assistant and tool-result message the loop produced. The returned
// Result is always non-nil even when err is non-nil, so callers can
// persist partial state.
func (s *Session) Run(ctx context.Context, message string) (*Result, error) {
	return s.runWith(ctx, message, nil)
}

// runWith is the shared core for Run and RunStream. extraSink, if
// non-nil, is invoked for every loop event in addition to the session's
// internal sink. The session's own sink collects ToolCallResults and
// keeps the running history in sync.
func (s *Session) runWith(ctx context.Context, message string, extraSink loop.Sink) (*Result, error) {
	s.mu.Lock()
	if s.model == nil {
		s.mu.Unlock()
		return nil, ErrNoModel
	}
	// Snapshot inputs.
	model := s.model
	system := s.system
	tools := append([]tool.Tool(nil), s.tools...)
	workDir := s.workDir
	rawBeforeStep := s.beforeStep
	rawTransformContext := s.transformContext
	plugins := append([]plugin.Plugin(nil), s.plugins...)
	messageSink := s.messageSink
	outputSchema := s.outputSchema

	// If any allowed tool is directory-scoped, the session must have a
	// working directory. Fail early before mutating history.
	for _, t := range tools {
		if _, ok := t.(tool.DirectoryScopedTool); ok && workDir == "" {
			s.mu.Unlock()
			return nil, fmt.Errorf("session cannot start: tool %q requires a working directory, but session has none", t.Name())
		}
	}

	// Append the user message before starting the loop so it ends up in
	// history even if the loop fails immediately.
	s.history = append(s.history, models.Message{
		Role:    models.RoleUser,
		Content: models.Content{models.TextPart{Text: message}},
	})
	historySnap := append([]models.Message(nil), s.history...)
	s.mu.Unlock()

	// Build the plugin registry. Done per-Run so plugins close over
	// the *current* session state (model, etc.) and so plugin Install
	// errors fail the call rather than the constructor. Plugin Name()
	// must be unique within a session — duplicates almost always mean
	// a misconfiguration (e.g. two storage plugins fighting over
	// initial history) and should fail loudly.
	reg := plugin.NewRegistry()
	seen := make(map[string]bool, len(plugins))
	for _, pl := range plugins {
		name := pl.Name()
		if seen[name] {
			return nil, fmt.Errorf("plugin %q already installed in this session", name)
		}
		seen[name] = true
		if err := pl.Install(reg); err != nil {
			return nil, fmt.Errorf("plugin %q install: %w", name, err)
		}
	}
	// Inject the session's own in-memory history as the final
	// BeforeRun contribution. Plugin BeforeRun hooks (e.g. storage
	// rehydration) run first; the session then appends its
	// in-memory snapshot on top. This keeps the loop's "BeforeRun is
	// the single source of initial history" invariant intact while
	// preserving the existing AddMessage / SetHistory / Run-then-
	// Run-again semantics for SDK consumers who don't use a storage
	// plugin.
	reg.RegisterBeforeRun(func(_ context.Context, current []models.Message) ([]models.Message, error) {
		return append(current, historySnap...), nil
	})
	built := reg.Build()

	// Hook composition: plugin-contributed hooks run first; user-
	// supplied raw hooks run last and see the post-plugin slice.
	beforeStep := composeBeforeStep(built.Hooks.BeforeStep, rawBeforeStep)
	transformContext := composeTransformContext(built.Hooks.TransformContext, rawTransformContext)

	// Tool composition: session tools first, then plugin tools (later
	// wins on name collision via the loop's registry).
	tools = append(tools, built.Tools...)

	// Sink fan-out: forward MessageEvents to the messageSink and
	// every event to plugin and extra sinks. Tool results are
	// collected from res.Turns after the loop returns (source order,
	// no shared mutable state across goroutines).
	internal := loop.SinkFunc(func(e loop.Event) {
		if messageSink != nil {
			if me, ok := e.(loop.MessageEvent); ok {
				messageSink(me.Message)
			}
		}
		if built.Sink != nil {
			built.Sink.OnEvent(e)
		}
		if extraSink != nil {
			extraSink.OnEvent(e)
		}
	})

	cfg := loop.Config{
		Model:        model,
		System:       system,
		Tools:        tools,
		WorkDir:      workDir,
		Sink:         internal,
		OutputSchema: outputSchema,
		Hooks: loop.Hooks{
			BeforeRun:        built.Hooks.BeforeRun,
			BeforeStep:       beforeStep,
			TransformContext: transformContext,
			BeforeToolCall:   built.Hooks.BeforeToolCall,
			AfterToolCall:    built.Hooks.AfterToolCall,
		},
	}

	res, runErr := loop.Run(ctx, cfg)

	// Adopt the loop's terminal message slice wholesale. This handles
	// both the simple case (loop appended turns to historySnap) and
	// the plugin-mutation case (a BeforeStep hook rewrote the slice).
	// loop.Run guarantees res != nil, even on error.
	s.mu.Lock()
	if res != nil {
		s.history = append([]models.Message(nil), res.Messages...)
	}
	s.mu.Unlock()

	// Collect tool calls from res.Turns in source order. Each Turn's
	// Results is already in source order; turns themselves are in
	// execution order. This replaces the old sink-based collection,
	// which was a data race under parallel tool execution.
	var toolCalls []ToolCallResult
	if res != nil {
		for _, t := range res.Turns {
			for _, tr := range t.Results {
				toolCalls = append(toolCalls, ToolCallResult{
					ToolName: tr.Name,
					Input:    tr.Args,
					Output:   tr.Output,
					Error:    errStringIf(tr.IsError, tr.Output),
				})
			}
		}
	}

	out := &Result{
		ToolCalls: toolCalls,
	}
	if res != nil {
		out.Usage = res.Usage
		out.Steps = res.Steps
		out.StopReason = res.StopReason
		// Extract response text from the last assistant message, if any.
		if last := lastAssistant(res.Messages); last != nil {
			out.Response = textOf(*last)
		}
	}
	if runErr != nil {
		return out, fmt.Errorf("loop: %w", runErr)
	}
	return out, nil
}

// errStringIf returns msg when isError is true, "" otherwise. Centralizes
// the contract that ToolCallResult.Error mirrors the IsError flag.
func errStringIf(isError bool, msg string) string {
	if !isError {
		return ""
	}
	return msg
}

// lastAssistant returns a pointer to the last RoleAssistant message in
// msgs, or nil if there is none. Used to extract Result.Response.
func lastAssistant(msgs []models.Message) *models.Message {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == models.RoleAssistant {
			return &msgs[i]
		}
	}
	return nil
}

// textOf concatenates every TextPart in a message in source order.
// Reasoning parts and tool calls are excluded; callers that need the
// full content walk msg.Content directly.
func textOf(msg models.Message) string {
	var out string
	for _, p := range msg.Content {
		if t, ok := p.(models.TextPart); ok {
			out += t.Text
		}
	}
	return out
}

// composeBeforeStep returns the composition of plugin and user
// BeforeStep hooks. If only one (or neither) is non-nil, returns it
// directly to keep the call path obvious.
func composeBeforeStep(pluginHook, userHook loop.BeforeStepHook) loop.BeforeStepHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info loop.BeforeStepInfo) ([]models.Message, error) {
		out, err := pluginHook(ctx, info)
		if err != nil {
			return nil, err
		}
		// Re-issue with the rewritten slice so the user hook sees
		// the post-plugin view.
		next := info
		next.Messages = out
		return userHook(ctx, next)
	}
}

// composeTransformContext mirrors composeBeforeStep for the per-turn
// transform seam.
func composeTransformContext(pluginHook, userHook loop.TransformContextHook) loop.TransformContextHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info loop.TransformContextInfo) ([]models.Message, error) {
		out, err := pluginHook(ctx, info)
		if err != nil {
			return nil, err
		}
		next := info
		next.Messages = out
		return userHook(ctx, next)
	}
}
