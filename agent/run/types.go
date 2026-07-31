// Package run is the agent inference run. It drives a models.Client
// across multiple turns, dispatches tool calls between turns, and emits
// lifecycle events to a sink.
//
// # What this package owns
//
// Everything between "we have a Model and a Tool list" and "the next
// assistant message has been streamed and any tool calls have been
// executed". This package does NOT own:
//
//   - Persistence policy. Callers may persist physical model-call attempts
//     through Config.ModelCallLifecycle and/or consume completed turns from
//     the event sink.
//   - HTTP/SSE transport. Caller drains the sink to whatever wire format
//     it needs.
//
// Compaction does not live in this package. Loop only offers a
// TransformHistory hook seam (see Hooks.TransformHistory) plus per-turn cumulative
// usage tracking; the agent/hook package ships a default compaction
// implementation that plugs into TransformHistory, and consumers can install
// their own hook for any other between-step transformation (budgeting,
// tool-result trimming, redaction, etc.).
//
// # Event vocabulary
//
// The loop emits two layers of events to the Sink:
//
//   - Raw models.StreamPart values from the active provider stream,
//     wrapped as a StreamPartEvent. Consumers that want to forward
//     provider streaming verbatim (e.g., the SSE handler) can do so.
//   - High-level lifecycle events: IterationStartEvent, IterationEndEvent,
//     ToolUseProposedEvent, ToolUseAuthorizedEvent, ToolExecutionStartEvent,
//     ToolExecutionEndEvent, MessageEvent. These
//     are convenient framing for UI consumers that don't want to track
//     stream parts themselves.
//
// Sinks that only care about one layer can ignore the other via a type
// switch.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
)

// Config carries everything the loop needs to run. All fields except
// Client, Model, and Messages have sensible zero-value defaults.
type Config struct {
	// Client streams provider requests. Required.
	Client models.Client

	// Model is the provider-qualified model ref sent on every request. Required.
	Model models.ModelRef

	// ModelInfo carries static metadata used for capability gates and hooks.
	ModelInfo models.ModelInfo

	// Messages is the conversation history the loop appends to. The loop
	// mutates this slice in place: assistant messages from each turn and
	// tool result messages from each batch are appended. Callers that
	// need the snapshot use Result.Messages instead.
	Messages []models.Message

	// System is the system prompt sent to the model. Empty string is
	// treated as "no system prompt".
	System string

	// Tools is the tool registry the model may call. nil means no tools
	// (the model gets an empty tool list and any tool calls are an
	// error). The loop builds the per-turn ToolDef list from this.
	Tools []tool.Tool

	// WorkDir is the working directory passed to every tool's Execute.
	// Tools that don't need it can ignore it. Empty string means the
	// process working directory at execution time.
	WorkDir string

	// Permissions are ordered tool permission rules. Later matching rules win.
	// Empty means preserve the historical behavior and allow every tool call.
	Permissions permission.Ruleset

	// ToolChoice controls how the model selects tools on every turn.
	// Zero value is treated as ToolChoiceAuto by all providers.
	// Typical use: force ToolChoiceRequired when structured output is needed,
	// or ToolChoiceNone to prevent tool use on a specific session.
	ToolChoice models.ToolChoice

	// Capabilities are request-level knobs forwarded to the model on every
	// turn. Providers silently ignore fields they don't support.
	// Example: set Capabilities.Thinking to enable extended reasoning on
	// Anthropic models.
	Capabilities models.Capabilities

	// OutputSchema, when set, is passed to the model on every iteration.
	// It only constrains text blocks (not tool-use blocks), so tool-calling
	// turns are unaffected; the constraint takes effect on the terminal
	// text-only turn.
	//
	// Providers that lack native structured-output support silently ignore
	// this field; consult ModelInfo.Capabilities.StructuredOutput for reliable
	// detection.
	OutputSchema *models.OutputSchema

	// MaxSteps caps the number of assistant turns. Zero means unlimited
	// (the loop terminates only when the model produces a turn with no
	// tool calls, or when a tool batch all returns terminate=true).
	//
	// Wingman's default is unlimited because the typical use case is
	// long-running coding agents. Callers that want safety nets set this
	// explicitly.
	MaxSteps int

	// ToolExecution overrides the default per-tool sequential/parallel
	// decision. Empty string defers to per-tool Sequential() opt-in.
	ToolExecution ToolExecutionMode

	// Hooks are user-supplied callbacks invoked at specific points in
	// the run. See Hooks for details. nil hooks are skipped.
	Hooks Hooks

	// Sink receives lifecycle events. nil discards events. The loop
	// guarantees Sink is called from a single goroutine (the loop's own)
	// so Sink implementations need not be concurrent-safe.
	Sink Sink

	// ModelCallLifecycle observes each physical provider request. Start runs
	// immediately before dispatch; Finish runs after the provider stream ends
	// and before any tool execution. nil disables model-call lifecycle hooks.
	ModelCallLifecycle ModelCallLifecycle

	// MessageCheckpoint synchronously persists the authoritative assistant
	// message snapshot as it is assembled. It may assign message and part IDs.
	// Unlike Sink, checkpoint failures stop the run.
	MessageCheckpoint MessageCheckpoint

	// ToolUseLifecycle durably records tool-use transitions. It provides a
	// pre-side-effect started fence and terminal accounting; it does not make
	// tool execution exactly-once.
	ToolUseLifecycle ToolUseLifecycle
}

