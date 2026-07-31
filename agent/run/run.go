package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/chaserensberger/wingman/models"
	"github.com/chaserensberger/wingman/permission"
	"github.com/chaserensberger/wingman/tool"
	"github.com/santhosh-tekuri/jsonschema/v6"
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
func Run(ctx context.Context, cfg Config) (result *Result, err error) {
	if cfg.Hooks.AfterRun != nil {
		defer func() {
			if result == nil {
				result = &Result{StopReason: StopReasonError}
			}
			if hookErr := cfg.Hooks.AfterRun(ctx, AfterRunInfo{Result: *result, Err: err}); hookErr != nil {
				err = errors.Join(err, hookErr)
			}
		}()
	}

	if cfg.Client == nil {
		return nil, errors.New("run.Run: Config.Client is required")
	}
	if cfg.Model.Provider == "" || cfg.Model.ID == "" {
		return nil, errors.New("run.Run: Config.Model is required")
	}
	if cfg.Hooks.BeforeRun != nil && len(cfg.Messages) > 0 {
		return nil, errors.New("run.Run: BeforeRun hook installed with non-empty Config.Messages; pick one source of initial history")
	}

	if cfg.OutputSchema != nil && !cfg.ModelInfo.Capabilities.StructuredOutput {
		info := cfg.ModelInfo
		return nil, fmt.Errorf("loop: model %s/%s does not support structured output", info.Provider, info.ID)
	}
	registry, err := tool.Compose(cfg.Tools)
	if err != nil {
		return nil, fmt.Errorf("run.Run: tool catalog: %w", err)
	}

	initial := append([]models.Message{}, cfg.Messages...)
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
		registry: registry,
		toolDefs: definitionsFor(registry),
	}

	// Serialize sink emission through a single drain goroutine. Tool
	// execution may run in parallel and emit events from worker
	// goroutines; the channel funnels them so Sink.OnEvent is always
	// called from one goroutine, preserving the documented contract.
	r.eventCh = make(chan Event, 64)
	r.eventWG.Add(1)
	go func() {
		defer r.eventWG.Done()
		for ev := range r.eventCh {
			if cfg.Sink != nil {
				cfg.Sink.OnEvent(ev)
			}
		}
	}()
	defer func() {
		close(r.eventCh)
		r.eventWG.Wait()
	}()

	return r.run(ctx)
}

// runner holds per-Run mutable state. Separating it from Config keeps
// Config's contract immutable from the caller's perspective: hooks see
// transformed snapshots, never the live runner state.
type runner struct {
	cfg      Config
	messages []models.Message
	turns    []Turn
	registry *tool.Registry
	toolDefs []models.ToolDef
	usage    models.Usage

	// structuredOutput is set on the terminal turn when an active schema
	// produced a valid JSON response.
	structuredOutput map[string]any

	// eventCh funnels every Sink event through a single drain
	// goroutine (started in Run). Workers can emit concurrently
	// during parallel tool execution; the drain serializes them.
	eventCh chan Event
	eventWG sync.WaitGroup
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

		// TransformHistory hook. Runs before OnTurnStart so the hook sees
		// (and the per-turn hooks operate on) any persisted mutation
		// the TransformHistory returned. step+1 reflects the upcoming turn.
		// Compaction is the canonical user of this seam (shipped in
		// agent/hook).
		if r.cfg.Hooks.TransformHistory != nil {
			info := TransformHistoryInfo{
				Step:      step + 1,
				Messages:  r.messages,
				Usage:     r.usage,
				Client:    r.cfg.Client,
				Model:     r.cfg.Model,
				ModelInfo: r.cfg.ModelInfo,
				// Transform hooks must enqueue through the runner rather than
				// bypassing its serialized sink drain.
				Sink: SinkFunc(r.emit),
			}
			newMsgs, err := r.cfg.Hooks.TransformHistory(ctx, info)
			if err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), fmt.Errorf("hook TransformHistory: %w", err)
			}
			if newMsgs != nil && len(newMsgs) != len(r.messages) {
				orig := len(r.messages)
				head := firstChangedMessage(r.messages, newMsgs)
				r.messages = newMsgs
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

		if r.cfg.Hooks.OnTurnStart != nil {
			if err := r.cfg.Hooks.OnTurnStart(ctx, step); err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), err
			}
		}
		r.emit(IterationStartEvent{Step: step})

		turn, err := r.runTurn(ctx, step)
		if err != nil {
			if !turn.StartedAt.IsZero() {
				r.turns = append(r.turns, turn)
			}
			r.emitError(err)
			// Distinguish abort from generic error so callers can decide
			// whether to retry or surface the error.
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return r.finalize(step, StopReasonAborted), err
			}
			return r.finalize(step, StopReasonError), err
		}

		r.turns = append(r.turns, turn)
		r.emit(IterationEndEvent{Step: step, Turn: turn})

		if r.cfg.Hooks.OnTurnEnd != nil {
			if err := r.cfg.Hooks.OnTurnEnd(ctx, step, turn); err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), err
			}
		}

		// Termination: the assistant produced no tool calls. The model
		// considers itself done. We're done.
		if len(turn.Results) == 0 {
			if err := r.handleStructuredOutput(turn); err != nil {
				r.emitError(err)
				return r.finalize(step, StopReasonError), err
			}
			return r.finalize(step, StopReasonEndTurn), nil
		}
	}
}

