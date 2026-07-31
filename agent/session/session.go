// Package session is a thin stateful wrapper over agent/run.
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

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/store"
	"github.com/chaserensberger/wingman/tool"
)

// Session is a single conversation. Construct with New.
type Session struct {
	id          string
	workDir     string
	client      models.Client
	model       models.ModelRef
	modelInfo   models.ModelInfo
	system      string
	tools       []tool.Tool
	permissions permission.Ruleset
	prompter    run.PermissionPrompter
	retry       run.RetryPolicy
	logger      *slog.Logger
	agentID     string
	runID       string

	// Plugins installed via WithPlugin. Composed into Built at Run
	// time so the session sees the model that was set most recently
	// (model can change via SetModelRef between turns).
	plugins          []plugin.Plugin
	generation       *plugin.Generation
	partDecoders     models.PartDecoders
	replacingPlugins bool
	closed           bool
	closeDone        chan struct{}
	closeErr         error

	// Raw hook overrides installed via WithTransformHistory / WithTransformContext.
	// These run *after* plugin-contributed hooks (last wins for transform
	// pipelines), so a user-supplied hook always has the final word.
	transformHistory  run.TransformHistoryHook
	transformContext  run.TransformContextHook
	transformToolDefs run.TransformToolDefsHook
	transformParams   run.TransformParamsHook
	afterRun          run.AfterRunHook

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
	runMu   sync.Mutex
}

// sessionMessagePersistence assigns one durable history index to every
// message emitted during a run. Checkpoints and the serialized sink share it.
type sessionMessagePersistence struct {
	mu               sync.Mutex
	session          *Session
	nextIdx          int
	assistantIndexes map[int]int
}

func (p *sessionMessagePersistence) Save(ctx context.Context, info run.MessageCheckpointInfo) (models.Message, error) {
	p.mu.Lock()
	if p.assistantIndexes == nil {
		p.assistantIndexes = make(map[int]int)
	}
	idx, ok := p.assistantIndexes[info.Step]
	if !ok {
		idx = p.nextIdx
		p.nextIdx++
		p.assistantIndexes[info.Step] = idx
	}
	p.mu.Unlock()
	return p.session.persistMessage(ctx, info.Message, idx)
}

func (p *sessionMessagePersistence) indexForEvent(event run.MessageEvent) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if event.Message.Role == models.RoleAssistant && event.Step > 0 {
		if idx, ok := p.assistantIndexes[event.Step]; ok {
			return idx
		}
	}
	idx := p.nextIdx
	p.nextIdx++
	return idx
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
		id:           store.NewID(store.PrefixSession),
		history:      []models.Message{},
		partDecoders: models.BuiltinPartDecoders(),
		closeDone:    make(chan struct{}),
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