// ToolUseStatus is a terminal durable tool-use status.
type ToolUseStatus string

const (
	ToolUseStatusCompleted   ToolUseStatus = "completed"
	ToolUseStatusFailed      ToolUseStatus = "failed"
	ToolUseStatusInterrupted ToolUseStatus = "interrupted"
	ToolUseStatusDeclined    ToolUseStatus = "declined"
)

// ToolUseLifecycle durably tracks a model-proposed tool use. Implementations
// may receive calls concurrently for different tool uses.
type ToolUseLifecycle interface {
	Propose(ctx context.Context, info ToolUseProposeInfo) (toolUseID string, err error)
	Authorize(ctx context.Context, info ToolUseAuthorizeInfo) error
	Start(ctx context.Context, info ToolUseStartInfo) error
	Finish(ctx context.Context, info ToolUseFinishInfo) error
}

// ToolUseProposeInfo identifies a proposed model tool call before execution.
type ToolUseProposeInfo struct {
	Step, Ordinal                  int
	CallID, Name                   string
	Args                           map[string]any
	MessageID, PartID, ModelCallID string
	ProposedAt                     time.Time
}

// ToolUseAuthorizeInfo records the authoritative, hook-rewritten arguments.
type ToolUseAuthorizeInfo struct {
	Step, Ordinal                  int
	ToolUseID, CallID, Name        string
	Args                           map[string]any
	MessageID, PartID, ModelCallID string
	AuthorizedAt                   time.Time
}

// ToolUseStartInfo records the durable boundary immediately before execution.
type ToolUseStartInfo struct {
	Step, Ordinal                  int
	ToolUseID, CallID, Name        string
	Args                           map[string]any
	MessageID, PartID, ModelCallID string
	StartedAt                      time.Time
}

// ToolUseFinishInfo records terminal accounting for a proposed tool use.
type ToolUseFinishInfo struct {
	Step, Ordinal                                    int
	ToolUseID, CallID, Name                          string
	Args                                             map[string]any
	MessageID, PartID, ModelCallID                   string
	ProposedAt, AuthorizedAt, StartedAt, CompletedAt time.Time
	Status                                           ToolUseStatus
	ToolResult                                       ToolResult
	ErrorType, ErrorMessage                          string
	Failure                                          error
}

// MessageCheckpoint persists an authoritative assistant message snapshot.
type MessageCheckpoint interface {
	Save(ctx context.Context, info MessageCheckpointInfo) (models.Message, error)
}

// MessageCheckpointInfo identifies the turn and complete assistant snapshot.
type MessageCheckpointInfo struct {
	Step    int
	Message models.Message
}

// ModelCallLifecycle persists or observes the lifecycle of a physical model
// request. A future provider retry loop invokes these methods once per attempt.
type ModelCallLifecycle interface {
	Start(ctx context.Context, info ModelCallStartInfo) (callID string, err error)
	Finish(ctx context.Context, info ModelCallFinishInfo) error
}