func firstChangedMessage(oldMsgs, newMsgs []models.Message) *models.Message {
	limit := len(oldMsgs)
	if len(newMsgs) < limit {
		limit = len(newMsgs)
	}
	for i := 0; i < limit; i++ {
		if !reflect.DeepEqual(oldMsgs[i], newMsgs[i]) {
			h := newMsgs[i]
			return &h
		}
	}
	if len(newMsgs) > len(oldMsgs) {
		h := newMsgs[len(oldMsgs)]
		return &h
	}
	if len(newMsgs) > 0 {
		h := newMsgs[0]
		return &h
	}
	return nil
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
			Step:      step,
			Messages:  append([]models.Message(nil), msgs...),
			Model:     r.cfg.Model,
			ModelInfo: r.cfg.ModelInfo,
		}
		m, err := r.cfg.Hooks.TransformContext(ctx, info)
		if err != nil {
			return Turn{}, fmt.Errorf("hook TransformContext: %w", err)
		}
		if m != nil && len(m) != len(msgs) {
			var head *models.Message
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

	toolDefs := r.toolDefs
	if r.cfg.Hooks.TransformToolDefs != nil {
		info := TransformToolDefsInfo{
			Step:      step,
			Tools:     append([]models.ToolDef(nil), toolDefs...),
			Model:     r.cfg.Model,
			ModelInfo: r.cfg.ModelInfo,
		}
		out, err := r.cfg.Hooks.TransformToolDefs(ctx, info)
		if err != nil {
			return Turn{}, fmt.Errorf("hook TransformToolDefs: %w", err)
		}
		toolDefs = out
	}

	params := SamplingParams{}
	if r.cfg.Hooks.TransformParams != nil {
		info := TransformParamsInfo{
			Step:      step,
			Model:     r.cfg.Model,
			ModelInfo: r.cfg.ModelInfo,
			Params:    params,
		}
		out, err := r.cfg.Hooks.TransformParams(ctx, info)
		if err != nil {
			return Turn{}, fmt.Errorf("hook TransformParams: %w", err)
		}
		params = out.Params
	}

	req := models.Request{
		Model:        r.cfg.Model,
		System:       system,
		Messages:     msgs,
		Tools:        toolDefs,
		ToolChoice:   r.cfg.ToolChoice,
		Capabilities: r.cfg.Capabilities,
		OutputSchema: r.cfg.OutputSchema,
	}
	if params.MaxOutputTokens != nil {
		req.MaxOutputTokens = *params.MaxOutputTokens
	}

	turn := Turn{Step: step, Attempt: 1}
	assistantMsg := models.Message{Role: models.RoleAssistant, State: models.MessageStateInProgress, Revision: 1}
	if err := r.checkpoint(ctx, step, &assistantMsg); err != nil {
		return Turn{}, r.retainFailedAssistant(ctx, step, &assistantMsg, err)
	}
	turn.Trace = models.NewCallTrace(req, models.LoweredOptions{})
	if lop, ok := r.cfg.Client.(interface {
		LoweredOptions(context.Context, models.Request) models.LoweredOptions
	}); ok {
		turn.Trace.Lowered = lop.LoweredOptions(ctx, req)
	}
	turn.StartedAt = time.Now()
	if r.cfg.ModelCallLifecycle != nil {
		callID, err := r.cfg.ModelCallLifecycle.Start(ctx, ModelCallStartInfo{
			Step:      turn.Step,
			Attempt:   turn.Attempt,
			MessageID: assistantMsg.ID,
			StartedAt: turn.StartedAt,
			Trace:     turn.Trace,
		})
		if err != nil {
			return Turn{}, r.retainFailedAssistant(ctx, step, &assistantMsg, fmt.Errorf("model call start: %w", err))
		}
		turn.ModelCallID = callID
	}

	streamCtx, cancelStream := context.WithCancel(ctx)
	defer cancelStream()
	stream, err := r.cfg.Client.Stream(streamCtx, req)
	if err != nil {
		turn.CompletedAt = time.Now()
		if withRequestID, ok := err.(interface{ ProviderRequestID() string }); ok {
			turn.ProviderRequestID = withRequestID.ProviderRequestID()
		}
		failure := fmt.Errorf("model stream: %w", err)
		turn.Failure = failure
		failure = r.retainFailedAssistant(ctx, step, &assistantMsg, failure)
		turn.Assistant = assistantMsg
		return turn, r.finishModelCall(ctx, turn, &assistantMsg, models.Usage{}, failure, failure)
	}

	// Drain the stream, forwarding raw parts to the sink. The stream's
	// terminal FinishPart carries the assembled assistant message via
	// stream.Final(); we also snapshot per-turn usage from FinishPart
	// here since stream.Final() only returns the message.
	var turnUsage models.Usage
	var finishReason models.FinishReason
	var providerRequestID string
	partIndexes := make(map[string]int)
	for part := range stream.Iter() {
		if fp, ok := part.(models.FinishPart); ok {
			turnUsage = fp.Usage
			finishReason = fp.Reason
		}
		if metadata, ok := part.(models.ResponseMetadataPart); ok {
			if requestID, ok := metadata.Meta["request_id"].(string); ok {
				providerRequestID = requestID
			}
		}
		partID, changed := applyStreamPart(&assistantMsg, partIndexes, part)
		if changed {
			assistantMsg.Revision++
			if err := r.checkpoint(ctx, step, &assistantMsg); err != nil {
				cancelStream()
				go func() {
					for range stream.Iter() {
					}
				}()
				err = r.retainFailedAssistant(ctx, step, &assistantMsg, err)
				turn.Assistant, turn.Failure = assistantMsg, err
				turn.CompletedAt, turn.Usage, turn.ProviderRequestID = time.Now(), turnUsage, providerRequestID
				return turn, r.finishModelCall(ctx, turn, &assistantMsg, turnUsage, err, err)
			}
			if partID != "" {
				partID = models.PartID(assistantMsg.Content[partIndexes[streamPartProviderID(part)]])
			}
		}
		r.emit(StreamPartEvent{Step: step, MessageID: assistantMsg.ID, PartID: partID, Revision: assistantMsg.Revision, Part: part})
	}
	finalMsg, err := stream.Final()
	turn.CompletedAt = time.Now()
	if err != nil {
		failure := fmt.Errorf("stream.Final: %w", err)
		turn.Failure = failure
		turn.Usage = turnUsage
		turn.ProviderRequestID = providerRequestID
		failure = r.retainFailedAssistant(ctx, step, &assistantMsg, failure)
		turn.Assistant = assistantMsg
		return turn, r.finishModelCall(ctx, turn, &assistantMsg, turnUsage, failure, failure)
	}
	if finalMsg == nil {
		err := errors.New("model returned nil assistant message without error")
		turn.Failure = err
		turn.Usage = turnUsage
		turn.ProviderRequestID = providerRequestID
		err = r.retainFailedAssistant(ctx, step, &assistantMsg, err)
		turn.Assistant, turn.Failure = assistantMsg, err
		return turn, r.finishModelCall(ctx, turn, &assistantMsg, turnUsage, err, err)
	}
	if !turnUsage.Empty() {
		finalMsg.Usage = &turnUsage
	}
	turn.Usage = turnUsage
	turn.ProviderRequestID = providerRequestID
	mergeFinalAssistant(&assistantMsg, *finalMsg)
	if finishReason != "" {
		assistantMsg.FinishReason = finishReason
	}
	assistantMsg.Revision++
	if err := r.checkpoint(ctx, step, &assistantMsg); err != nil {
		err = r.retainFailedAssistant(ctx, step, &assistantMsg, err)
		turn.Assistant, turn.Failure = assistantMsg, err
		return turn, r.finishModelCall(ctx, turn, &assistantMsg, turnUsage, nil, err)
	}
	if err := r.finishModelCall(ctx, turn, &assistantMsg, turnUsage, nil, nil); err != nil {
		return turn, err
	}

	// Cumulative usage across the run. Providers report cumulative
	// per-call counts; we sum because each turn is a fresh call.
	r.usage.InputTokens += turnUsage.InputTokens
	r.usage.OutputTokens += turnUsage.OutputTokens
	r.usage.TotalTokens += turnUsage.TotalTokens
	r.usage.ReasoningTokens += turnUsage.ReasoningTokens
	r.usage.CachedInputTokens += turnUsage.CachedInputTokens
	r.usage.CacheWriteTokens += turnUsage.CacheWriteTokens

	calls := extractToolCalls(assistantMsg)
	if len(calls) == 0 {
		assistantMsg.State = models.MessageStateCompleted
		assistantMsg.Revision++
		if err := r.checkpoint(ctx, step, &assistantMsg); err != nil {
			err = r.retainFailedAssistant(ctx, step, &assistantMsg, err)
			turn.Assistant, turn.Failure = assistantMsg, err
			return turn, err
		}
		r.messages = append(r.messages, assistantMsg)
		r.emit(MessageEvent{Step: step, Message: assistantMsg})
		turn.Assistant = assistantMsg
		return turn, nil
	}

	// Retain the assistant turn before executing tools so a hook failure does
	// not discard the model's tool calls. The terminal state is emitted below.
	assistantMsg.Content = toolPartsFromResults(assistantMsg.Content, nil)
	assistantMsg.Revision++
	if err := r.checkpoint(ctx, step, &assistantMsg); err != nil {
		err = r.retainFailedAssistant(ctx, step, &assistantMsg, err)
		turn.Assistant, turn.Failure = assistantMsg, err
		return turn, err
	}
	r.messages = append(r.messages, assistantMsg)
	// The checkpoint may have assigned canonical part IDs; resolve from that
	// snapshot so durable tool-use records reference persisted identities.
	calls = extractToolCalls(assistantMsg)

	// Resolve each call against the registry. Unknown-tool calls get a
	// nil Tool; BeforeToolCall still fires so hooks can synthesize.
	resolved := make([]ToolCall, len(calls))
	for i, c := range calls {
		t, _ := r.registry.Get(c.Name) // ignore not-found; t will be nil
		args := c.Input
		if args == nil {
			args = map[string]any{}
		}
		resolved[i] = ToolCall{ID: c.CallID, ToolUseID: c.ToolUseID, Name: c.Name, Args: args, Tool: t, MessageID: assistantMsg.ID, PartID: c.ID, ModelCallID: turn.ModelCallID, Step: step, Ordinal: i + 1}
	}
	if r.cfg.ToolUseLifecycle != nil {
		proposed := make([]ToolCall, 0, len(resolved))
		proposedIDs := make(map[string]struct{}, len(resolved))
		for i := range resolved {
			call := &resolved[i]
			call.ProposedAt = time.Now()
			id, proposalErr := r.cfg.ToolUseLifecycle.Propose(ctx, ToolUseProposeInfo{Step: call.Step, Ordinal: call.Ordinal, CallID: call.ID, Name: call.Name, Args: call.Args, MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID, ProposedAt: call.ProposedAt})
			if proposalErr != nil || id == "" {
				if proposalErr == nil {
					proposalErr = errors.New("tool use proposal returned empty ID")
				}
				runErr := fmt.Errorf("tool use proposal: %w", proposalErr)
				return turn, r.finalizeToolTurn(ctx, &turn, &assistantMsg, r.interruptToolUses(ctx, proposed, runErr, "proposal"), models.MessageStateFailed, runErr)
			}
			if _, exists := proposedIDs[id]; exists {
				proposalErr = fmt.Errorf("tool use proposal returned duplicate ID %q", id)
				runErr := fmt.Errorf("tool use proposal: %w", proposalErr)
				return turn, r.finalizeToolTurn(ctx, &turn, &assistantMsg, r.interruptToolUses(ctx, proposed, runErr, "proposal"), models.MessageStateFailed, runErr)
			}
			proposedIDs[id] = struct{}{}
			call.ToolUseID = id
			proposed = append(proposed, *call)
			assistantMsg.Content = applyToolUseIDs(assistantMsg.Content, resolved)
			r.emit(ToolUseProposedEvent{Call: *call})
		}
		assistantMsg.Content = applyToolUseIDs(assistantMsg.Content, resolved)
		assistantMsg.Revision++
		if checkpointErr := r.checkpoint(ctx, step, &assistantMsg); checkpointErr != nil {
			return turn, r.finalizeToolTurn(ctx, &turn, &assistantMsg, r.interruptToolUses(ctx, proposed, checkpointErr, "checkpoint"), models.MessageStateFailed, checkpointErr)
		}
		r.messages[len(r.messages)-1] = assistantMsg
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
			results[i] = res
			if err != nil {
				return turn, r.finalizeToolTurn(ctx, &turn, &assistantMsg, results, models.MessageStateFailed, err)
			}
		}
	case ToolExecutionParallel:
		var wg sync.WaitGroup
		errCh := make(chan error, len(resolved))
		for i := range resolved {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				res, err := r.executeOne(ctx, resolved[i])
				// Safe: each goroutine writes a unique index, no overlap.
				results[i] = res
				if err != nil {
					errCh <- err
					return
				}
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
			return turn, r.finalizeToolTurn(ctx, &turn, &assistantMsg, results, models.MessageStateFailed, firstErr)
		}
	default:
		return turn, fmt.Errorf("unknown ToolExecutionMode: %q", mode)
	}

	if err := r.finalizeToolTurn(ctx, &turn, &assistantMsg, results, models.MessageStateCompleted, nil); err != nil {
		return turn, err
	}
	return turn, nil
}

