package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/wingagent/tool"
	"github.com/chaserensberger/wingman/wingmodels"
)

// Run executes the loop with the given config until one of the
// termination conditions is reached:
//
//   - The assistant produces a turn with no tool calls (StopReasonEndTurn).
//   - The MaxSteps limit is hit (StopReasonMaxSteps).
//   - The context is cancelled (StopReasonAborted; Run returns ctx.Err()).
//   - A provider stream errors out (StopReasonError).
//   - A hook returns an error other than ErrSkipTool (StopReasonError).
//
// The returned Result.Messages is always populated, even on error, with
// whatever conversation state had been assembled when termination
// happened. This lets callers persist partial state.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.Model == nil {
		return nil, errors.New("loop.Run: Config.Model is required")
	}
	if cfg.Hooks.BeforeRun != nil && len(cfg.Messages) > 0 {
		return nil, errors.New("loop.Run: BeforeRun hook installed with non-empty Config.Messages; pick one source of initial history")
	}

	initial := append([]wingmodels.Message{}, cfg.Messages...)
	if cfg.Hooks.BeforeRun != nil {
		out, err := cfg.Hooks.BeforeRun(ctx, initial)
		if err != nil {
			return &Result{Messages: initial, StopReason: StopReasonError}, fmt.Errorf("hook BeforeRun: %w", err)
		}
		if out != nil {
			initial = out
		}
	}

	r := &runner{
		cfg:      cfg,
		messages: initial,
		registry: buildRegistry(cfg.Tools),
		toolDefs: buildToolDefs(cfg.Tools),
	}
	return r.run(ctx)
}

// runner holds per-Run mutable state. Separating it from Config keeps
// Config's contract immutable from the caller's perspective: hooks see
// transformed snapshots, never the live runner state.
type runner struct {
	cfg      Config
	messages []wingmodels.Message
	registry *tool.Registry
	toolDefs []wingmodels.ToolDef
	usage    wingmodels.Usage
}

// run is the main loop body.
func (r *runner) run(ctx context.Context) (*Result, error) {
	step := 0
	for {
		// Cancellation check at top of every iteration. Provider streams
		// honor ctx independently; this catches cancellations between
		// turns (e.g., during tool execution that ignored ctx).
		if err := ctx.Err(); err != nil {
			return r.finalize(step, StopReasonAborted), err
		}

		if r.cfg.MaxSteps > 0 && step >= r.cfg.MaxSteps {
			return r.finalize(step, StopReasonMaxSteps), nil
		}

		// BeforeStep hook. Runs before BeforeIteration so the hook sees
		// (and the per-turn hooks operate on) any persisted mutation
		// the BeforeStep returned. step+1 reflects the upcoming turn.
		// Compaction is the canonical user of this seam (shipped in
		// wingagent/hook).
		if r.cfg.Hooks.BeforeStep != nil {
			info := BeforeStepInfo{
				Step:     step + 1,
				Messages: r.messages,
				Usage:    r.usage,
				Model:    r.cfg.Model,
				Sink:     r.cfg.Sink,
			}
			newMsgs, err := r.cfg.Hooks.BeforeStep(ctx, info)
			if err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), fmt.Errorf("hook BeforeStep: %w", err)
			}
			if newMsgs != nil && len(newMsgs) != len(r.messages) {
				orig := len(r.messages)
				r.messages = newMsgs
				var head *wingmodels.Message
				if len(newMsgs) > 0 {
					h := newMsgs[0]
					head = &h
				}
				r.emit(ContextTransformedEvent{
					Step:          step + 1,
					Phase:         "before_step",
					OriginalCount: orig,
					NewCount:      len(newMsgs),
					Head:          head,
				})
			}
		}

		step++

		if r.cfg.Hooks.BeforeIteration != nil {
			if err := r.cfg.Hooks.BeforeIteration(ctx, step); err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), err
			}
		}
		r.emit(IterationStartEvent{Step: step})

		turn, err := r.runTurn(ctx, step)
		if err != nil {
			r.emitError(err)
			// Distinguish abort from generic error so callers can decide
			// whether to retry or surface the error.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return r.finalize(step, StopReasonAborted), err
			}
			return r.finalize(step, StopReasonError), err
		}

		r.emit(IterationEndEvent{Step: step, Turn: turn})

		if r.cfg.Hooks.AfterIteration != nil {
			if err := r.cfg.Hooks.AfterIteration(ctx, step, turn); err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), err
			}
		}

		// Termination: the assistant produced no tool calls. The model
		// considers itself done. We're done.
		if len(turn.Results) == 0 {
			return r.finalize(step, StopReasonEndTurn), nil
		}
	}
}