// ModelCallStartInfo identifies a physical model request before it is dispatched.
type ModelCallStartInfo struct {
	Step      int
	Attempt   int
	MessageID string
	StartedAt time.Time
	Trace     models.CallTrace
}

// ModelCallFinishInfo records the terminal state of a physical model request.
// Assistant is nil when the provider did not produce a message. Failure is the
// physical provider error, if any.
type ModelCallFinishInfo struct {
	Step              int
	Attempt           int
	CallID            string
	StartedAt         time.Time
	CompletedAt       time.Time
	Trace             models.CallTrace
	Assistant         *models.Message
	Usage             models.Usage
	ProviderRequestID string
	Failure           error
}

// ToolExecutionMode selects per-call vs per-batch tool dispatch.
type ToolExecutionMode string

const (
	// ToolExecutionDefault honors per-tool Sequential() opt-in. If any
	// tool in a batch is sequential, the whole batch runs sequentially.
	// Otherwise tools run in parallel.
	ToolExecutionDefault ToolExecutionMode = ""

	// ToolExecutionParallel forces every batch to run in parallel,
	// ignoring Sequential() opt-ins. Use with caution.
	ToolExecutionParallel ToolExecutionMode = "parallel"

	// ToolExecutionSequential forces every batch to run sequentially.
	ToolExecutionSequential ToolExecutionMode = "sequential"
)

// Hooks bundles the loop's extension points. Every field is optional.
//
// Calling convention notes:
//   - Each hook receives the loop's context and can return an error to
//     fail the run. Returning ErrSkipTool from BeforeToolCall skips the
//     execution and synthesizes a tool result with the returned args/
//     error message; this is the soft-deny path. Returning any other
//     error fails the run.
//   - TransformContext and TransformSystem return new values; the loop
//     uses the returned slice/string going forward but does not write
//     back into Config.Messages. This means transforms are per-turn.
//   - TransformHistory, in contrast, mutates the loop's running history: the
//     returned slice replaces r.messages and persists across subsequent
//     turns. Use TransformHistory for compaction / budget enforcement /
//     anything that should outlive a single turn; use TransformContext
//     for per-turn ephemeral edits (redaction, injection).
//   - Hooks run synchronously on the loop goroutine. Slow hooks slow the
//     run. Hooks that need concurrency should fire-and-forget into
//     their own goroutines.
//
// To add a new lifecycle seam:
//  1. Declare the Info struct + Hook function type below.
//  2. Add a field to Hooks here.
//  3. Add the call site in run.go at the appropriate point.
//  4. Define an event type (and isEvent method) if observers should see
//     it cross the Sink boundary.
//
// Candidate future seams: AfterRun (final cleanup).
type Hooks struct {
	// BeforeRun fires exactly once at the start of Run, after Config
	// validation and before the first iteration. It returns the
	// initial message history the loop will work with. The canonical
	// user is the storage plugin (rehydrate from disk); other use
	// cases include injecting a session-resumption marker or a header
	// system-of-record message.
	//
	// Mutually exclusive with Config.Messages: if BeforeRun is set,
	// Config.Messages must be empty. The loop returns a config error
	// otherwise. This forces a single source of truth for initial
	// history and prevents accidental double-loading.
	//
	// Multiple plugins compose via the plugin registry; each receives
	// the previous hook's accumulated history. Returning a nil slice
	// is a no-op (the chain continues with the accumulator unchanged).
	BeforeRun BeforeRunHook

	// OnTurnStart fires at the top of each turn, after MaxSteps is
	// checked and after TransformHistory / TransformContext / the LLM call.
	// step is 1-indexed.
	OnTurnStart func(ctx context.Context, step int) error

	// OnTurnEnd fires after a turn's assistant message and tool
	// results have been appended. The Turn carries everything that
	// happened in the turn. Errors here fail the run.
	OnTurnEnd func(ctx context.Context, step int, turn Turn) error

	// TransformHistory, if non-nil, is invoked at the top of each loop
	// iteration (after MaxSteps gating, before OnTurnStart). The
	// returned slice replaces the loop's running message history and
	// persists across subsequent turns. Use this for compaction, budget
	// enforcement, or any other transformation that should outlive a
	// single turn. Returning the input slice unchanged is a no-op.
	//
	// If the returned slice's length differs from the input, the loop
	// emits a ContextTransformedEvent so observers can react.
	//
	// Errors fail the run.
	TransformHistory TransformHistoryHook

	// TransformSystem may rewrite the system prompt for this turn. The
	// returned string replaces Config.System for the LLM call only;
	// subsequent turns see the original Config.System unless transformed
	// again. Useful for time-of-day injection, project context, etc.
	TransformSystem func(ctx context.Context, system string) (string, error)

	// TransformContext may rewrite the message history for this turn
	// only. The returned slice is sent to the model in place of the
	// loop's running history; the running history itself is unaffected,
	// so transforms apply only to the wire request for this turn. Use
	// cases: per-turn redaction, just-in-time injection, ephemeral
	// trimming.
	//
	// Contrast with TransformHistory, which persists its mutations.
	TransformContext TransformContextHook

	// BeforeToolCall fires for each tool call after the assistant turn,
	// before execution. It may return rewritten args to mutate the call.
	// Return ErrSkipTool to skip execution; the loop will synthesize a
	// tool result with the returned args (if non-nil) and the error's
	// message (if of type *ToolDecision; see ErrSkipTool docs). Any
	// other error fails the run.
	BeforeToolCall BeforeToolCallFunc

	// AfterToolCall fires after each tool call's execution (including
	// when execution failed). It may rewrite the result string. Returns
	// the (possibly rewritten) result; an error here fails the run.
	AfterToolCall AfterToolCallFunc

	// AfterRun fires exactly once at the end of Run, after the loop has
	// terminated for any reason (success, error, max steps, context
	// cancellation). It receives the final Result (possibly partial if
	// Err != nil) and the run error, if any. Errors returned from
	// AfterRun are joined with the existing run error using errors.Join.
	// If AfterRun returns nil, the existing return value is unchanged.
	AfterRun AfterRunHook

	// TransformToolDefs rewrites the tool definitions for this turn's
	// wire request only. Returning the input slice unchanged is a no-op.
	// Returning a nil slice means "send no tools this turn". Errors fail
	// the run. The loop's running tool registry is unaffected.
	TransformToolDefs TransformToolDefsHook

	// TransformParams mutates the per-request sampling parameters before
	// the LLM call. The hook receives the params the loop is about to
	// send and returns the params that should be used for this turn's
	// wire request only. The loop's own Config is unaffected. Errors fail
	// the run.
	TransformParams TransformParamsHook
}