func (r *runner) checkpoint(ctx context.Context, step int, message *models.Message) error {
	if r.cfg.MessageCheckpoint == nil {
		return nil
	}
	saved, err := r.cfg.MessageCheckpoint.Save(ctx, MessageCheckpointInfo{Step: step, Message: *message})
	if err != nil {
		return fmt.Errorf("message checkpoint: %w", err)
	}
	*message = saved
	return nil
}

// retainFailedAssistant settles the local snapshot even when the caller's
// context was cancelled. It is intentionally best-effort after the first
// checkpoint failure: the original failure remains visible to the caller.
func (r *runner) retainFailedAssistant(ctx context.Context, step int, message *models.Message, runErr error) error {
	message.State = models.MessageStateFailed
	message.Revision++
	checkpointErr := r.checkpoint(context.WithoutCancel(ctx), step, message)
	r.messages = append(r.messages, *message)
	return errors.Join(runErr, checkpointErr)
}

func streamPartProviderID(part models.StreamPart) string {
	switch p := part.(type) {
	case models.TextStartPart:
		return p.ID
	case models.TextDeltaPart:
		return p.ID
	case models.TextEndPart:
		return p.ID
	case models.ReasoningStartPart:
		return p.ID
	case models.ReasoningDeltaPart:
		return p.ID
	case models.ReasoningEndPart:
		return p.ID
	case models.ToolInputStartPart:
		return p.ID
	case models.ToolInputDeltaPart:
		return p.ID
	case models.ToolInputEndPart:
		return p.ID
	case models.ToolCallPart_:
		return p.ID
	}
	return ""
}

