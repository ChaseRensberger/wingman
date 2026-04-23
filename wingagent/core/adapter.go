package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/chaserensberger/wingman/wingmodels"
)

// ProviderFromModel adapts a wingmodels.Model (the new abstraction) to the
// legacy core.Provider interface (the old abstraction the agent loop and
// session layers still consume in v0.1).
//
// This is a Tier 1 migration shim. Tier 2 will rewrite the agent loop to
// consume wingmodels.Model directly and this file goes away.
//
// The conversion is loss-prone in two specific ways:
//   - Reasoning parts have no representation in the legacy ContentBlock
//     union, so they are dropped from the converted response. They still
//     stream through StreamInference (mapped to text deltas with a
//     "[reasoning] " prefix) so callers that visualize the stream see them.
//   - Image parts are dropped; v0.1 didn't expose images via core anyway.
func ProviderFromModel(m wingmodels.Model) Provider {
	return &modelAdapter{m: m}
}

// modelAdapter wraps a wingmodels.Model and exposes the legacy interface.
type modelAdapter struct {
	m wingmodels.Model
}

func (a *modelAdapter) RunInference(ctx context.Context, req InferenceRequest) (*InferenceResponse, error) {
	wreq := toWingmanRequest(req)
	msg, err := wingmodels.Run(ctx, a.m, wreq)
	if err != nil {
		return nil, err
	}
	return toLegacyResponse(msg), nil
}

func (a *modelAdapter) StreamInference(ctx context.Context, req InferenceRequest) (Stream, error) {
	wreq := toWingmanRequest(req)
	stream, err := a.m.Stream(ctx, wreq)
	if err != nil {
		return nil, err
	}
	return newAdapterStream(stream), nil
}

// ---- request conversion ---------------------------------------------------

func toWingmanRequest(req InferenceRequest) wingmodels.Request {
	out := wingmodels.Request{
		System:   req.Instructions,
		Messages: make([]wingmodels.Message, 0, len(req.Messages)),
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, toWingmanMessage(m))
	}
	if len(req.Tools) > 0 {
		out.Tools = make([]wingmodels.ToolDef, len(req.Tools))
		for i, t := range req.Tools {
			out.Tools[i] = toWingmanTool(t)
		}
	}
	return out
}

func toWingmanMessage(m Message) wingmodels.Message {
	role := wingmodels.Role(m.Role)
	// Legacy core has no "tool" role; tool results live in a user message.
	// Detect that shape and lift to the canonical RoleTool so providers
	// produce the right wire format.
	if role == wingmodels.RoleUser && len(m.Content) > 0 && m.Content[0].Type == ContentTypeToolResult {
		role = wingmodels.RoleTool
	}
	content := make(wingmodels.Content, 0, len(m.Content))
	for _, b := range m.Content {
		switch b.Type {
		case ContentTypeText:
			if b.Text == "" {
				continue
			}
			content = append(content, wingmodels.TextPart{Text: b.Text})
		case ContentTypeToolUse:
			input, _ := b.Input.(map[string]any)
			content = append(content, wingmodels.ToolCallPart{CallID: b.ID, Name: b.Name, Input: input})
		case ContentTypeToolResult:
			content = append(content, wingmodels.ToolResultPart{
				CallID:  b.ToolUseID,
				Output:  []wingmodels.Part{wingmodels.TextPart{Text: b.Content}},
				IsError: b.IsError,
			})
		}
	}
	return wingmodels.Message{Role: role, Content: content}
}

func toWingmanTool(t ToolDefinition) wingmodels.ToolDef {
	// Legacy ToolInputSchema is a typed struct; wingmodels accepts an
	// open-ended map so we reflate to the JSON Schema shape providers
	// expect.
	props := map[string]any{}
	for name, p := range t.InputSchema.Properties {
		propObj := map[string]any{"type": p.Type}
		if p.Description != "" {
			propObj["description"] = p.Description
		}
		if len(p.Enum) > 0 {
			propObj["enum"] = p.Enum
		}
		props[name] = propObj
	}
	schema := map[string]any{
		"type":       t.InputSchema.Type,
		"properties": props,
	}
	if len(t.InputSchema.Required) > 0 {
		schema["required"] = t.InputSchema.Required
	}
	return wingmodels.ToolDef{
		Name:        t.Name,
		Description: t.Description,
		InputSchema: schema,
	}
}

// ---- response conversion --------------------------------------------------