// TransformHistoryInfo is the input to a TransformHistoryHook. Step is 1-indexed and
// reflects the upcoming iteration. Messages is the loop's current
// running history (the hook may inspect or copy but should treat it as
// read-only; return a new slice to mutate). Usage is the cumulative
// token usage across all completed turns. Model is the loop's model ref.
// Sink is the
// loop's event sink: hooks that synthesize new history messages
// (compaction markers, redaction notices, etc.) should emit a
// MessageEvent for each so observers (storage, UIs) see them on the
// same channel as loop-produced messages.
type TransformHistoryInfo struct {
	Step      int
	Messages  []models.Message
	Usage     models.Usage
	Client    models.Client
	Model     models.ModelRef
	ModelInfo models.ModelInfo
	Sink      Sink
}

// TransformHistoryHook is the signature for Hooks.TransformHistory. See its docs.
type TransformHistoryHook func(ctx context.Context, info TransformHistoryInfo) ([]models.Message, error)

// BeforeRunHook is the signature for Hooks.BeforeRun. It returns the
// initial message history for the run. Composed across plugins by the
// registry: each hook receives the accumulated history from prior hooks
// and returns the new accumulated history. Returning nil is a no-op
// (chain continues with the accumulator unchanged).
type BeforeRunHook func(ctx context.Context, current []models.Message) ([]models.Message, error)

// TransformContextInfo is the input to a TransformContextHook. Step is
// 1-indexed and reflects the current iteration. Messages is the slice
// being prepared for the model (post-TransformHistory). ModelInfo is supplied
// so hooks can introspect (e.g. context window) for budget decisions.
type TransformContextInfo struct {
	Step      int
	Messages  []models.Message
	Model     models.ModelRef
	ModelInfo models.ModelInfo
}