// applyStreamPart updates the canonical snapshot and returns the provider
// block ID for a mutated durable part.
func applyStreamPart(message *models.Message, indexes map[string]int, part models.StreamPart) (string, bool) {
	id := streamPartProviderID(part)
	appendPart := func(p models.Part) {
		indexes[id] = len(message.Content)
		message.Content = append(message.Content, p)
	}
	switch p := part.(type) {
	case models.TextStartPart:
		appendPart(models.TextPart{Text: "", ProviderMetadata: p.ProviderMetadata})
	case models.TextDeltaPart:
		i, ok := indexes[id]
		if !ok {
			return "", false
		}
		text := message.Content[i].(models.TextPart)
		text.Text += p.Delta
		text.ProviderMetadata = p.ProviderMetadata
		message.Content[i] = text
	case models.TextEndPart:
		if _, ok := indexes[id]; !ok {
			return "", false
		}
	case models.ReasoningStartPart:
		appendPart(models.ReasoningPart{Reasoning: "", ProviderMetadata: p.ProviderMetadata})
	case models.ReasoningDeltaPart:
		i, ok := indexes[id]
		if !ok {
			return "", false
		}
		reasoning := message.Content[i].(models.ReasoningPart)
		reasoning.Reasoning += p.Delta
		reasoning.ProviderMetadata = p.ProviderMetadata
		message.Content[i] = reasoning
	case models.ReasoningEndPart:
		if _, ok := indexes[id]; !ok {
			return "", false
		}
	case models.ToolInputStartPart:
		appendPart(models.ToolPart{Name: p.ToolName, State: models.ToolStatePending, Input: map[string]any{}, ProviderMetadata: p.ProviderMetadata})
	case models.ToolInputDeltaPart:
		i, ok := indexes[id]
		if !ok {
			return "", false
		}
		toolPart := message.Content[i].(models.ToolPart)
		toolPart.InputRaw += p.Delta
		toolPart.ProviderMetadata = p.ProviderMetadata
		message.Content[i] = toolPart
	case models.ToolInputEndPart:
		if _, ok := indexes[id]; !ok {
			return "", false
		}
	case models.ToolCallPart_:
		i, ok := indexes[id]
		if !ok {
			appendPart(models.ToolPart{CallID: p.ID, Name: p.ToolName, State: models.ToolStatePending, Input: p.Input, ProviderExecuted: p.ProviderExecuted, ProviderMetadata: p.ProviderMetadata})
			return id, true
		}
		toolPart, ok := message.Content[i].(models.ToolPart)
		if !ok {
			return "", false
		}
		toolPart.CallID, toolPart.Name, toolPart.Input = p.ID, p.ToolName, p.Input
		toolPart.ProviderExecuted, toolPart.ProviderMetadata = p.ProviderExecuted, p.ProviderMetadata
		message.Content[i] = toolPart
	default:
		return "", false
	}
	return id, true
}

func mergeFinalAssistant(current *models.Message, final models.Message) {
	texts, reasonings := make(map[int]string), make(map[int]string)
	tools := make(map[string]models.ToolPart)
	textN, reasoningN := 0, 0
	for _, part := range current.Content {
		switch p := part.(type) {
		case models.TextPart:
			texts[textN] = p.ID
			textN++
		case models.ReasoningPart:
			reasonings[reasoningN] = p.ID
			reasoningN++
		case models.ToolPart:
			tools[p.CallID] = p
		case models.ToolCallPart:
			tools[p.CallID] = models.ToolPart{ID: p.ID, ToolUseID: p.ToolUseID, CallID: p.CallID, Name: p.Name, State: models.ToolStatePending, Input: p.Input, ProviderExecuted: p.ProviderExecuted, ProviderMetadata: p.ProviderMetadata}
		}
	}
	textN, reasoningN = 0, 0
	content := make(models.Content, len(final.Content))
	for i, part := range final.Content {
		id := ""
		switch p := part.(type) {
		case models.TextPart:
			id = texts[textN]
			textN++
		case models.ReasoningPart:
			id = reasonings[reasoningN]
			reasoningN++
		case models.ToolCallPart:
			if streamed, ok := tools[p.CallID]; ok {
				streamed.CallID = p.CallID
				streamed.Name = p.Name
				streamed.State = models.ToolStatePending
				streamed.Input = p.Input
				if streamed.ToolUseID == "" {
					streamed.ToolUseID = p.ToolUseID
				}
				streamed.ProviderExecuted = p.ProviderExecuted
				streamed.ProviderMetadata = p.ProviderMetadata
				part = streamed
				id = streamed.ID
			}
		case models.ToolPart:
			if streamed, ok := tools[p.CallID]; ok {
				p.ID = streamed.ID
				if p.ToolUseID == "" {
					p.ToolUseID = streamed.ToolUseID
				}
				if p.InputRaw == "" {
					p.InputRaw = streamed.InputRaw
				}
				part = p
				id = p.ID
			}
		}
		if id != "" {
			part = models.WithPartID(part, id)
		}
		content[i] = part
	}
	current.Content, current.FinishReason, current.Origin, current.Usage, current.Metadata = content, final.FinishReason, final.Origin, final.Usage, final.Metadata
}

// finishModelCall records terminal physical-call state exactly once. The
// caller's cancellation must not prevent durable settlement of that state.
func (r *runner) finishModelCall(ctx context.Context, turn Turn, assistant *models.Message, usage models.Usage, failure, runErr error) error {
	if r.cfg.ModelCallLifecycle == nil {
		return runErr
	}
	hookErr := r.cfg.ModelCallLifecycle.Finish(context.WithoutCancel(ctx), ModelCallFinishInfo{
		Step:              turn.Step,
		Attempt:           turn.Attempt,
		CallID:            turn.ModelCallID,
		StartedAt:         turn.StartedAt,
		CompletedAt:       turn.CompletedAt,
		Trace:             turn.Trace,
		Assistant:         assistant,
		Usage:             usage,
		ProviderRequestID: turn.ProviderRequestID,
		Failure:           failure,
	})
	return errors.Join(runErr, hookErr)
}