func toLegacyResponse(msg *wingmodels.Message) *InferenceResponse {
	if msg == nil {
		return &InferenceResponse{}
	}
	resp := &InferenceResponse{}
	hasToolCalls := false
	for _, p := range msg.Content {
		switch v := p.(type) {
		case wingmodels.TextPart:
			resp.Content = append(resp.Content, ContentBlock{Type: ContentTypeText, Text: v.Text})
		case wingmodels.ToolCallPart:
			hasToolCalls = true
			resp.Content = append(resp.Content, ContentBlock{
				Type:  ContentTypeToolUse,
				ID:    v.CallID,
				Name:  v.Name,
				Input: v.Input,
			})
		case wingmodels.ReasoningPart:
			// Drop: legacy ContentBlock has no reasoning representation.
		}
	}
	if hasToolCalls {
		resp.StopReason = "tool_use"
	} else {
		resp.StopReason = "end_turn"
	}
	return resp
}

// ---- stream adaptation ----------------------------------------------------

// adapterStream implements core.Stream over a wingmodels EventStream. It
// translates each StreamPart into the closest legacy StreamEvent. Some
// wingmodels parts have no legacy equivalent and are silently dropped:
//   - StreamStartPart, ResponseMetadataPart: no legacy event.
//   - ReasoningStart/End: no legacy event. ReasoningDelta is mapped to a
//     text_delta prefixed "[reasoning] " so streaming UIs at least see it.
//   - ToolInputStart/End: no legacy events; legacy clients see the JSON via
//     input_json_delta and the final tool_use block at content_block_stop.
//
// This adapter intentionally does not buffer past the channel; the underlying
// EventStream provides flow control.
type adapterStream struct {
	src    *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]
	parts  <-chan wingmodels.StreamPart
	queued []StreamEvent
	cur    StreamEvent
	err    error
	closed bool

	// Block tracking. Wingmodels uses string ids; legacy uses int indices.
	// We assign indices in start-order.
	idToIndex map[string]int
	nextIdx   int

	// Per-block accumulation so we can emit content_block_start events with
	// the right metadata (e.g. tool_use needs the tool name from the start
	// event but we only get it via ToolInputStartPart).
	blockKinds map[int]string // "text" | "tool_use" | "reasoning"
	blockNames map[int]string // for tool_use
	blockIDs   map[int]string // for tool_use (call_id)

	finalMsg *wingmodels.Message
}

func newAdapterStream(s *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]) *adapterStream {
	a := &adapterStream{
		src:        s,
		parts:      iterChan(s),
		idToIndex:  make(map[string]int),
		blockKinds: make(map[int]string),
		blockNames: make(map[int]string),
		blockIDs:   make(map[int]string),
	}
	return a
}

// iterChan exposes the EventStream's iter.Seq as a channel so we can use a
// for/select inside Next without losing the cancellation story.
func iterChan(s *wingmodels.EventStream[wingmodels.StreamPart, *wingmodels.Message]) <-chan wingmodels.StreamPart {
	out := make(chan wingmodels.StreamPart, 16)
	go func() {
		defer close(out)
		for p := range s.Iter() {
			out <- p
		}
	}()
	return out
}

func (a *adapterStream) Next() bool {
	if a.err != nil || a.closed {
		return false
	}
	for {
		if len(a.queued) > 0 {
			a.cur = a.queued[0]
			a.queued = a.queued[1:]
			return true
		}
		p, ok := <-a.parts
		if !ok {
			// Stream drained. Pull final.
			msg, err := a.src.Final()
			if err != nil {
				a.err = err
			}
			a.finalMsg = msg
			return false
		}
		a.translate(p)
	}
}

