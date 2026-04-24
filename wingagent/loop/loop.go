// Package loop is the wingagent inference loop. It drives a wingmodels.Model
// across multiple turns, dispatches tool calls between turns, and emits
// lifecycle events to a sink.
//
// # Design influences
//
//   - pi-mono's agent-loop.ts (bb/pi-mono/packages/agent/src/agent-loop.ts)
//     contributed the per-turn shape: drain pending messages, stream one
//     assistant response, execute tool calls, loop. The "sequential trumps
//     parallel per batch" rule and tool-batch terminate semantics are
//     ports of pi's behavior.
//   - opencode's session/prompt.ts (bb/opencode/packages/opencode/src/
//     session/prompt.ts) contributed the hook slicing: tool args in/out
//     are separate from message-history transform, which is separate from
//     system-prompt transform, which is separate from sampling parameters.
//     Six well-defined seams instead of one big "TransformContext".
//   - We diverge from both on calling convention. opencode uses
//     mutate-the-output-object hooks; pi-mono returns optional partials.
//     We use Go-idiomatic (NewValue, error) returns where a sentinel
//     error (ErrSkipTool, ErrDenyTool) controls flow. No magic, no
//     reflection.
//
// # What this package owns
//
// Everything between "we have a Model and a Tool list" and "the next
// assistant message has been streamed and any tool calls have been
// executed". This package does NOT own:
//
//   - Persistence. The caller (typically wingagent/session) hooks into
//     the event sink to write to storage as turns complete.
//   - HTTP/SSE transport. Caller drains the sink to whatever wire format
//     it needs.
//
// Compaction does not live in this package. Loop only offers a
// BeforeStep hook seam (see Hooks.BeforeStep) plus per-turn cumulative
// usage tracking; the wingagent/hook package ships a default compaction
// implementation that plugs into BeforeStep, and consumers can install
// their own hook for any other between-step transformation (budgeting,
// tool-result trimming, redaction, etc.).
//
// # Event vocabulary
//
// The loop emits two layers of events to the Sink:
//
//   - Raw wingmodels.StreamPart values from the active provider stream,
//     wrapped as a StreamPartEvent. Consumers that want to forward
//     provider streaming verbatim (e.g., the SSE handler) can do so.
//   - High-level lifecycle events: IterationStartEvent, IterationEndEvent,
//     ToolExecutionStartEvent, ToolExecutionEndEvent, MessageEvent. These
//     are convenient framing for UI consumers that don't want to track
//     stream parts themselves.
//
// Sinks that only care about one layer can ignore the other via a type
// switch.
package loop

import (
	"context"
	"errors"
	"time"

	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels"
)

