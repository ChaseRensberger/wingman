// Package session is a thin stateful wrapper over agent/loop.
//
// A Session owns:
//   - an identifier (ULID)
//   - a working directory passed to tool executions
//   - a models.Client + model ref + system prompt + tool registry
//   - the running message history
//   - optional lifecycle hooks (TransformHistory / TransformContext)
//   - optional persistence via WithStore
//
// Session itself is concurrency-safe (mu-guarded). Run and RunStream
// drive a single inference loop turn batch and append both the user
// message and any new assistant/tool messages produced by the loop into
// the session's running history.
//
// Plugins (agent/plugin) are opt-in: nothing is installed by
// default. Pass WithPlugin(compaction.New()) to enable summarization;
// pass any other plugin to extend behavior at the TransformHistory,
// TransformContext, BeforeToolCall, AfterToolCall, Sink, Tool, or
// Part-registry seams. WithTransformHistory / WithTransformContext remain
// available for power users who want to install one-off hooks without
// the plugin bundle.
//
// Persistence is wired directly via WithStore. When a store is
// configured, the session hydrates prior history on the first Run and
// persists every new message (user, assistant, and tool results) as
// they are produced. Nil store means in-memory only.
package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/agent/loop"
	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

// Session is a single conversation. Construct with New.
type Session struct {
	id        string
	workDir   string
	client    models.Client
	model     models.ModelRef
	modelInfo models.ModelInfo
	system    string
	tools     []tool.Tool
	logger    *slog.Logger

	// Plugins installed via WithPlugin. Composed into Built at Run
	// time so the session sees the model that was set most recently
	// (model can change via SetModelRef between turns).
	plugins []plugin.Plugin

	// Raw hook overrides installed via WithTransformHistory / WithTransformContext.
	// These run *after* plugin-contributed hooks (last wins for transform
	// pipelines), so a user-supplied hook always has the final word.
	transformHistory  loop.TransformHistoryHook
	transformContext  loop.TransformContextHook
	transformToolDefs loop.TransformToolDefsHook
	transformParams   loop.TransformParamsHook
	afterRun          loop.AfterRunHook

	// messageSink, if non-nil, is invoked for every loop MessageEvent
	// (including plugin-injected messages such as compaction markers
	// emitted via info.Sink). Servers wire this to store.AppendMessage
	// for incremental persistence.
	messageSink func(models.Message)

	// outputSchema, if non-nil, constrains the assistant's reply on every
	// loop turn to a JSON document conforming to the schema. See
	// WithOutputSchema for details.
	outputSchema *models.OutputSchema

	// store, if non-nil, provides message-level persistence. Hydration
	// happens on the first Run when history is empty; upserts happen
	// for every message appended to history.
	store store.Store

	history []models.Message
	mu      sync.RWMutex
}

// Option configures a new Session.
type Option func(*Session)

// New returns a Session with a freshly minted KSUID (ses_ prefix) and
// the supplied options applied. A new Session has an empty history and
// no model; Run/RunStream will return ErrNoModel until WithClient and
// WithModelRef (or SetModelRef) are applied.
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

// WithClient sets the model client used for inference.
func WithClient(c models.Client) Option {
	return func(s *Session) { s.client = c }
}

// WithModelRef sets the model used for inference.
func WithModelRef(ref models.ModelRef, info models.ModelInfo) Option {
	return func(s *Session) {
		s.model = ref
		s.modelInfo = info
	}
}

// WithModel is a compatibility helper for tests and embedders that already
// have a concrete client-like model value. It does not change the loop's
// client/model-ref contract.
func WithModel(c models.Client) Option {
	return func(s *Session) {
		s.client = c
		if infoProvider, ok := c.(interface{ Info() models.ModelInfo }); ok {
			info := infoProvider.Info()
			s.model = models.ModelRef{Provider: info.Provider, ID: info.ID, API: info.API, BaseURL: info.BaseURL}
			s.modelInfo = info
		}
	}
}