// TransformContextHook is the signature for Hooks.TransformContext. See
// its docs.
type TransformContextHook func(ctx context.Context, info TransformContextInfo) ([]models.Message, error)

// BeforeToolCallFunc is the signature for Hooks.BeforeToolCall. See
// the field docs for semantics. Named so plugin composition layers can
// reference the type without re-stating the signature.
type BeforeToolCallFunc func(ctx context.Context, call ToolCall) (newArgs map[string]any, err error)

// AfterToolCallFunc is the signature for Hooks.AfterToolCall. Hooks receive
// and return the structured result so output, errors, and metadata stay distinct.
type AfterToolCallFunc func(ctx context.Context, call ToolCall, result ToolResult) (ToolResult, error)

// AfterRunInfo is the input to an AfterRunHook.
type AfterRunInfo struct {
	Result Result // the final Result the loop will return
	Err    error  // non-nil if the loop terminated with an error
}

// AfterRunHook is the signature for Hooks.AfterRun.
type AfterRunHook func(ctx context.Context, info AfterRunInfo) error

// TransformToolDefsInfo is the input to a TransformToolDefsHook.
type TransformToolDefsInfo struct {
	Step      int              // 1-indexed iteration the request is being built for
	Tools     []models.ToolDef // current tool definitions about to be sent to the model
	Model     models.ModelRef  // the model the request is going to
	ModelInfo models.ModelInfo
}

// TransformToolDefsHook is the signature for Hooks.TransformToolDefs.
type TransformToolDefsHook func(ctx context.Context, info TransformToolDefsInfo) ([]models.ToolDef, error)

// SamplingParams is the set of per-request sampling knobs exposed by
// the run. Pointer fields distinguish "unset" from "set to zero".
type SamplingParams struct {
	MaxOutputTokens *int
}

// TransformParamsInfo is the input to a TransformParamsHook.
type TransformParamsInfo struct {
	Step      int
	Model     models.ModelRef
	ModelInfo models.ModelInfo
	Params    SamplingParams
}

// TransformParamsResult is the output of a TransformParamsHook.
type TransformParamsResult struct {
	Params SamplingParams
}

// TransformParamsHook is the signature for Hooks.TransformParams.
type TransformParamsHook func(ctx context.Context, info TransformParamsInfo) (TransformParamsResult, error)

// ErrSkipTool is returned from BeforeToolCall to skip tool execution
// without failing the run. The loop synthesizes a tool result message
// containing the error's Unwrap target as the result text and isError=true.
//
// Callers that want a custom skip message wrap it: fmt.Errorf("not
// permitted: bash on prod: %w", run.ErrSkipTool).
var ErrSkipTool = errors.New("skip tool")

// ToolCall is a tool invocation request from the model, surfaced to
// hooks and tool-execution events.
type ToolCall struct {
	// ID is the provider-assigned call ID. Use this to correlate hook
	// invocations with tool result messages.
	ID string `json:"call_id"`

	ToolUseID    string    `json:"tool_use_id,omitempty"`
	MessageID    string    `json:"message_id,omitempty"`
	PartID       string    `json:"part_id,omitempty"`
	ModelCallID  string    `json:"model_call_id,omitempty"`
	Step         int       `json:"step,omitempty"`
	Ordinal      int       `json:"ordinal,omitempty"`
	ProposedAt   time.Time `json:"proposed_at,omitempty"`
	AuthorizedAt time.Time `json:"authorized_at,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`

	// Name is the tool's registered name.
	Name string `json:"name"`

	// Args is the parsed tool arguments. The loop guarantees this is
	// never nil; absent args are an empty map.
	Args map[string]any `json:"args"`

	// Tool is the resolved Tool implementation, or nil if the model
	// called an unknown tool. BeforeToolCall fires even for unknown
	// tools so hooks can synthesize a custom error.
	Tool tool.Tool `json:"-"`
}

// ToolResult is the outcome of a single tool execution.
type ToolResult struct {
	CallID    string         `json:"call_id"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Status    ToolUseStatus  `json:"status,omitempty"`
	Name      string         `json:"name"`
	Args      map[string]any `json:"args"`
	Output    string         `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	IsError   bool           `json:"is_error"`
	// Duration is the wall-clock time spent in Tool.Execute (excluding
	// hook overhead). Zero for skipped or unknown-tool calls.
	Duration time.Duration `json:"duration,omitempty"`
}