// runTurn streams one assistant message, executes any tool calls in it,
// and appends both the assistant message and the resulting tool result
// message (if any) to r.messages.
func (r *runner) runTurn(ctx context.Context, step int) (Turn, error) {
	// Build the per-turn request. TransformSystem and TransformContext
	// produce per-turn snapshots; r.messages is unchanged.
	system := r.cfg.System
	if r.cfg.Hooks.TransformSystem != nil {
		s, err := r.cfg.Hooks.TransformSystem(ctx, system)
		if err != nil {
			return Turn{}, fmt.Errorf("hook TransformSystem: %w", err)
		}
		system = s
	}

	msgs := r.messages
	if r.cfg.Hooks.TransformContext != nil {
		info := TransformContextInfo{
			Step:     step,
			Messages: append([]wingmodels.Message(nil), msgs...),
			Model:    r.cfg.Model,
		}
		m, err := r.cfg.Hooks.TransformContext(ctx, info)
		if err != nil {
			return Turn{}, fmt.Errorf("hook TransformContext: %w", err)
		}
		if m != nil && len(m) != len(msgs) {
			var head *wingmodels.Message
			if len(m) > 0 {
				h := m[0]
				head = &h
			}
			r.emit(ContextTransformedEvent{
				Step:          step,
				Phase:         "transform_context",
				OriginalCount: len(msgs),
				NewCount:      len(m),
				Head:          head,
			})
		}
		if m != nil {
			msgs = m
		}
	}

	// Note: any plugin-defined Part types that providers don't
	// understand must be reduced to core types by the plugin's own
	// TransformContextHook (the read-side seam). The loop is
	// deliberately unaware of any specific plugin's part types.

	req := wingmodels.Request{
		System:       system,
		Messages:     msgs,
		Tools:        r.toolDefs,
		ToolChoice:   r.cfg.ToolChoice,
		Capabilities: r.cfg.Capabilities,
	}

	stream, err := r.cfg.Model.Stream(ctx, req)
	if err != nil {
		return Turn{}, fmt.Errorf("model.Stream: %w", err)
	}

	// Drain the stream, forwarding raw parts to the sink. The stream's
	// terminal FinishPart carries the assembled assistant message via
	// stream.Final(); we also snapshot per-turn usage from FinishPart
	// here since stream.Final() only returns the message.
	var turnUsage wingmodels.Usage
	for part := range stream.Iter() {
		if fp, ok := part.(wingmodels.FinishPart); ok {
			turnUsage = fp.Usage
		}
		r.emit(StreamPartEvent{Step: step, Part: part})
	}
	assistantMsg, err := stream.Final()
	if err != nil {
		return Turn{}, fmt.Errorf("stream.Final: %w", err)
	}
	if assistantMsg == nil {
		return Turn{}, errors.New("model returned nil assistant message without error")
	}

	// Cumulative usage across the loop. Providers report cumulative
	// per-call counts; we sum because each turn is a fresh call.
	r.usage.InputTokens += turnUsage.InputTokens
	r.usage.OutputTokens += turnUsage.OutputTokens
	r.usage.TotalTokens += turnUsage.TotalTokens
	r.usage.ReasoningTokens += turnUsage.ReasoningTokens
	r.usage.CachedInputTokens += turnUsage.CachedInputTokens
	r.usage.CacheWriteTokens += turnUsage.CacheWriteTokens

	r.messages = append(r.messages, *assistantMsg)
	r.emit(MessageEvent{Message: *assistantMsg})

	calls := extractToolCalls(*assistantMsg)
	turn := Turn{
		Step:      step,
		Assistant: *assistantMsg,
		Usage:     turnUsage,
	}
	if len(calls) == 0 {
		return turn, nil
	}

	// Resolve each call against the registry. Unknown-tool calls get a
	// nil Tool; BeforeToolCall still fires so hooks can synthesize.
	resolved := make([]ToolCall, len(calls))
	for i, c := range calls {
		t, _ := r.registry.Get(c.Name) // ignore not-found; t will be nil
		args := c.Input
		if args == nil {
			args = map[string]any{}
		}
		resolved[i] = ToolCall{ID: c.CallID, Name: c.Name, Args: args, Tool: t}
	}

	// Decide execution mode for this batch.
	mode := r.cfg.ToolExecution
	if mode == ToolExecutionDefault {
		if anySequential(resolved) {
			mode = ToolExecutionSequential
		} else {
			mode = ToolExecutionParallel
		}
	}

	results := make([]ToolResult, len(resolved))
	switch mode {
	case ToolExecutionSequential:
		for i := range resolved {
			res, err := r.executeOne(ctx, resolved[i])
			if err != nil {
				return turn, err
			}
			results[i] = res
		}
	case ToolExecutionParallel:
		var wg sync.WaitGroup
		errCh := make(chan error, len(resolved))
		for i := range resolved {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				res, err := r.executeOne(ctx, resolved[i])
				if err != nil {
					errCh <- err
					return
				}
				// Safe: each goroutine writes a unique index, no overlap.
				results[i] = res
			}(i)
		}
		wg.Wait()
		close(errCh)
		// Surface the first hook error. There may be multiple; we
		// prioritize ctx errors over others to give clear cancellation
		// semantics.
		var firstErr error
		for e := range errCh {
			if firstErr == nil || (errors.Is(e, context.Canceled) && !errors.Is(firstErr, context.Canceled)) {
				firstErr = e
			}
		}
		if firstErr != nil {
			return turn, firstErr
		}
	default:
		return turn, fmt.Errorf("unknown ToolExecutionMode: %q", mode)
	}

	turn.Results = results

	// Build a single tool result message containing all results in
	// source order. Providers expect this shape: one message with
	// role=tool whose content is a sequence of ToolResultPart blocks.
	resultMsg := buildToolResultMessage(results)
	r.messages = append(r.messages, resultMsg)
	r.emit(MessageEvent{Message: resultMsg})

	return turn, nil
}