func (r *runner) finalizeToolTurn(ctx context.Context, turn *Turn, assistantMsg *models.Message, results []ToolResult, state models.MessageState, runErr error) error {
	turn.Results = completedToolResults(results)
	assistantMsg.Content = toolPartsFromResults(assistantMsg.Content, turn.Results)
	assistantMsg.State = state
	assistantMsg.Revision++
	if err := r.checkpoint(context.WithoutCancel(ctx), turn.Step, assistantMsg); err != nil {
		r.messages[len(r.messages)-1] = *assistantMsg
		turn.Assistant = *assistantMsg
		return errors.Join(runErr, err)
	}
	turn.Assistant = *assistantMsg
	r.messages[len(r.messages)-1] = *assistantMsg
	if state == models.MessageStateCompleted {
		r.emit(MessageEvent{Step: turn.Step, Message: *assistantMsg})
	}
	return runErr
}

func completedToolResults(results []ToolResult) []ToolResult {
	out := make([]ToolResult, 0, len(results))
	for _, result := range results {
		if result.CallID != "" || result.ToolUseID != "" {
			out = append(out, result)
		}
	}
	return out
}

func toolPartsFromResults(content models.Content, results []ToolResult) models.Content {
	byToolUseID := make(map[string]ToolResult, len(results))
	byCallID := make(map[string]ToolResult, len(results))
	for _, result := range results {
		if result.ToolUseID != "" {
			byToolUseID[result.ToolUseID] = result
		} else {
			byCallID[result.CallID] = result
		}
	}
	out := make(models.Content, 0, len(content))
	for _, part := range content {
		var toolPart models.ToolPart
		switch p := part.(type) {
		case models.ToolCallPart:
			toolPart = models.ToolPart{ID: p.ID, ToolUseID: p.ToolUseID, CallID: p.CallID, Name: p.Name, State: models.ToolStatePending, Input: p.Input, ProviderExecuted: p.ProviderExecuted, ProviderMetadata: p.ProviderMetadata}
		case models.ToolPart:
			toolPart = p
		default:
			out = append(out, part)
			continue
		}
		var result ToolResult
		var ok bool
		if toolPart.ToolUseID != "" {
			result, ok = byToolUseID[toolPart.ToolUseID]
		} else {
			result, ok = byCallID[toolPart.CallID]
		}
		if !ok {
			toolPart.State = models.ToolStatePending
			out = append(out, toolPart)
			continue
		}
		state := models.ToolStateCompleted
		if result.IsError {
			state = models.ToolStateError
		}
		completedAt := time.Now().UTC().UnixMilli()
		startedAt := completedAt - result.Duration.Milliseconds()
		toolPart.State = state
		toolPart.Input = result.Args
		if result.ToolUseID != "" {
			toolPart.ToolUseID = result.ToolUseID
		}
		toolPart.Output = result.Output
		toolPart.Structured = result.Structured
		toolPart.Metadata = result.Metadata
		toolPart.Error = result.Error
		toolPart.StartedAt = startedAt
		toolPart.CompletedAt = completedAt
		out = append(out, toolPart)
	}
	return out
}