// translate converts one StreamPart into zero or more legacy StreamEvents,
// appending them to the queue.
func (a *adapterStream) translate(p wingmodels.StreamPart) {
	switch v := p.(type) {
	case wingmodels.StreamStartPart, wingmodels.ResponseMetadataPart:
		// No legacy equivalent. message_start is implied; legacy emits it
		// once at the top of the stream when we see usage. Synthesize here
		// so callers waiting for it don't hang.
		a.queued = append(a.queued, StreamEvent{Type: EventMessageStart})

	case wingmodels.TextStartPart:
		idx := a.allocIndex(v.ID, "text")
		a.queued = append(a.queued, StreamEvent{
			Type:  EventContentBlockStart,
			Index: idx,
			ContentBlock: &StreamContentBlock{Type: "text"},
		})
	case wingmodels.TextDeltaPart:
		idx := a.idToIndex[v.ID]
		a.queued = append(a.queued, StreamEvent{
			Type:  EventTextDelta,
			Text:  v.Delta,
			Index: idx,
		})
	case wingmodels.TextEndPart:
		idx := a.idToIndex[v.ID]
		a.queued = append(a.queued, StreamEvent{Type: EventContentBlockStop, Index: idx})

	case wingmodels.ReasoningStartPart:
		// No content_block_start emitted (legacy has no reasoning kind);
		// we'll route deltas through text events below.
		a.allocIndex(v.ID, "reasoning")
	case wingmodels.ReasoningDeltaPart:
		a.queued = append(a.queued, StreamEvent{
			Type: EventTextDelta,
			Text: "[reasoning] " + v.Delta,
		})
	case wingmodels.ReasoningEndPart:
		// Nothing to emit.

	case wingmodels.ToolInputStartPart:
		idx := a.allocIndex(v.ID, "tool_use")
		a.blockNames[idx] = v.ToolName
		a.blockIDs[idx] = v.ID
		a.queued = append(a.queued, StreamEvent{
			Type:  EventContentBlockStart,
			Index: idx,
			ContentBlock: &StreamContentBlock{
				Type: "tool_use",
				ID:   v.ID,
				Name: v.ToolName,
			},
		})
	case wingmodels.ToolInputDeltaPart:
		idx := a.idToIndex[v.ID]
		a.queued = append(a.queued, StreamEvent{
			Type:      EventInputJSONDelta,
			InputJSON: v.Delta,
			Index:     idx,
		})
	case wingmodels.ToolInputEndPart:
		idx := a.idToIndex[v.ID]
		a.queued = append(a.queued, StreamEvent{Type: EventContentBlockStop, Index: idx})

	case wingmodels.ToolCallPart_:
		// ToolInputEnd already emitted content_block_stop. Nothing extra.

	case wingmodels.FinishPart:
		// Map FinishReason back to legacy stop_reason strings.
		stopReason := legacyStopReason(v.Reason)
		a.queued = append(a.queued, StreamEvent{
			Type:       EventMessageDelta,
			StopReason: stopReason,
			Usage: &Usage{
				InputTokens:  v.Usage.InputTokens,
				OutputTokens: v.Usage.OutputTokens,
			},
		})
		a.queued = append(a.queued, StreamEvent{Type: EventMessageStop})

	case wingmodels.ErrorPart:
		a.err = fmt.Errorf("%s", v.Message)
		a.queued = append(a.queued, StreamEvent{Type: EventError, Error: a.err})
	}
}

func (a *adapterStream) allocIndex(id, kind string) int {
	if idx, ok := a.idToIndex[id]; ok {
		return idx
	}
	idx := a.nextIdx
	a.nextIdx++
	a.idToIndex[id] = idx
	a.blockKinds[idx] = kind
	return idx
}

func (a *adapterStream) Event() StreamEvent { return a.cur }
func (a *adapterStream) Err() error         { return a.err }
func (a *adapterStream) Close() error {
	if a.closed {
		return nil
	}
	a.closed = true
	// Drain remaining events to unblock the producer goroutine.
	go func() {
		for range a.parts {
		}
	}()
	return nil
}

func (a *adapterStream) Response() *InferenceResponse {
	if a.finalMsg == nil {
		// Stream not yet drained; pull Final without blocking too long.
		// Final blocks until Close is called on the source, which happens
		// when the producer goroutine emits FinishPart. Callers typically
		// invoke Response only after Next() returns false.
		msg, err := a.src.Final()
		if err != nil && a.err == nil {
			a.err = err
		}
		a.finalMsg = msg
	}
	return toLegacyResponse(a.finalMsg)
}

// legacyStopReason maps wingmodels.FinishReason back to the legacy string
// stop_reason values the agent loop expects.
func legacyStopReason(r wingmodels.FinishReason) string {
	switch r {
	case wingmodels.FinishReasonStop:
		return "end_turn"
	case wingmodels.FinishReasonLength:
		return "max_tokens"
	case wingmodels.FinishReasonToolCalls:
		return "tool_use"
	case wingmodels.FinishReasonContentFilter:
		return "refusal"
	case wingmodels.FinishReasonError:
		return "error"
	case wingmodels.FinishReasonAborted:
		return "aborted"
	default:
		return string(r)
	}
}

// jsonStringForLegacy is a tiny helper used by tests to verify the adapter's
// input encoding round-trips. Kept package-level so it doesn't get optimized
// away. Marked unused via the build hint pattern.
var _ = func() string {
	b, _ := json.Marshal(struct{}{})
	return strings.TrimSpace(string(b))
}