// Turn is one iteration of the loop: an assistant message and the tool
// results produced by executing its tool calls.
type Turn struct {
	Step int
	// ModelCallID is the stable opaque ID returned by ModelCallLifecycle.Start.
	// It is empty when no lifecycle is configured.
	ModelCallID string
	// Attempt is the physical provider attempt within this step. It is currently
	// always 1 because the loop has no provider retry policy.
	Attempt int
	// ProviderRequestID is the provider request ID observed in response metadata.
	ProviderRequestID string
	Assistant         models.Message
	// Results is in source order (the order the assistant emitted the
	// tool calls in), regardless of execution mode. Empty if the
	// assistant produced no tool calls.
	Results     []ToolResult
	Usage       models.Usage
	StartedAt   time.Time
	CompletedAt time.Time
	Trace       models.CallTrace
	// Failure is set when an upstream model attempt fails after it starts.
	Failure error
}

// Result is the loop's terminal value, returned from Run.
type Result struct {
	// Messages is the full conversation after the run. This includes
	// the input messages the caller passed in. Callers that want only
	// the new messages take Messages[len(input):].
	Messages []models.Message

	// Turns is every completed turn in execution order. Each Turn
	// includes the assistant message and tool results in source order
	// (the order the assistant emitted the tool calls in). Empty if
	// the loop terminated before completing any turn.
	Turns []Turn

	// Usage is the cumulative token usage across every turn.
	Usage models.Usage

	// Steps is the number of turns the loop ran (1-indexed; 1 means one
	// assistant turn).
	Steps int

	// StopReason describes why the loop terminated. See StopReason
	// constants.
	StopReason StopReason

	// StructuredOutput is populated when the run had an active OutputSchema
	// and the model returned a parseable, schema-valid final message.
	StructuredOutput map[string]any
}

// StopReason explains why Run returned.
type StopReason string

const (
	// StopReasonEndTurn: the assistant produced a turn with no tool
	// calls. The model considers itself done.
	StopReasonEndTurn StopReason = "end_turn"

	// StopReasonMaxSteps: the loop hit Config.MaxSteps before the
	// assistant produced a tool-call-free turn.
	StopReasonMaxSteps StopReason = "max_steps"

	// StopReasonAborted: the loop's context was cancelled.
	StopReasonAborted StopReason = "aborted"

	// StopReasonError: an unrecoverable error happened (provider error,
	// hook error). Run also returns the error in this case.
	StopReasonError StopReason = "error"
)

// Sink receives loop lifecycle events. The loop serializes all
// emissions through a single internal goroutine before delivering them
// to OnEvent, so implementations need not be concurrent-safe even when
// tools execute in parallel.
//
// The loop does not consult Sink's return value (events are
// fire-and-forget); to halt the loop, hooks should return errors.
type Sink interface {
	OnEvent(Event)
}

// SinkFunc adapts a plain function into a Sink.
type SinkFunc func(Event)

// OnEvent implements Sink.
func (f SinkFunc) OnEvent(e Event) { f(e) }

// Event is the closed union of values that pass through Sink.OnEvent.
// The unexported isEvent method gates the union.
type Event interface {
	isEvent()
}

// IterationStartEvent fires at the top of a turn, after OnTurnStart
// hook but before the LLM call.
type IterationStartEvent struct {
	Step int `json:"step"`
}

// IterationEndEvent fires after a turn completes, before OnTurnEnd
// hook. Carries the same Turn the hook receives.
type IterationEndEvent struct {
	Step int  `json:"step"`
	Turn Turn `json:"turn"`
}

// MessageEvent fires when a complete message has been appended to the
// running history. This includes the assistant message at the end of
// each turn and any tool result messages produced by tool execution.
type MessageEvent struct {
	// Step identifies a provider-produced assistant message. Synthesized
	// messages and non-turn events leave it zero.
	Step    int            `json:"step,omitempty"`
	Message models.Message `json:"message"`
}

// ToolUseProposedEvent fires after a durable tool-use proposal commits.
type ToolUseProposedEvent struct {
	Call ToolCall `json:"call"`
}