// executeOne runs the BeforeToolCall hook, dispatches the tool, runs the
// AfterToolCall hook, and emits start/end events. Returns the assembled
// ToolResult; the only error path is hook errors other than ErrSkipTool
// and lifecycle transition errors. Tool execution errors become
// part of the result (IsError=true), not return errors.
func (r *runner) executeOne(ctx context.Context, call ToolCall) (ToolResult, error) {
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
					CallID: call.ID, ToolUseID: call.ToolUseID,
					Name:    call.Name,
					Args:    args,
					Error:   err.Error(),
					IsError: true,
				}
				return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "tool_skipped", err)
			}
			res := ToolResult{CallID: call.ID, ToolUseID: call.ToolUseID, Name: call.Name, Args: call.Args, Error: err.Error(), IsError: true}
			settled, settleErr := r.settleToolUse(ctx, call, res, ToolUseStatusFailed, "before_tool_call", err)
			return settled, errors.Join(fmt.Errorf("hook BeforeToolCall: %w", err), settleErr)
		}
		if newArgs != nil {
			call.Args = newArgs
		}
	}

	// Unknown tool: synthesize an error result. We still go through
	// AfterToolCall so hooks see every call uniformly.
	if call.Tool == nil {
		res := ToolResult{
			CallID: call.ID, ToolUseID: call.ToolUseID,
			Name:    call.Name,
			Args:    call.Args,
			Error:   fmt.Sprintf("tool %q is not registered", call.Name),
			IsError: true,
		}
		return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "unknown_tool", nil)
	}

	// Real execution. Tool errors become result text with IsError=true;
	// only hook errors fail the run.
	if err := validateToolInput(call.Tool, call.Args); err != nil {
		res := ToolResult{
			CallID: call.ID, ToolUseID: call.ToolUseID,
			Name:    call.Name,
			Args:    call.Args,
			Error:   err.Error(),
			IsError: true,
		}
		return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "input_validation", nil)
	}
	decision := r.checkPermission(call)
	if decision.err != nil {
		res := ToolResult{CallID: call.ID, ToolUseID: call.ToolUseID, Name: call.Name, Args: call.Args, Error: decision.err.Error(), IsError: true}
		return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "permission_check", decision.err)
	}
	if decision.denyResource != "" {
		res := permissionToolResult(call, permission.EffectDeny, decision.action, decision.denyResource)
		res.ToolUseID = call.ToolUseID
		return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "permission_denied", nil)
	}
	if len(decision.askResources) > 0 {
		if r.cfg.PermissionPrompter == nil {
			res := permissionToolResult(call, permission.EffectAsk, decision.action, decision.askResources[0])
			res.ToolUseID = call.ToolUseID
			return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "permission_unavailable", nil)
		}
		response, err := r.cfg.PermissionPrompter.Request(ctx, PermissionRequestInfo{
			Step: call.Step, Ordinal: call.Ordinal, ToolUseID: call.ToolUseID, CallID: call.ID, Name: call.Name, Args: call.Args,
			MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID, Action: decision.action, Resources: decision.askResources,
		})
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				cause := ctx.Err()
				if cause == nil {
					cause = err
				}
				errorType := "permission_interrupted"
				if errors.Is(cause, context.DeadlineExceeded) {
					errorType = "permission_timeout"
				}
				res := permissionToolResult(call, permission.EffectAsk, decision.action, decision.askResources[0])
				res.ToolUseID, res.Error = call.ToolUseID, cause.Error()
				settled, settleErr := r.settleToolUse(context.WithoutCancel(ctx), call, res, ToolUseStatusInterrupted, errorType, cause)
				return settled, errors.Join(cause, settleErr)
			}
			res := permissionToolResult(call, permission.EffectAsk, decision.action, decision.askResources[0])
			res.ToolUseID, res.Error = call.ToolUseID, err.Error()
			settled, settleErr := r.settleToolUse(ctx, call, res, ToolUseStatusFailed, "permission_prompt", err)
			return settled, errors.Join(fmt.Errorf("permission prompt: %w", err), settleErr)
		}
		switch response {
		case PermissionResponseOnce, PermissionResponseAlways:
		case PermissionResponseReject:
			res := permissionToolResult(call, permission.EffectAsk, decision.action, decision.askResources[0])
			res.ToolUseID = call.ToolUseID
			res.Metadata["error_type"] = "permission_denied"
			res.Metadata["permission"].(map[string]any)["response"] = "reject"
			return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "permission_denied", nil)
		default:
			res := permissionToolResult(call, permission.EffectAsk, decision.action, decision.askResources[0])
			res.ToolUseID = call.ToolUseID
			return r.settleToolUse(ctx, call, res, ToolUseStatusDeclined, "permission_invalid_response", nil)
		}
	}
	if r.cfg.ToolUseLifecycle != nil {
		authorizeInfo := toolUseAuthorizeInfo(call, time.Now())
		if err := r.cfg.ToolUseLifecycle.Authorize(ctx, authorizeInfo); err != nil {
			res := ToolResult{CallID: call.ID, ToolUseID: call.ToolUseID, Name: call.Name, Args: call.Args, Error: err.Error(), IsError: true}
			settled, settleErr := r.settleToolUse(ctx, call, res, ToolUseStatusFailed, "authorize", err)
			return settled, errors.Join(fmt.Errorf("tool use authorize: %w", err), settleErr)
		}
		call.AuthorizedAt = authorizeInfo.AuthorizedAt
		r.emit(ToolUseAuthorizedEvent{Call: call})
		startInfo := toolUseStartInfo(call, time.Now())
		if err := r.cfg.ToolUseLifecycle.Start(ctx, startInfo); err != nil {
			res := ToolResult{CallID: call.ID, ToolUseID: call.ToolUseID, Name: call.Name, Args: call.Args, Error: err.Error(), IsError: true}
			settled, settleErr := r.settleToolUse(ctx, call, res, ToolUseStatusFailed, "start", err)
			return settled, errors.Join(fmt.Errorf("tool use start: %w", err), settleErr)
		}
		call.StartedAt = startInfo.StartedAt
	}
	r.emit(ToolExecutionStartEvent{Call: call})
	start := time.Now()
	inv := tool.Invocation{
		Input:       call.Args,
		WorkDir:     r.cfg.WorkDir,
		SessionID:   r.cfg.SessionID,
		RunID:       r.cfg.RunID,
		AgentID:     r.cfg.AgentID,
		ToolUseID:   call.ToolUseID,
		CallID:      call.ID,
		MessageID:   call.MessageID,
		PartID:      call.PartID,
		ModelCallID: call.ModelCallID,
		Progress: tool.NewProgress(func(delta string, metadata map[string]any) {
			r.emit(ToolExecutionProgressEvent{
				CallID:      call.ID,
				ToolUseID:   call.ToolUseID,
				Name:        call.Name,
				OutputDelta: delta,
				Metadata:    metadata,
			})
		}),
	}
	toolResult, execErr := call.Tool.Execute(ctx, inv)
	duration := time.Since(start)

	res := ToolResult{
		CallID: call.ID, ToolUseID: call.ToolUseID,
		Name:       call.Name,
		Args:       call.Args,
		Output:     toolResult.Text,
		Structured: toolResult.Structured,
		Metadata:   toolResult.Metadata,
		IsError:    execErr != nil,
		Duration:   duration,
	}
	if execErr != nil {
		res.Error = execErr.Error()
	}

	status, errorType := ToolUseStatusCompleted, ""
	if execErr != nil {
		status, errorType = ToolUseStatusFailed, "execution"
		if errors.Is(execErr, context.Canceled) || errors.Is(execErr, context.DeadlineExceeded) {
			status, errorType = ToolUseStatusInterrupted, "interrupted"
		}
	} else if schema := call.Tool.Definition().OutputSchema; schema != nil {
		if toolResult.Structured == nil {
			execErr = &tool.ResultValidationError{Tool: call.Name, Err: errors.New("structured result is required")}
		} else if err := validateAgainstSchema(schema, toolResult.Structured); err != nil {
			execErr = &tool.ResultValidationError{Tool: call.Name, Err: err}
		}
		if execErr != nil {
			res.Error = execErr.Error()
			res.ErrorType = "result_validation"
			res.IsError = true
			status, errorType = ToolUseStatusFailed, "result_validation"
		}
	}
	return r.settleToolUse(ctx, call, res, status, errorType, execErr)
}

func toolUseAuthorizeInfo(call ToolCall, authorizedAt time.Time) ToolUseAuthorizeInfo {
	return ToolUseAuthorizeInfo{Step: call.Step, Ordinal: call.Ordinal, ToolUseID: call.ToolUseID, CallID: call.ID, Name: call.Name, Args: call.Args, MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID, AuthorizedAt: authorizedAt}
}

func toolUseStartInfo(call ToolCall, startedAt time.Time) ToolUseStartInfo {
	return ToolUseStartInfo{Step: call.Step, Ordinal: call.Ordinal, ToolUseID: call.ToolUseID, CallID: call.ID, Name: call.Name, Args: call.Args, MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID, StartedAt: startedAt}
}

// settleToolUse runs the post-call hook before durable terminal accounting.
func (r *runner) settleToolUse(ctx context.Context, call ToolCall, res ToolResult, status ToolUseStatus, errorType string, failure error) (ToolResult, error) {
	updated, afterErr := r.runAfterToolCallResult(ctx, call, res)
	if afterErr != nil {
		status, errorType, failure = ToolUseStatusFailed, "after_tool_call", afterErr
	}
	updated.Status = status
	if r.cfg.ToolUseLifecycle != nil {
		info := ToolUseFinishInfo{Step: call.Step, Ordinal: call.Ordinal, ToolUseID: call.ToolUseID, CallID: call.ID, Name: call.Name, Args: call.Args, MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID, ProposedAt: call.ProposedAt, AuthorizedAt: call.AuthorizedAt, StartedAt: call.StartedAt, CompletedAt: time.Now(), Status: status, ToolResult: updated, ErrorType: errorType, ErrorMessage: updated.Error, Failure: failure}
		if info.ErrorMessage == "" && failure != nil {
			info.ErrorMessage = failure.Error()
		}
		if err := r.cfg.ToolUseLifecycle.Finish(context.WithoutCancel(ctx), info); err != nil {
			if updated.Error != "" {
				updated.Error += "\n"
			}
			updated.Error += fmt.Sprintf("tool use finish: %v", err)
			updated.IsError = true
			return updated, errors.Join(afterErr, fmt.Errorf("tool use finish: %w", err))
		}
	}
	r.emit(ToolExecutionEndEvent{Result: updated})
	return updated, afterErr
}