// WithModel is a compatibility helper for embedders that already
// have a concrete client-like model value. It does not change the loop's
// client/model-ref contract.
func WithModel(c models.Client) Option {
	return func(s *Session) {
		s.client = c
		if infoProvider, ok := c.(interface{ Info() models.ModelInfo }); ok {
			info := infoProvider.Info()
			s.model = models.ModelRef{
				Provider:      info.Provider,
				ID:            info.ID,
				API:           info.API,
				BaseURL:       info.BaseURL,
				Env:           info.Env,
				ContextWindow: info.ContextWindow,
				MaxOutput:     info.MaxOutput,
				Capabilities:  info.Capabilities,
			}
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

// WithPermissions sets runtime tool permission rules for this session.
func WithPermissions(rules permission.Ruleset) Option {
	return func(s *Session) { s.permissions = append(permission.Ruleset(nil), rules...) }
}

// WithPermissionPrompter resolves authored ask permission rules during runs.
// A nil prompter explicitly declines those calls without executing them.
func WithPermissionPrompter(prompter run.PermissionPrompter) Option {
	return func(s *Session) { s.prompter = prompter }
}

// WithRetryPolicy configures provider dispatch retries for this session.
func WithRetryPolicy(policy run.RetryPolicy) Option {
	return func(s *Session) { s.retry = policy }
}

// WithLogger enables structured runtime logs for this session. The logger is
// expected to already carry request/session attributes supplied by the caller.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Session) { s.logger = logger }
}

// WithAgentID sets the effective agent identifier persisted with model calls.
func WithAgentID(id string) Option {
	return func(s *Session) { s.agentID = id }
}

// WithRunID associates persisted model attempts with a durable session run.
func WithRunID(id string) Option {
	return func(s *Session) { s.runID = id }
}

// WithTransformHistory installs a raw hook that runs before each loop step
// and may persistently mutate the message slice (compaction-shaped).
// Composed *after* any plugin-contributed TransformHistory hooks; receives
// the post-plugin slice. Prefer WithPlugin for reusable behavior;
// reserve this for one-off ad-hoc hooks.
func WithTransformHistory(h run.TransformHistoryHook) Option {
	return func(s *Session) { s.transformHistory = h }
}

// WithTransformContext installs a raw ephemeral per-turn hook that may
// rewrite the message slice sent to the provider without affecting
// session history. Composed *after* any plugin-contributed
// TransformContext hooks (sees the post-plugin slice). Useful for
// redaction or per-turn context injection.
func WithTransformContext(h run.TransformContextHook) Option {
	return func(s *Session) { s.transformContext = h }
}

// WithTransformToolDefs installs a raw per-turn hook that may rewrite
// the tool definitions sent to the provider without affecting the
// session's running tool registry. Composed *after* any
// plugin-contributed TransformToolDefs hooks.
func WithTransformToolDefs(h run.TransformToolDefsHook) Option {
	return func(s *Session) { s.transformToolDefs = h }
}

// WithTransformParams installs a raw per-turn hook that may rewrite
// the sampling parameters sent to the provider. Composed *after* any
// plugin-contributed TransformParams hooks.
func WithTransformParams(h run.TransformParamsHook) Option {
	return func(s *Session) { s.transformParams = h }
}

// WithAfterRun installs a raw hook that fires exactly once at the end
// of Run, after plugin-contributed AfterRun hooks. Errors are joined.
func WithAfterRun(h run.AfterRunHook) Option {
	return func(s *Session) { s.afterRun = h }
}

// WithPlugin declares one or more plugins for lazy activation before the first
// run. Plugins contribute hooks, tools, bounded sinks, and scoped Part decoders.
// Hook composition order is declaration order. Tool names must be unique.
//
// Nothing is installed by default; bare New() sessions run with an
// empty plugin set.
func WithPlugin(plugins ...plugin.Plugin) Option {
	return func(s *Session) { s.plugins = append(s.plugins, plugins...) }
}

// SetPlugins stages and atomically replaces the active plugin generation.
// Activation failure leaves the previous generation active. Cleanup for the
// replaced generation runs after in-flight work has finished.
func (s *Session) SetPlugins(ctx context.Context, plugins ...plugin.Plugin) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.replacingPlugins {
		s.mu.Unlock()
		return ErrPluginReplacementInProgress
	}
	s.replacingPlugins = true
	s.mu.Unlock()

	var next *plugin.Generation
	var err error
	if len(plugins) > 0 {
		next, err = plugin.ActivateAll(plugins...)
	}
	if err != nil {
		s.mu.Lock()
		s.replacingPlugins = false
		s.mu.Unlock()
		return err
	}

	s.runMu.Lock()
	s.mu.Lock()
	if s.closed {
		s.replacingPlugins = false
		s.mu.Unlock()
		s.runMu.Unlock()
		_ = next.Close(ctx)
		return ErrClosed
	}
	previous := s.generation
	s.plugins = append([]plugin.Plugin(nil), plugins...)
	s.generation = next
	s.partDecoders = models.BuiltinPartDecoders()
	if next != nil {
		s.partDecoders = next.Parts()
	}
	s.replacingPlugins = false
	s.mu.Unlock()
	err = previous.Close(ctx)
	s.runMu.Unlock()
	return err
}

// Close waits for the active run and releases the current plugin generation.
// It is idempotent.
func (s *Session) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		done := s.closeDone
		s.mu.Unlock()
		select {
		case <-done:
			s.mu.RLock()
			err := s.closeErr
			s.mu.RUnlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.replacingPlugins {
		s.mu.Unlock()
		return ErrPluginReplacementInProgress
	}
	s.closed = true
	s.mu.Unlock()

	s.runMu.Lock()
	s.mu.Lock()
	generation := s.generation
	s.generation = nil
	s.mu.Unlock()
	err := generation.Close(ctx)
	s.mu.Lock()
	s.closeErr = err
	close(s.closeDone)
	s.mu.Unlock()
	s.runMu.Unlock()
	return err
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
	// run.StopReason exactly; re-exported here so callers don't import
	// the loop package just for the constants.
	StopReason run.StopReason

	// StructuredOutput is populated when the run had an active OutputSchema
	// and the model returned a parseable, schema-valid final message.
	StructuredOutput map[string]any
}

// ToolCallResult is a serialization-friendly view of one tool call.
// Wire format: handlers JSON-encode this into HTTP responses, so the
// field names matter.
type ToolCallResult struct {
	ToolName   string         `json:"tool_name"`
	Input      any            `json:"input,omitempty"`
	Output     string         `json:"output,omitempty"`
	Structured any            `json:"structured,omitempty"`
	Error      string         `json:"error,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// Sentinel errors. ErrNoModel is returned when Run is called before a
// model has been configured.
var (
	ErrNoModel                     = errors.New("session: no model configured")
	ErrClosed                      = errors.New("session: closed")
	ErrPluginReplacementInProgress = errors.New("session: plugin replacement already in progress")
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
func (s *Session) runWith(ctx context.Context, message string, extraSink run.Sink) (*Result, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	if s.client == nil || s.model.Provider == "" || s.model.ID == "" {
		s.mu.Unlock()
		return nil, ErrNoModel
	}
	// Snapshot inputs.
	client := s.client
	model := s.model
	modelInfo := s.modelInfo
	system := s.system
	currentDate := "Current date: " + time.Now().Format(time.DateOnly) + "."
	if system == "" {
		system = currentDate
	} else {
		system += "\n\n" + currentDate
	}
	tools := append([]tool.Tool(nil), s.tools...)
	permissions := append(permission.Ruleset(nil), s.permissions...)
	prompter := s.prompter
	retry := s.retry
	workDir := s.workDir
	logger := s.logger
	rawTransformHistory := s.transformHistory
	rawTransformContext := s.transformContext
	rawTransformToolDefs := s.transformToolDefs
	rawTransformParams := s.transformParams
	rawAfterRun := s.afterRun
	if s.generation == nil && len(s.plugins) > 0 {
		if s.replacingPlugins {
			s.mu.Unlock()
			return nil, ErrPluginReplacementInProgress
		}
		s.replacingPlugins = true
		plugins := append([]plugin.Plugin(nil), s.plugins...)
		s.mu.Unlock()
		generation, err := plugin.ActivateAll(plugins...)
		s.mu.Lock()
		s.replacingPlugins = false
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		if s.closed {
			s.mu.Unlock()
			_ = generation.Close(ctx)
			return nil, ErrClosed
		}
		s.generation = generation
		s.partDecoders = generation.Parts()
	}
	built := plugin.Built{}
	if s.generation != nil {
		built = s.generation.Runtime()
	}
	tools = append(tools, built.Tools...)
	if _, err := tool.Compose(tools); err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("session tool catalog: %w", err)
	}
	for _, t := range tools {
		if tool.IsDirectoryScoped(t) && workDir == "" {
			s.mu.Unlock()
			return nil, fmt.Errorf("session cannot start: tool %q requires a working directory, but session has none", t.Name())
		}
	}
	messageSink := s.messageSink
	outputSchema := s.outputSchema
	agentID := s.agentID
	runID := s.runID

	// Hydrate prior history from the store on first run.
	if err := s.hydrate(ctx); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	finalizeUnsettledTools(s.history)

	// Append the user message before starting the loop so it ends up in
	// history even if the loop fails immediately.
	s.history = append(s.history, models.Message{
		Role:    models.RoleUser,
		Content: models.Content{models.TextPart{Text: message}},
	})
	userMsgIdx := len(s.history) - 1
	userMsg, err := s.persistMessage(ctx, s.history[userMsgIdx], userMsgIdx)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.history[userMsgIdx] = userMsg
	historySnap := append([]models.Message(nil), s.history...)
	s.mu.Unlock()

	// Inject the session's own in-memory history as the final
	// BeforeRun contribution. Plugin BeforeRun hooks run first;
	// the session then appends its in-memory snapshot on top. This
	// keeps the loop's "BeforeRun is the single source of initial
	// history" invariant intact while preserving the existing
	// AddMessage / SetHistory / Run-then-Run-again semantics for
	// SDK consumers.
	pluginBeforeRun := built.Hooks.BeforeRun
	built.Hooks.BeforeRun = func(ctx context.Context, current []models.Message) ([]models.Message, error) {
		if pluginBeforeRun != nil {
			var err error
			current, err = pluginBeforeRun(ctx, current)
			if err != nil {
				return nil, err
			}
		}
		return append(current, historySnap...), nil
	}

	// Hook composition: plugin-contributed hooks run first; user-
	// supplied raw hooks run last and see the post-plugin slice.
	transformHistory := composeTransformHistory(built.Hooks.TransformHistory, rawTransformHistory)
	transformContext := composeTransformContext(built.Hooks.TransformContext, rawTransformContext)
	transformToolDefs := composeTransformToolDefs(built.Hooks.TransformToolDefs, rawTransformToolDefs)
	transformParams := composeTransformParams(built.Hooks.TransformParams, rawTransformParams)
	afterRun := composeAfterRun(built.Hooks.AfterRun, rawAfterRun)

	// Sink fan-out: persist MessageEvents, then forward to the
	// messageSink, plugin sinks, and extraSink. Tool results are
	// collected from res.Turns after the loop returns.
	var persistErr error
	messagePersistence := &sessionMessagePersistence{session: s, nextIdx: len(historySnap)}
	if logger != nil {
		logger = logger.With(
			"session_id", s.id,
			"provider", model.Provider,
			"model", model.ID,
			"tools", len(tools),
			"workdir_set", workDir != "",
		)
	}

	internal := run.SinkFunc(func(e run.Event) {
		logLoopEvent(logger, e)
		if me, ok := e.(run.MessageEvent); ok {
			if s.store != nil {
				idx := messagePersistence.indexForEvent(me)
				message, err := s.persistMessage(ctx, me.Message, idx)
				if err != nil && persistErr == nil {
					persistErr = err
				}
				if err == nil {
					me.Message = message
					e = me
				}
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

	cfg := run.Config{
		SessionID:          s.id,
		RunID:              runID,
		AgentID:            agentID,
		Client:             client,
		Model:              model,
		ModelInfo:          modelInfo,
		Capabilities:       models.Capabilities{Thinking: modelInfo.Capabilities.Reasoning},
		System:             system,
		Tools:              tools,
		WorkDir:            workDir,
		Permissions:        permissions,
		PermissionPrompter: prompter,
		Retry:              retry,
		Sink:               internal,
		OutputSchema:       outputSchema,
		Hooks: run.Hooks{
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
	if s.store != nil {
		cfg.MessageCheckpoint = messagePersistence
		cfg.ModelCallLifecycle = &modelCallRecorder{
			store:     s.store,
			sessionID: s.id,
			runID:     runID,
			agentID:   agentID,
			model:     model,
			modelInfo: modelInfo,
		}
		cfg.ToolUseLifecycle = &toolUseRecorder{
			store:     s.store,
			sessionID: s.id,
			runID:     runID,
		}
	}

	start := time.Now()
	if logger != nil {
		logger.Info("session run started", "history_messages", len(historySnap))
	}
	res, runErr := run.Run(ctx, cfg)

	// Adopt the loop's terminal message slice wholesale. This handles
	// both the simple case (loop appended turns to historySnap) and
	// the plugin-mutation case (a TransformHistory hook rewrote the slice).
	// run.Run guarantees res != nil, even on error.
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
					ToolName:   tr.Name,
					Input:      tr.Args,
					Output:     tr.Output,
					Structured: tr.Structured,
					Error:      errStringIf(tr.IsError, tr.Error),
					Metadata:   tr.Metadata,
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
		if s.store != nil {
			for _, turn := range res.Turns {
				stopReason := ""
				if turn.Step == res.Steps {
					stopReason = string(res.StopReason)
				}
				var structuredOutput map[string]any
				if turn.Step == res.Steps {
					structuredOutput = res.StructuredOutput
				}
				if err := s.persistModelCall(context.WithoutCancel(ctx), turn.Assistant.ID, turn, model, modelInfo, runID, agentID, stopReason, structuredOutput); err != nil && persistErr == nil {
					persistErr = err
				}
			}
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

func finalizeUnsettledTools(messages []models.Message) {
	completedAt := time.Now().UTC().UnixMilli()
	for i := range messages {
		if messages[i].Role != models.RoleAssistant {
			continue
		}
		for j, part := range messages[i].Content {
			tool, ok := part.(models.ToolPart)
			if !ok || (tool.State != models.ToolStatePending && tool.State != models.ToolStateRunning) {
				continue
			}
			tool.State = models.ToolStateError
			tool.Error = "Tool execution interrupted"
			tool.Output = ""
			tool.CompletedAt = completedAt
			messages[i].Content[j] = tool
		}
	}
}

func logLoopEvent(logger *slog.Logger, e run.Event) {
	if logger == nil {
		return
	}
	switch v := e.(type) {
	case run.IterationStartEvent:
		logger.Debug("loop turn started", "step", v.Step)
	case run.IterationEndEvent:
		logger.Info("loop turn completed",
			"step", v.Step,
			"tool_calls", len(v.Turn.Results),
			"input_tokens", v.Turn.Usage.InputTokens,
			"output_tokens", v.Turn.Usage.OutputTokens,
			"total_tokens", v.Turn.Usage.TotalTokens,
		)
	case run.ToolExecutionStartEvent:
		logger.Info("tool execution started", "tool", v.Call.Name, "call_id", v.Call.ID)
	case run.ToolExecutionEndEvent:
		logger.Info("tool execution completed",
			"tool", v.Result.Name,
			"call_id", v.Result.CallID,
			"duration_ms", v.Result.Duration.Milliseconds(),
			"is_error", v.Result.IsError,
		)
	case run.ContextTransformedEvent:
		logger.Info("context transformed",
			"step", v.Step,
			"phase", v.Phase,
			"original_count", v.OriginalCount,
			"new_count", v.NewCount,
		)
	case run.StructuredOutputEvent:
		logger.Info("structured output produced", "schema", v.Schema)
	case run.ErrorEvent:
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
func composeTransformHistory(pluginHook, userHook run.TransformHistoryHook) run.TransformHistoryHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info run.TransformHistoryInfo) ([]models.Message, error) {
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
func composeTransformContext(pluginHook, userHook run.TransformContextHook) run.TransformContextHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info run.TransformContextInfo) ([]models.Message, error) {
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
func composeTransformToolDefs(pluginHook, userHook run.TransformToolDefsHook) run.TransformToolDefsHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info run.TransformToolDefsInfo) ([]models.ToolDef, error) {
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
func composeTransformParams(pluginHook, userHook run.TransformParamsHook) run.TransformParamsHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info run.TransformParamsInfo) (run.TransformParamsResult, error) {
		out, err := pluginHook(ctx, info)
		if err != nil {
			return run.TransformParamsResult{}, err
		}
		next := info
		next.Params = out.Params
		return userHook(ctx, next)
	}
}

// composeAfterRun runs the plugin hook first, then the user hook.
// Errors from both are joined.
func composeAfterRun(pluginHook, userHook run.AfterRunHook) run.AfterRunHook {
	switch {
	case pluginHook == nil && userHook == nil:
		return nil
	case pluginHook == nil:
		return userHook
	case userHook == nil:
		return pluginHook
	}
	return func(ctx context.Context, info run.AfterRunInfo) error {
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