// executeOne runs the BeforeToolCall hook, dispatches the tool, runs the
// AfterToolCall hook, and emits start/end events. Returns the assembled
// ToolResult; the only error path is hook errors other than ErrSkipTool
// and provider/runtime panics-as-errors. Tool execution errors become
// part of the result (IsError=true), not return errors.
func (r *runner) executeOne(ctx context.Context, call ToolCall) (ToolResult, error) {
	r.emit(ToolExecutionStartEvent{Call: call})

	// BeforeToolCall: may rewrite args or skip.
	if r.cfg.Hooks.BeforeToolCall != nil {
		newArgs, err := r.cfg.Hooks.BeforeToolCall(ctx, call)
		if err != nil {
			if errors.Is(err, ErrSkipTool) {
				// Skip path: synthesize an error result, do not execute.
				args := newArgs
				if args == nil {
					args = call.Args
				}
				res := ToolResult{
					CallID:  call.ID,
					Name:    call.Name,
					Args:    args,
					Output:  err.Error(),
					IsError: true,
				}
				res = r.runAfterToolCall(ctx, call, res) // hook still fires
				r.emit(ToolExecutionEndEvent{Result: res})
				return res, nil
			}
			return ToolResult{}, fmt.Errorf("hook BeforeToolCall: %w", err)
		}
		if newArgs != nil {
			call.Args = newArgs
		}
	}

	// Unknown tool: synthesize an error result. We still go through
	// AfterToolCall so hooks see every call uniformly.
	if call.Tool == nil {
		res := ToolResult{
			CallID:  call.ID,
			Name:    call.Name,
			Args:    call.Args,
			Output:  fmt.Sprintf("tool %q is not registered", call.Name),
			IsError: true,
		}
		res = r.runAfterToolCall(ctx, call, res)
		r.emit(ToolExecutionEndEvent{Result: res})
		return res, nil
	}

	// Real execution. Tool errors become result text with IsError=true;
	// only hook errors fail the loop. This mirrors pi-mono's behavior
	// and means the model can recover from tool errors by trying again.
	start := time.Now()
	output, execErr := call.Tool.Execute(ctx, call.Args, r.cfg.WorkDir)
	duration := time.Since(start)

	res := ToolResult{
		CallID:   call.ID,
		Name:     call.Name,
		Args:     call.Args,
		Output:   output,
		IsError:  execErr != nil,
		Duration: duration,
	}
	if execErr != nil {
		res.Output = execErr.Error()
	}

	res = r.runAfterToolCall(ctx, call, res)
	r.emit(ToolExecutionEndEvent{Result: res})
	return res, nil
}