// ToolUseAuthorizedEvent fires after authoritative arguments are durably authorized.
type ToolUseAuthorizedEvent struct {
	Call ToolCall `json:"call"`
}

// ToolExecutionStartEvent fires immediately before Tool.Execute, after the
// durable Start transition commits.
type ToolExecutionStartEvent struct {
	Call ToolCall `json:"call"`
}

// ToolExecutionProgressEvent fires when a tool reports incremental
// output or metadata during execution. Only tools that opt into
// streaming produce these events.
type ToolExecutionProgressEvent struct {
	CallID      string         `json:"call_id"`
	ToolUseID   string         `json:"tool_use_id,omitempty"`
	Name        string         `json:"name"`
	OutputDelta string         `json:"output_delta,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// ToolExecutionEndEvent fires after Tool.Execute returns or after the
// skip/error path produces a synthetic result. Events are delivered
// serially via the sink goroutine; for parallel batches the delivery
// order is the completion order, not source order. Consumers that need
// source order should read Turn.Results from IterationEndEvent or
// Result.Turns instead.
type ToolExecutionEndEvent struct {
	Result ToolResult `json:"result"`
}

// StreamPartEvent wraps a raw provider stream part. Consumers that want
// to forward provider streaming verbatim (e.g., over SSE) consume these.
// Consumers that only want lifecycle events ignore them.
type StreamPartEvent struct {
	Step      int               `json:"step"`
	MessageID string            `json:"message_id,omitempty"`
	PartID    string            `json:"part_id,omitempty"`
	Revision  int64             `json:"revision,omitempty"`
	Part      models.StreamPart `json:"part"`
}

func (e StreamPartEvent) MarshalJSON() ([]byte, error) {
	part, err := models.MarshalStreamPart(e.Part)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Step      int             `json:"step"`
		MessageID string          `json:"message_id,omitempty"`
		PartID    string          `json:"part_id,omitempty"`
		Revision  int64           `json:"revision,omitempty"`
		Part      json.RawMessage `json:"part"`
	}{
		Step: e.Step, MessageID: e.MessageID, PartID: e.PartID, Revision: e.Revision, Part: part,
	})
}

// ErrorEvent fires when the loop is about to terminate with an error.
// The error is also returned from Run; ErrorEvent exists so streaming
// consumers see it in-band before Run returns.
type ErrorEvent struct {
	Err error
}

// StructuredOutputEvent fires once per run, immediately after a
// successful parse + validation of the final assistant message. Sinks
// can use this to surface the typed result to clients.
type StructuredOutputEvent struct {
	Schema  string         // OutputSchema.Name, if set
	RawJSON string         // raw text of the final assistant message
	Parsed  map[string]any // parsed + schema-validated JSON
}

// ContextTransformedEvent fires when a hook (TransformHistory or
// TransformContext) replaced the message slice with one of a different
// length. Phase is "before_step" (mutation persisted into running
// history by TransformHistory) or "transform_context" (mutation ephemeral, applied only to
// this turn's request). Head is the first message of the post-hook
// slice when len > 0, nil otherwise; observers wanting to discriminate
// between hook kinds inspect Head's part type discriminators (e.g. a
// part with Type() == "compaction_marker" identifies a compaction-
// driven mutation). The loop never imports plugin types — it only
// surfaces the message and lets observers introspect.
type ContextTransformedEvent struct {
	Step          int
	Phase         string // "before_step" | "transform_context"
	OriginalCount int
	NewCount      int
	Head          *models.Message
}

func (IterationStartEvent) isEvent()        {}
func (IterationEndEvent) isEvent()          {}
func (MessageEvent) isEvent()               {}
func (ToolUseProposedEvent) isEvent()       {}
func (ToolUseAuthorizedEvent) isEvent()     {}
func (ToolExecutionStartEvent) isEvent()    {}
func (ToolExecutionProgressEvent) isEvent() {}
func (ToolExecutionEndEvent) isEvent()      {}
func (StreamPartEvent) isEvent()            {}
func (ErrorEvent) isEvent()                 {}
func (ContextTransformedEvent) isEvent()    {}
func (StructuredOutputEvent) isEvent()      {}