// Config carries everything the loop needs to run. All fields except
// Model and Messages have sensible zero-value defaults.
type Config struct {
	// Model is the wingmodels.Model the loop streams against. Required.
	Model wingmodels.Model

	// Messages is the conversation history the loop appends to. The loop
	// mutates this slice in place: assistant messages from each turn and
	// tool result messages from each batch are appended. Callers that
	// need the snapshot use Result.Messages instead.
	Messages []wingmodels.Message

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
	// the loop. See Hooks for details. nil hooks are skipped.
	Hooks Hooks

	// Sink receives lifecycle events. nil discards events. The loop
	// guarantees Sink is called from a single goroutine (the loop's own)
	// so Sink implementations need not be concurrent-safe.
	Sink Sink
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
//     fail the loop. Returning ErrSkipTool from BeforeToolCall skips the
//     execution and synthesizes a tool result with the returned args/
//     error message; this is the soft-deny path. Returning any other
//     error fails the loop.
//   - TransformContext and TransformSystem return new values; the loop
//     uses the returned slice/string going forward but does not write
//     back into Config.Messages. This means transforms are per-turn.
//   - BeforeStep, in contrast, mutates the loop's running history: the
//     returned slice replaces r.messages and persists across subsequent
//     turns. Use BeforeStep for compaction / budget enforcement /
//     anything that should outlive a single turn; use TransformContext
//     for per-turn ephemeral edits (redaction, injection).
//   - Hooks run synchronously on the loop goroutine. Slow hooks slow the
//     loop. Hooks that need concurrency should fire-and-forget into
//     their own goroutines.
//
// To add a new lifecycle seam:
//  1. Declare the Info struct + Hook function type below.
//  2. Add a field to Hooks here.
//  3. Add the call site in run.go at the appropriate point.
//  4. Define an event type (and isEvent method) if observers should see
//     it cross the Sink boundary.
//
// Candidate future seams: BeforeRun (one-shot prelude), AfterStep
// (per-iteration telemetry), AfterRun (final cleanup).
type Hooks struct {
	// BeforeIteration fires at the top of each turn, after MaxSteps is
	// checked but before BeforeStep / TransformContext / the LLM call.
	// step is 1-indexed.
	BeforeIteration func(ctx context.Context, step int) error

	// AfterIteration fires after a turn's assistant message and tool
	// results have been appended. The Turn carries everything that
	// happened in the turn. Errors here fail the loop.
	AfterIteration func(ctx context.Context, step int, turn Turn) error

	// BeforeStep, if non-nil, is invoked at the top of each loop
	// iteration (after MaxSteps gating, before BeforeIteration). The
	// returned slice replaces the loop's running message history and
	// persists across subsequent turns. Use this for compaction, budget
	// enforcement, or any other transformation that should outlive a
	// single turn. Returning the input slice unchanged is a no-op.
	//
	// If the returned slice's length differs from the input, the loop
	// emits a ContextTransformedEvent so observers can react.
	//
	// Errors fail the loop.
	BeforeStep BeforeStepHook

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
	// Contrast with BeforeStep, which persists its mutations.
	TransformContext TransformContextHook

	// BeforeToolCall fires for each tool call after the assistant turn,
	// before execution. It may return rewritten args to mutate the call.
	// Return ErrSkipTool to skip execution; the loop will synthesize a
	// tool result with the returned args (if non-nil) and the error's
	// message (if of type *ToolDecision; see ErrSkipTool docs). Any
	// other error fails the loop.
	BeforeToolCall func(ctx context.Context, call ToolCall) (newArgs map[string]any, err error)

	// AfterToolCall fires after each tool call's execution (including
	// when execution failed). It may rewrite the result string. Returns
	// the (possibly rewritten) result; an error here fails the loop.
	AfterToolCall func(ctx context.Context, call ToolCall, result string, isError bool) (newResult string, err error)
}

// BeforeStepInfo is the input to a BeforeStepHook. Step is 1-indexed and
// reflects the upcoming iteration. Messages is the loop's current
// running history (the hook may inspect or copy but should treat it as
// read-only; return a new slice to mutate). Usage is the cumulative
// token usage across all completed turns. Model is the loop's model;
// hooks may use it for sub-calls (e.g. summarization).
type BeforeStepInfo struct {
	Step     int
	Messages []wingmodels.Message
	Usage    wingmodels.Usage
	Model    wingmodels.Model
}

// BeforeStepHook is the signature for Hooks.BeforeStep. See its docs.
type BeforeStepHook func(ctx context.Context, info BeforeStepInfo) ([]wingmodels.Message, error)

// TransformContextInfo is the input to a TransformContextHook. Step is
// 1-indexed and reflects the current iteration. Messages is the slice
// being prepared for the model (post-BeforeStep). Model is supplied so
// hooks can introspect (e.g. context window) for budget decisions.
type TransformContextInfo struct {
	Step     int
	Messages []wingmodels.Message
	Model    wingmodels.Model
}

// TransformContextHook is the signature for Hooks.TransformContext. See
// its docs.
type TransformContextHook func(ctx context.Context, info TransformContextInfo) ([]wingmodels.Message, error)

// ErrSkipTool is returned from BeforeToolCall to skip tool execution
// without failing the loop. The loop synthesizes a tool result message
// containing the error's Unwrap target as the result text and isError=true.
//
// Callers that want a custom skip message wrap it: fmt.Errorf("not
// permitted: bash on prod: %w", loop.ErrSkipTool).
var ErrSkipTool = errors.New("skip tool")

// ToolCall is a tool invocation request from the model, surfaced to
// hooks and tool-execution events.
type ToolCall struct {
	// ID is the provider-assigned call ID. Use this to correlate hook
	// invocations with tool result messages.
	ID string

	// Name is the tool's registered name.
	Name string

	// Args is the parsed tool arguments. The loop guarantees this is
	// never nil; absent args are an empty map.
	Args map[string]any

	// Tool is the resolved Tool implementation, or nil if the model
	// called an unknown tool. BeforeToolCall fires even for unknown
	// tools so hooks can synthesize a custom error.
	Tool tool.Tool
}

// ToolResult is the outcome of a single tool execution.
type ToolResult struct {
	CallID  string
	Name    string
	Args    map[string]any
	Output  string
	IsError bool
	// Duration is the wall-clock time spent in Tool.Execute (excluding
	// hook overhead). Zero for skipped or unknown-tool calls.
	Duration time.Duration
}

// Turn is one iteration of the loop: an assistant message and the tool
// results produced by executing its tool calls.
type Turn struct {
	Step      int
	Assistant wingmodels.Message
	// Results is in source order (the order the assistant emitted the
	// tool calls in), regardless of execution mode. Empty if the
	// assistant produced no tool calls.
	Results []ToolResult
	Usage   wingmodels.Usage
}

// Result is the loop's terminal value, returned from Run.
type Result struct {
	// Messages is the full conversation after the loop. This includes
	// the input messages the caller passed in. Callers that want only
	// the new messages take Messages[len(input):].
	Messages []wingmodels.Message

	// Usage is the cumulative token usage across every turn.
	Usage wingmodels.Usage

	// Steps is the number of turns the loop ran (1-indexed; 1 means one
	// assistant turn).
	Steps int

	// StopReason describes why the loop terminated. See StopReason
	// constants.
	StopReason StopReason
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

// Sink receives loop lifecycle events. The loop emits events from a
// single goroutine, so implementations need not be concurrent-safe.
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

// IterationStartEvent fires at the top of a turn, after BeforeIteration
// hook but before the LLM call.
type IterationStartEvent struct {
	Step int
}

// IterationEndEvent fires after a turn completes, before AfterIteration
// hook. Carries the same Turn the hook receives.
type IterationEndEvent struct {
	Step int
	Turn Turn
}

// MessageEvent fires when a complete message has been appended to the
// running history. This includes the assistant message at the end of
// each turn and any tool result messages produced by tool execution.
type MessageEvent struct {
	Message wingmodels.Message
}

// ToolExecutionStartEvent fires immediately before Tool.Execute is
// invoked (or, for unknown/skipped tools, where it would have been).
type ToolExecutionStartEvent struct {
	Call ToolCall
}

// ToolExecutionEndEvent fires after Tool.Execute returns or after the
// skip/error path produces a synthetic result. Order is completion order
// when running in parallel (so it is NOT necessarily source order).
type ToolExecutionEndEvent struct {
	Result ToolResult
}

// StreamPartEvent wraps a raw provider stream part. Consumers that want
// to forward provider streaming verbatim (e.g., over SSE) consume these.
// Consumers that only want lifecycle events ignore them.
type StreamPartEvent struct {
	Step int
	Part wingmodels.StreamPart
}

// ErrorEvent fires when the loop is about to terminate with an error.
// The error is also returned from Run; ErrorEvent exists so streaming
// consumers see it in-band before Run returns.
type ErrorEvent struct {
	Err error
}

// ContextTransformedEvent fires when a hook (BeforeStep or
// TransformContext) replaced the message slice with one of a different
// length. Phase is "before_step" (mutation persisted into running
// history) or "transform_context" (mutation ephemeral, applied only to
// this turn's request). Head is the first message of the post-hook
// slice when len > 0, nil otherwise; observers wanting to discriminate
// between hook kinds inspect Head's parts (e.g. a CompactionMarkerPart
// identifies a compaction-driven mutation). This keeps the loop
// ignorant of any specific hook's semantics.
type ContextTransformedEvent struct {
	Step          int
	Phase         string // "before_step" | "transform_context"
	OriginalCount int
	NewCount      int
	Head          *wingmodels.Message
}

func (IterationStartEvent) isEvent()      {}
func (IterationEndEvent) isEvent()        {}
func (MessageEvent) isEvent()             {}
func (ToolExecutionStartEvent) isEvent()  {}
func (ToolExecutionEndEvent) isEvent()    {}
func (StreamPartEvent) isEvent()          {}
func (ErrorEvent) isEvent()               {}
func (ContextTransformedEvent) isEvent()  {}