func (r *runner) interruptToolUses(ctx context.Context, calls []ToolCall, cause error, errorType string) []ToolResult {
	if r.cfg.ToolUseLifecycle == nil {
		return nil
	}
	results := make([]ToolResult, 0, len(calls))
	for _, call := range calls {
		res := ToolResult{CallID: call.ID, ToolUseID: call.ToolUseID, Status: ToolUseStatusInterrupted, Name: call.Name, Args: call.Args, Error: cause.Error(), IsError: true}
		results = append(results, res)
		if err := r.cfg.ToolUseLifecycle.Finish(context.WithoutCancel(ctx), ToolUseFinishInfo{Step: call.Step, Ordinal: call.Ordinal, ToolUseID: call.ToolUseID, CallID: call.ID, Name: call.Name, Args: call.Args, MessageID: call.MessageID, PartID: call.PartID, ModelCallID: call.ModelCallID, ProposedAt: call.ProposedAt, AuthorizedAt: call.AuthorizedAt, StartedAt: call.StartedAt, CompletedAt: time.Now(), Status: ToolUseStatusInterrupted, ToolResult: res, ErrorType: errorType, ErrorMessage: cause.Error(), Failure: cause}); err == nil {
			r.emit(ToolExecutionEndEvent{Result: res})
		}
	}
	return results
}

func applyToolUseIDs(content models.Content, calls []ToolCall) models.Content {
	ids := make(map[string]string, len(calls))
	for _, call := range calls {
		ids[call.PartID] = call.ToolUseID
	}
	out := append(models.Content(nil), content...)
	for i, part := range out {
		switch p := part.(type) {
		case models.ToolPart:
			p.ToolUseID = ids[p.ID]
			out[i] = p
		case models.ToolCallPart:
			p.ToolUseID = ids[p.ID]
			out[i] = p
		}
	}
	return out
}

type permissionDecision struct {
	action       string
	askResources []string
	denyResource string
	err          error
}

func (r *runner) checkPermission(call ToolCall) permissionDecision {
	if len(r.cfg.Permissions) == 0 {
		return permissionDecision{}
	}
	action, resources, err := permissionTarget(call, r.cfg.WorkDir)
	if err != nil {
		return permissionDecision{err: err}
	}
	if len(resources) == 0 {
		resources = []string{"*"}
	}
	decision := permissionDecision{action: action}
	for _, resource := range resources {
		evaluated := permission.Evaluate(action, resource, r.cfg.Permissions, permission.EffectAllow)
		switch evaluated.Effect {
		case permission.EffectDeny:
			decision.denyResource = resource
			return decision
		case permission.EffectAsk:
			decision.askResources = append(decision.askResources, resource)
		}
	}
	return decision
}

func permissionToolResult(call ToolCall, effect permission.Effect, action, resource string) ToolResult {
	label := "permission denied"
	if effect == permission.EffectAsk {
		label = "permission required"
	}
	return ToolResult{
		CallID:  call.ID,
		Name:    call.Name,
		Args:    call.Args,
		Error:   fmt.Sprintf("%s: %s %s", label, action, resource),
		IsError: true,
		Metadata: map[string]any{
			"permission": map[string]any{
				"effect":   string(effect),
				"action":   action,
				"resource": resource,
			},
		},
	}
}

func permissionTarget(call ToolCall, workDir string) (string, []string, error) {
	if call.Tool != nil {
		check, declared, err := tool.PermissionFor(call.Tool, tool.Invocation{Input: call.Args, WorkDir: workDir})
		if err != nil {
			return "", nil, err
		}
		if declared {
			return check.Action, check.Resources, nil
		}
	}
	switch call.Name {
	case "write", "edit":
		return "edit", pathResources(call, workDir), nil
	case "apply_patch":
		return "edit", []string{"*"}, nil
	case "read":
		return "read", pathResources(call, workDir), nil
	case "bash":
		return "bash", []string{stringArg(call.Args, "command", "*")}, nil
	case "glob", "grep":
		return call.Name, []string{stringArg(call.Args, "pattern", "*")}, nil
	case "webfetch":
		return "webfetch", []string{stringArg(call.Args, "url", "*")}, nil
	case "websearch":
		return "websearch", []string{stringArg(call.Args, "query", "*")}, nil
	default:
		return call.Name, []string{"*"}, nil
	}
}

func pathResources(call ToolCall, workDir string) []string {
	if call.Name == "apply_patch" {
		return []string{"*"}
	}
	path := stringArg(call.Args, "filePath", "")
	if path == "" {
		path = stringArg(call.Args, "path", "")
	}
	if path == "" {
		return []string{"*"}
	}
	if workDir != "" {
		if rel := relativeResource(workDir, path); rel != "" {
			return []string{rel}
		}
	}
	return []string{filepath.ToSlash(filepath.Clean(path))}
}

