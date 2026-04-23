package wingmodels

import (
	"iter"
	"strings"
)

// Snapshot is a point-in-time view of an in-flight assistant message. It is
// recomputed on each event, so consumers that want to render incrementally
// should treat it as immutable per yield.
//
// Pi-mono shipped this as `partial: AssistantMessage` baked into every event.
// We keep the wire format thin (AI SDK v3 shape) and expose the same
// ergonomic via Accumulate so callers opt in by wrapping the stream.
type Snapshot struct {
	// Message is the assembled message so far. Role is RoleAssistant. Content
	// holds completed and in-progress parts in the order they began.
	Message *Message
	// Usage is the running usage if a FinishPart has been seen, zero
	// otherwise. (Most providers only report usage at finish time.)
	Usage Usage
	// Reason is set only after a FinishPart is observed.
	Reason FinishReason
}

// Accumulate wraps an EventStream and yields (Snapshot, StreamPart) pairs.
// The Snapshot reflects the state after applying the StreamPart.
//
// Snapshot.Message.Content layout:
//   - text-* events build a TextPart whose Text grows with each delta.
//   - reasoning-* events build a ReasoningPart whose Reasoning grows.
//   - tool-input-* + tool-call build a ToolCallPart. The ToolCallPart appears
//     in Content at tool-input-start time with empty Input; Input is filled
//     in when the tool-call event arrives.
//   - id-keyed: the id on each part event is mapped to a Content index
//     internally, allowing parallel content blocks (some providers stream
//     multiple text blocks interleaved with tool calls).
//
// Order in Content matches the order of the *-start events. This is the
// natural reading order for UIs.
//
// Single-consumer: Accumulate ranges over stream.Iter, so only one Accumulate
// per stream.
func Accumulate(stream *EventStream[StreamPart, *Message]) iter.Seq2[Snapshot, StreamPart] {
	return func(yield func(Snapshot, StreamPart) bool) {
		// idIndex maps a stream-part id to the index in msg.Content where its
		// part lives. Parts are appended in start-event order, so different
		// ids get different indices.
		idIndex := make(map[string]int)
		// builders track in-progress text/reasoning content so we can
		// efficiently append deltas without re-allocating the Part struct on
		// every event.
		textBuilders := make(map[string]*strings.Builder)
		reasoningBuilders := make(map[string]*strings.Builder)

		msg := &Message{Role: RoleAssistant, Content: Content{}}
		snap := Snapshot{Message: msg}

		// flush rebuilds the Content slice from builders so consumers see
		// up-to-date Text/Reasoning. We do this once per yield rather than on
		// every Push to avoid quadratic concatenation.
		flush := func() {
			for id, b := range textBuilders {
				if i, ok := idIndex[id]; ok {
					if tp, ok2 := msg.Content[i].(TextPart); ok2 {
						tp.Text = b.String()
						msg.Content[i] = tp
					}
				}
			}
			for id, b := range reasoningBuilders {
				if i, ok := idIndex[id]; ok {
					if rp, ok2 := msg.Content[i].(ReasoningPart); ok2 {
						rp.Reasoning = b.String()
						msg.Content[i] = rp
					}
				}
			}
		}

		for ev := range stream.Iter() {
			switch p := ev.(type) {
			case TextStartPart:
				idIndex[p.ID] = len(msg.Content)
				msg.Content = append(msg.Content, TextPart{})
				textBuilders[p.ID] = &strings.Builder{}
			case TextDeltaPart:
				if b, ok := textBuilders[p.ID]; ok {
					b.WriteString(p.Delta)
				}
			case TextEndPart:
				// nothing to do; flush below picks up final text
			case ReasoningStartPart:
				idIndex[p.ID] = len(msg.Content)
				msg.Content = append(msg.Content, ReasoningPart{})
				reasoningBuilders[p.ID] = &strings.Builder{}
			case ReasoningDeltaPart:
				if b, ok := reasoningBuilders[p.ID]; ok {
					b.WriteString(p.Delta)
				}
			case ReasoningEndPart:
				// flush below
			case ToolInputStartPart:
				idIndex[p.ID] = len(msg.Content)
				msg.Content = append(msg.Content, ToolCallPart{
					CallID: p.ID,
					Name:   p.ToolName,
					Input:  nil,
				})
			case ToolInputDeltaPart, ToolInputEndPart:
				// tool input arg streaming is informational only at the
				// snapshot level; the parsed input arrives via ToolCallPart_.
			case ToolCallPart_:
				if i, ok := idIndex[p.ID]; ok {
					if tc, ok2 := msg.Content[i].(ToolCallPart); ok2 {
						tc.Input = p.Input
						// some providers send tool-call without a prior
						// tool-input-start; trust the call-event ToolName.
						if tc.Name == "" {
							tc.Name = p.ToolName
						}
						msg.Content[i] = tc
					}
				} else {
					idIndex[p.ID] = len(msg.Content)
					msg.Content = append(msg.Content, ToolCallPart{
						CallID: p.ID,
						Name:   p.ToolName,
						Input:  p.Input,
					})
				}
			case ResponseMetadataPart:
				if msg.Metadata == nil {
					msg.Metadata = Meta{}
				}
				if p.ID != "" {
					msg.Metadata["response_id"] = p.ID
				}
				if p.ModelID != "" {
					msg.Metadata["model_id"] = p.ModelID
				}
				if p.Timestamp != "" {
					msg.Metadata["timestamp"] = p.Timestamp
				}
			case FinishPart:
				snap.Usage = p.Usage
				snap.Reason = p.Reason
				// FinishPart.Message is authoritative if the provider built
				// one; prefer it over our accumulated view.
				if p.Message != nil {
					msg = p.Message
					snap.Message = msg
				}
			case ErrorPart, StreamStartPart, ToolResultPart_:
				// no snapshot mutation
			}

			flush()
			if !yield(snap, ev) {
				return
			}
		}
	}
}