// WithSystem sets the system prompt sent on every turn.
func WithSystem(prompt string) Option {
	return func(s *Session) { s.system = prompt }
}

// WithTools registers the tools the model may call.
func WithTools(tools ...tool.Tool) Option {
	return func(s *Session) { s.tools = append(s.tools, tools...) }
}

// WithLogger enables structured runtime logs for this session. The logger is
// expected to already carry request/session attributes supplied by the caller.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Session) { s.logger = logger }
}

// WithTransformHistory installs a raw hook that runs before each loop step
// and may persistently mutate the message slice (compaction-shaped).
// Composed *after* any plugin-contributed TransformHistory hooks; receives
// the post-plugin slice. Prefer WithPlugin for reusable behavior;
// reserve this for one-off ad-hoc hooks.
func WithTransformHistory(h loop.TransformHistoryHook) Option {
	return func(s *Session) { s.transformHistory = h }
}

// WithTransformContext installs a raw ephemeral per-turn hook that may
// rewrite the message slice sent to the provider without affecting
// session history. Composed *after* any plugin-contributed
// TransformContext hooks (sees the post-plugin slice). Useful for
// redaction or per-turn context injection.
func WithTransformContext(h loop.TransformContextHook) Option {
	return func(s *Session) { s.transformContext = h }
}

// WithTransformToolDefs installs a raw per-turn hook that may rewrite
// the tool definitions sent to the provider without affecting the
// session's running tool registry. Composed *after* any
// plugin-contributed TransformToolDefs hooks.
func WithTransformToolDefs(h loop.TransformToolDefsHook) Option {
	return func(s *Session) { s.transformToolDefs = h }
}

// WithTransformParams installs a raw per-turn hook that may rewrite
// the sampling parameters sent to the provider. Composed *after* any
// plugin-contributed TransformParams hooks.
func WithTransformParams(h loop.TransformParamsHook) Option {
	return func(s *Session) { s.transformParams = h }
}