// runAfterToolCall runs the AfterToolCall hook if configured. Hook
// errors are surfaced via the error event but do not abort the call;
// they bubble up via the executeOne path's error return.
//
// Implementation note: we deliberately swallow hook errors here and let
// executeOne's caller handle them. That keeps executeOne's signature
// clean and avoids an extra error return path. The hook's effect on the
// result (the new output text) is applied iff it returns no error.
func (r *runner) runAfterToolCall(ctx context.Context, call ToolCall, res ToolResult) ToolResult {
	if r.cfg.Hooks.AfterToolCall == nil {
		return res
	}
	newOutput, err := r.cfg.Hooks.AfterToolCall(ctx, call, res.Output, res.IsError)
	if err != nil {
		// Surface as part of the result; the loop's caller will see the
		// hook error path through the next executeOne return. We DO NOT
		// return the error here because runAfterToolCall has no error
		// return; instead we annotate the result and let the loop carry
		// on. To make the loop fail on AfterToolCall errors, we'd need a
		// second return value here. v0.1 trade-off: AfterToolCall
		// errors are advisory only.
		res.Output = fmt.Sprintf("%s\n[after_tool_call hook error: %v]", res.Output, err)
		res.IsError = true
		return res
	}
	res.Output = newOutput
	return res
}

// emit forwards an event to the sink, if any. nil sink discards.
func (r *runner) emit(e Event) {
	if r.cfg.Sink == nil {
		return
	}
	r.cfg.Sink.OnEvent(e)
}

// emitError emits an ErrorEvent. Convenience over emit so the call sites
// read clearly.
func (r *runner) emitError(err error) {
	r.emit(ErrorEvent{Err: err})
}

// finalize builds the Result. Callers always get a non-nil Result so
// they can persist partial state on errors.
func (r *runner) finalize(step int, reason StopReason) *Result {
	return &Result{
		Messages:   r.messages,
		Usage:      r.usage,
		Steps:      step,
		StopReason: reason,
	}
}

// buildRegistry produces a Registry seeded with every tool. Loop callers
// could pass a pre-built Registry, but the per-Run cost is negligible
// (small map of pointers) and freshness avoids stale registrations.
func buildRegistry(ts []tool.Tool) *tool.Registry {
	reg := tool.NewRegistry()
	for _, t := range ts {
		reg.Register(t)
	}
	return reg
}

// buildToolDefs converts the configured tools' typed Definitions to the
// open-ended ToolDef shape providers expect.
func buildToolDefs(ts []tool.Tool) []wingmodels.ToolDef {
	if len(ts) == 0 {
		return nil
	}
	out := make([]wingmodels.ToolDef, len(ts))
	for i, t := range ts {
		out[i] = t.Definition().AsModelToolDef()
	}
	return out
}

// extractToolCalls pulls every ToolCallPart out of an assistant message
// in source order.
func extractToolCalls(msg wingmodels.Message) []wingmodels.ToolCallPart {
	var calls []wingmodels.ToolCallPart
	for _, p := range msg.Content {
		if c, ok := p.(wingmodels.ToolCallPart); ok {
			calls = append(calls, c)
		}
	}
	return calls
}

// anySequential reports whether any tool in calls implements
// SequentialTool and returns true. nil tools (unknown) are treated as
// parallel-safe (they don't actually execute anyway).
func anySequential(calls []ToolCall) bool {
	for _, c := range calls {
		if c.Tool == nil {
			continue
		}
		if seq, ok := c.Tool.(tool.SequentialTool); ok && seq.Sequential() {
			return true
		}
	}
	return false
}

// buildToolResultMessage constructs the wingmodels.Message that bundles
// all tool results from a batch. It uses RoleTool and one ToolResultPart
// per result. Providers (Anthropic, Ollama) translate this into their
// native tool-result shape on the wire.
//
// The output of each tool is wrapped in a single TextPart since v0.1
// tools return strings. Multimodal tool outputs are deferred.
func buildToolResultMessage(results []ToolResult) wingmodels.Message {
	content := make(wingmodels.Content, 0, len(results))
	for _, r := range results {
		content = append(content, wingmodels.ToolResultPart{
			CallID:  r.CallID,
			Output:  []wingmodels.Part{wingmodels.TextPart{Text: r.Output}},
			IsError: r.IsError,
		})
	}
	return wingmodels.Message{Role: wingmodels.RoleTool, Content: content}
}

// CoerceArgs turns an arbitrary value (typically the model's parsed tool
// input) into a map[string]any. Providers occasionally return JSON-RAW
// strings instead of maps; this helper normalizes both. Exported so
// hooks and tools can use it directly.
func CoerceArgs(v any) (map[string]any, error) {
	if v == nil {
		return map[string]any{}, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	if s, ok := v.(string); ok {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			return nil, fmt.Errorf("tool args is a string but not valid JSON: %w", err)
		}
		return m, nil
	}
	// Fallback: marshal then unmarshal. Handles structs that the model
	// somehow produced with typed fields.
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal tool args: %w", err)
	}
	return m, nil
}