func relativeResource(workDir, raw string) string {
	path := raw
	if !filepath.IsAbs(path) {
		path = filepath.Join(workDir, path)
	}
	path = filepath.Clean(path)
	workDir = filepath.Clean(workDir)
	rel, err := filepath.Rel(workDir, path)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func stringArg(args map[string]any, key, fallback string) string {
	value, ok := args[key].(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func validateToolInput(t tool.Tool, args map[string]any) error {
	def := t.Definition().AsModelToolDef()
	if len(def.InputSchema) == 0 {
		return nil
	}
	schema := def.InputSchema
	b, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("tool %q input schema marshal error: %w", t.Name(), err)
	}
	if err := json.Unmarshal(b, &schema); err != nil {
		return fmt.Errorf("tool %q input schema normalize error: %w", t.Name(), err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("tool-input.json", schema); err != nil {
		return fmt.Errorf("tool %q input schema compile error: %w", t.Name(), err)
	}
	sch, err := c.Compile("tool-input.json")
	if err != nil {
		return fmt.Errorf("tool %q input schema compile error: %w", t.Name(), err)
	}
	if err := sch.Validate(args); err != nil {
		return fmt.Errorf("tool %q input validation error: %w", t.Name(), err)
	}
	return nil
}

// runAfterToolCall runs the AfterToolCall hook if configured. Hook
// errors are surfaced via the error event but do not abort the call;
// they bubble up via the executeOne path's error return.
//
// Implementation note: we deliberately swallow hook errors here and let
// executeOne's caller handle them. That keeps executeOne's signature
// clean and avoids an extra error return path. The hook's effect on the
// result is applied iff it returns no error.
func (r *runner) runAfterToolCall(ctx context.Context, call ToolCall, res ToolResult) ToolResult {
	updated, _ := r.runAfterToolCallResult(ctx, call, res)
	return updated
}

func (r *runner) runAfterToolCallResult(ctx context.Context, call ToolCall, res ToolResult) (ToolResult, error) {
	if r.cfg.Hooks.AfterToolCall == nil {
		return res, nil
	}
	updated, err := r.cfg.Hooks.AfterToolCall(ctx, call, res)
	if err != nil {
		// Surface as part of the result; the loop's caller will see the
		// hook error path through the next executeOne return. We DO NOT
		// return the error here because runAfterToolCall has no error
		// return; instead we annotate the result and let the loop carry
		// on. To make the loop fail on AfterToolCall errors, we'd need a
		// second return value here. v0.1 trade-off: AfterToolCall
		// errors are advisory only.
		if res.Error != "" {
			res.Error += "\n"
		}
		res.Error += fmt.Sprintf("after_tool_call hook error: %v", err)
		res.IsError = true
		return res, err
	}
	updated.CallID = res.CallID
	updated.ToolUseID = res.ToolUseID
	updated.Name = res.Name
	updated.Args = res.Args
	updated.Duration = res.Duration
	return updated, nil
}

// emit forwards an event to the sink via the drain goroutine. Safe to
// call from any goroutine; ordering across concurrent callers is
// determined by channel send order.
func (r *runner) emit(e Event) {
	r.eventCh <- e
}

// emitError emits an ErrorEvent. Convenience over emit so the call sites
// read clearly.
func (r *runner) emitError(err error) {
	r.emit(ErrorEvent{Err: err})
}

// handleStructuredOutput parses and validates the assistant's final text
// when an OutputSchema is active. On success it populates
// r.structuredOutput and emits a StructuredOutputEvent.
func (r *runner) handleStructuredOutput(turn Turn) error {
	if r.cfg.OutputSchema == nil {
		return nil
	}
	text := textOf(turn.Assistant)
	if text == "" {
		return fmt.Errorf("loop: structured output required but assistant returned empty text")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return fmt.Errorf("loop: structured output parse error: %w (raw: %s)", err, text)
	}
	if err := validateAgainstSchema(r.cfg.OutputSchema.Schema, parsed); err != nil {
		return fmt.Errorf("loop: structured output validation error: %w (raw: %s)", err, text)
	}
	r.structuredOutput = parsed
	r.emit(StructuredOutputEvent{
		Schema:  r.cfg.OutputSchema.Name,
		RawJSON: text,
		Parsed:  parsed,
	})
	return nil
}

func validateAgainstSchema(schema map[string]any, value any) error {
	c := jsonschema.NewCompiler()
	if err := c.AddResource("schema.json", schema); err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := sch.Validate(value); err != nil {
		return err
	}
	return nil
}

// textOf concatenates every TextPart in a message in source order.
func textOf(msg models.Message) string {
	var out string
	for _, p := range msg.Content {
		if t, ok := p.(models.TextPart); ok {
			out += t.Text
		}
	}
	return out
}

// finalize builds the Result. Callers always get a non-nil Result so
// they can persist partial state on errors.
func (r *runner) finalize(step int, reason StopReason) *Result {
	return &Result{
		Messages:         r.messages,
		Turns:            r.turns,
		Usage:            r.usage,
		Steps:            step,
		StopReason:       reason,
		StructuredOutput: r.structuredOutput,
	}
}

// definitionsFor converts one composed tool catalog to model definitions.
func definitionsFor(registry *tool.Registry) []models.ToolDef {
	definitions := registry.Definitions()
	if len(definitions) == 0 {
		return nil
	}
	out := make([]models.ToolDef, len(definitions))
	for i, definition := range definitions {
		out[i] = definition.AsModelToolDef()
	}
	return out
}

// extractToolCalls pulls every ToolCallPart out of an assistant message
// in source order.
func extractToolCalls(msg models.Message) []models.ToolCallPart {
	var calls []models.ToolCallPart
	for _, p := range msg.Content {
		switch c := p.(type) {
		case models.ToolCallPart:
			calls = append(calls, c)
		case models.ToolPart:
			calls = append(calls, models.ToolCallPart{ID: c.ID, ToolUseID: c.ToolUseID, CallID: c.CallID, Name: c.Name, Input: c.Input, ProviderExecuted: c.ProviderExecuted, ProviderMetadata: c.ProviderMetadata})
		}
	}
	return calls
}

// anySequential reports whether any tool in calls requires sequential
// execution. nil tools (unknown) are treated as parallel-safe because they do
// not execute.
func anySequential(calls []ToolCall) bool {
	for _, c := range calls {
		if c.Tool == nil {
			continue
		}
		if tool.IsSequential(c.Tool) {
			return true
		}
	}
	return false
}

// buildToolResultMessage constructs the models.Message that bundles
// all tool results from a batch. It uses RoleTool and one ToolResultPart
// per result. Providers (Anthropic, Ollama) translate this into their
// native tool-result shape on the wire.
//
// The output of each tool is wrapped in a single TextPart since v0.1
// tools return strings. Multimodal tool outputs are deferred.
func buildToolResultMessage(results []ToolResult) models.Message {
	content := make(models.Content, 0, len(results))
	for _, r := range results {
		text := toolResultText(r)
		content = append(content, models.ToolResultPart{
			CallID: r.CallID, ToolUseID: r.ToolUseID,
			Name:     r.Name,
			Output:   []models.Part{models.TextPart{Text: text}},
			IsError:  r.IsError,
			Metadata: r.Metadata,
		})
	}
	return models.Message{Role: models.RoleTool, Content: content}
}

// toolResultText builds the model-facing text from a ToolResult. When
// an error occurred, the partial output is preserved and the error is
// appended so the model can see both.
func toolResultText(r ToolResult) string {
	if r.IsError {
		if r.Output != "" && r.Error != "" {
			return r.Output + "\n" + r.Error
		}
		if r.Error != "" {
			return r.Error
		}
	}
	return r.Output
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