// WithAfterRun installs a raw hook that fires exactly once at the end
// of Run, after plugin-contributed AfterRun hooks. Errors are joined.
func WithAfterRun(h loop.AfterRunHook) Option {
	return func(s *Session) { s.afterRun = h }
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
// MessageEvent through the loop sink. Use this to observe messages
// incrementally as they're produced rather than batching at end of
// turn. Calls are synchronous on the loop goroutine; the callback
// must not block.
func WithMessageSink(fn func(models.Message)) Option {
	return func(s *Session) { s.messageSink = fn }
}

// WithOutputSchema constrains the assistant's reply on every loop turn
// to a JSON document conforming to the supplied schema. Providers that
// do not support native structured output silently ignore this; consult
// ModelInfo.Capabilities.StructuredOutput to detect support.
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

// SetModelRef swaps the active model.
func (s *Session) SetModelRef(ref models.ModelRef, info models.ModelInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = ref
	s.modelInfo = info
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

	// StructuredOutput is populated when the run had an active OutputSchema
	// and the model returned a parseable, schema-valid final message.
	StructuredOutput map[string]any
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
	if s.client == nil || s.model.Provider == "" || s.model.ID == "" {
		s.mu.Unlock()
		return nil, ErrNoModel
	}
	// Snapshot inputs.
	client := s.client
	model := s.model
	modelInfo := s.modelInfo
	system := s.system
	tools := append([]tool.Tool(nil), s.tools...)
	workDir := s.workDir
	logger := s.logger
	rawTransformHistory := s.transformHistory
	rawTransformContext := s.transformContext
	rawTransformToolDefs := s.transformToolDefs
	rawTransformParams := s.transformParams
	rawAfterRun := s.afterRun
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

	// Hydrate prior history from the store on first run.
	if err := s.hydrate(ctx); err != nil {
		s.mu.Unlock()
		return nil, err
	}

	// Append the user message before starting the loop so it ends up in
	// history even if the loop fails immediately.
	s.history = append(s.history, models.Message{
		Role:    models.RoleUser,
		Content: models.Content{models.TextPart{Text: message}},
	})
	userMsgIdx := len(s.history) - 1
	if err := s.persistMessage(ctx, s.history[userMsgIdx], userMsgIdx); err != nil {
		s.mu.Unlock()
		return nil, err
	}
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
	// BeforeRun contribution. Plugin BeforeRun hooks run first;
	// the session then appends its in-memory snapshot on top. This
	// keeps the loop's "BeforeRun is the single source of initial
	// history" invariant intact while preserving the existing
	// AddMessage / SetHistory / Run-then-Run-again semantics for
	// SDK consumers.
	reg.RegisterBeforeRun(func(_ context.Context, current []models.Message) ([]models.Message, error) {
		return append(current, historySnap...), nil
	})
	built := reg.Build()

	// Hook composition: plugin-contributed hooks run first; user-
	// supplied raw hooks run last and see the post-plugin slice.
	transformHistory := composeTransformHistory(built.Hooks.TransformHistory, rawTransformHistory)
	transformContext := composeTransformContext(built.Hooks.TransformContext, rawTransformContext)
	transformToolDefs := composeTransformToolDefs(built.Hooks.TransformToolDefs, rawTransformToolDefs)
	transformParams := composeTransformParams(built.Hooks.TransformParams, rawTransformParams)
	afterRun := composeAfterRun(built.Hooks.AfterRun, rawAfterRun)

	// Tool composition: session tools first, then plugin tools (later
	// wins on name collision via the loop's registry).
	tools = append(tools, built.Tools...)

	// Sink fan-out: persist MessageEvents, then forward to the
	// messageSink, plugin sinks, and extraSink. Tool results are
	// collected from res.Turns after the loop returns.
	var persistErr error
	nextMsgIdx := len(historySnap)
	if logger != nil {
		logger = logger.With(
			"session_id", s.id,
			"provider", model.Provider,
			"model", model.ID,
			"tools", len(tools),
			"workdir_set", workDir != "",
		)
	}

	internal := loop.SinkFunc(func(e loop.Event) {
		logLoopEvent(logger, e)
		if me, ok := e.(loop.MessageEvent); ok {
			if s.store != nil {
				if err := s.persistMessage(ctx, me.Message, nextMsgIdx); err != nil && persistErr == nil {
					persistErr = err
				}
				nextMsgIdx++
			}
			if messageSink != nil {
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
		Client:       client,
		Model:        model,
		ModelInfo:    modelInfo,
		System:       system,
		Tools:        tools,
		WorkDir:      workDir,
		Sink:         internal,
		OutputSchema: outputSchema,
		Hooks: loop.Hooks{
			BeforeRun:         built.Hooks.BeforeRun,
			TransformHistory:  transformHistory,
			TransformContext:  transformContext,
			TransformToolDefs: transformToolDefs,
			TransformParams:   transformParams,
			BeforeToolCall:    built.Hooks.BeforeToolCall,
			AfterToolCall:     built.Hooks.AfterToolCall,
			AfterRun:          afterRun,
		},
	}

	start := time.Now()
	if logger != nil {
		logger.Info("session run started", "history_messages", len(historySnap))
	}
	res, runErr := loop.Run(ctx, cfg)

	// Adopt the loop's terminal message slice wholesale. This handles
	// both the simple case (loop appended turns to historySnap) and
	// the plugin-mutation case (a TransformHistory hook rewrote the slice).
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
		out.StructuredOutput = res.StructuredOutput
		// Extract response text from the last assistant message, if any.
		if last := lastAssistant(res.Messages); last != nil {
			out.Response = textOf(*last)
		}
	}
	if logger != nil {
		attrs := []any{
			"duration_ms", time.Since(start).Milliseconds(),
			"tool_calls", len(toolCalls),
			"input_tokens", out.Usage.InputTokens,
			"output_tokens", out.Usage.OutputTokens,
			"total_tokens", out.Usage.TotalTokens,
			"reasoning_tokens", out.Usage.ReasoningTokens,
			"cached_input_tokens", out.Usage.CachedInputTokens,
			"cache_write_tokens", out.Usage.CacheWriteTokens,
			"steps", out.Steps,
			"stop_reason", out.StopReason,
		}
		if runErr != nil {
			logger.Error("session run failed", append(attrs, "error", runErr)...)
		} else if persistErr != nil {
			logger.Error("session run persistence failed", append(attrs, "error", persistErr)...)
		} else {
			logger.Info("session run completed", attrs...)
		}
	}
	if runErr != nil {
		return out, fmt.Errorf("loop: %w", runErr)
	}
	if persistErr != nil {
		return out, fmt.Errorf("persist: %w", persistErr)
	}
	return out, nil
}

func logLoopEvent(logger *slog.Logger, e loop.Event) {
	if logger == nil {
		return
	}
	switch v := e.(type) {
	case loop.IterationStartEvent:
		logger.Debug("loop turn started", "step", v.Step)
	case loop.IterationEndEvent:
		logger.Info("loop turn completed",
			"step", v.Step,
			"tool_calls", len(v.Turn.Results),
			"input_tokens", v.Turn.Usage.InputTokens,
			"output_tokens", v.Turn.Usage.OutputTokens,
			"total_tokens", v.Turn.Usage.TotalTokens,
		)
	case loop.ToolExecutionStartEvent:
		logger.Info("tool execution started", "tool", v.Call.Name, "call_id", v.Call.ID)
	case loop.ToolExecutionEndEvent:
		logger.Info("tool execution completed",
			"tool", v.Result.Name,
			"call_id", v.Result.CallID,
			"duration_ms", v.Result.Duration.Milliseconds(),
			"is_error", v.Result.IsError,
		)
	case loop.ContextTransformedEvent:
		logger.Info("context transformed",
			"step", v.Step,
			"phase", v.Phase,
			"original_count", v.OriginalCount,
			"new_count", v.NewCount,
		)
	case loop.StructuredOutputEvent:
		logger.Info("structured output produced", "schema", v.Schema)
	case loop.ErrorEvent:
		logger.Error("loop error", "error", v.Err)
	}
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

// composeTransformHistory returns the composition of plugin and user
// TransformHistory hooks. If only one (or neither) is non-nil, returns it
// directly to keep the call path obvious.
func composeTransformHistory(pluginHook, userHook loop.TransformHistoryHook) loop.TransformHistoryHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info loop.TransformHistoryInfo) ([]models.Message, error) {
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

// composeTransformContext mirrors composeTransformHistory for the per-turn
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

// composeTransformToolDefs mirrors composeTransformHistory for the
// tool-definitions transform seam.
func composeTransformToolDefs(pluginHook, userHook loop.TransformToolDefsHook) loop.TransformToolDefsHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info loop.TransformToolDefsInfo) ([]models.ToolDef, error) {
		out, err := pluginHook(ctx, info)
		if err != nil {
			return nil, err
		}
		next := info
		next.Tools = out
		return userHook(ctx, next)
	}
}

// composeTransformParams mirrors composeTransformHistory for the
// sampling-parameters transform seam.
func composeTransformParams(pluginHook, userHook loop.TransformParamsHook) loop.TransformParamsHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info loop.TransformParamsInfo) (loop.TransformParamsResult, error) {
		out, err := pluginHook(ctx, info)
		if err != nil {
			return loop.TransformParamsResult{}, err
		}
		next := info
		next.Params = out.Params
		return userHook(ctx, next)
	}
}

// composeAfterRun runs the plugin hook first, then the user hook.
// Errors from both are joined.
func composeAfterRun(pluginHook, userHook loop.AfterRunHook) loop.AfterRunHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info loop.AfterRunInfo) error {
		var errs []error
		if err := pluginHook(ctx, info); err != nil {
			errs = append(errs, err)
		}
		if err := userHook(ctx, info); err != nil {
			errs = append(errs, err)
		}
		return errors.Join(errs...)
	}
}
