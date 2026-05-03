package wingmodels

import (
	"encoding/json"
	"fmt"
)

// StreamPart is one event in a model's streaming response. The shape mirrors
// Vercel AI SDK v3 LanguageModelV3StreamPart exactly (hyphenated kind names),
// with two wingman additions:
//
//  1. FinishPart carries the assembled *Message, not just usage and reason.
//     This lets consumers grab the final message without rebuilding state.
//  2. The "aborted" finish reason exists on top of AI SDK's enum; see
//     FinishReason in wingmodels.go.
//
// Reference: bb/ai/packages/provider/src/language-model/v3/language-model-v3-stream-part.ts.
//
// Tool flow is three-phase:
//
//	tool-input-start (id, name)
//	tool-input-delta (id, delta) ...
//	tool-input-end   (id)
//	tool-call        (id, name, parsed input)
//
// Then optionally tool-result for provider-executed tools (not used in v0.1
// since wingharness executes tools client-side; the part type is reserved for
// future MCP integration).
//
// Stream lifecycle:
//
//	stream-start (warnings)?
//	(text-* | reasoning-* | tool-input-* | tool-call)*
//	response-metadata?
//	error*
//	finish (usage, reason, message)
//
// Providers MUST emit exactly one FinishPart as the terminator. Errors mid-
// stream are emitted as ErrorPart events; the FinishPart that follows carries
// FinishReasonError or FinishReasonAborted.
type StreamPart interface {
	// Kind returns the part discriminator as it appears on the wire.
	Kind() string
	// streamPart seals the union to this package.
	streamPart()
}

// StreamPart kind discriminators. Stable; on the wire as the "type" field.
const (
	KindStreamStart      = "stream-start"
	KindTextStart        = "text-start"
	KindTextDelta        = "text-delta"
	KindTextEnd          = "text-end"
	KindReasoningStart   = "reasoning-start"
	KindReasoningDelta   = "reasoning-delta"
	KindReasoningEnd     = "reasoning-end"
	KindToolInputStart   = "tool-input-start"
	KindToolInputDelta   = "tool-input-delta"
	KindToolInputEnd     = "tool-input-end"
	KindToolCall         = "tool-call"
	KindToolResult       = "tool-result"
	KindResponseMetadata = "response-metadata"
	KindFinish           = "finish"
	KindError            = "error"
)

// Warning is a non-fatal advisory from the provider (e.g. an unsupported
// option was silently dropped). Mirrors AI SDK v3 SharedV3Warning loosely;
// kept simple in v0.1.
type Warning struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	// Setting names the request setting that triggered the warning, if any.
	Setting string `json:"setting,omitempty"`
}

// StreamStartPart is the optional first event, carrying any provider warnings
// produced while validating the request.
type StreamStartPart struct {
	Warnings []Warning `json:"warnings,omitempty"`
}

func (StreamStartPart) Kind() string { return KindStreamStart }
func (StreamStartPart) streamPart()  {}

// TextStartPart opens a text content block. ID groups subsequent
// TextDeltaParts with this start; the matching TextEndPart closes it.
type TextStartPart struct {
	ID string `json:"id"`
}

func (TextStartPart) Kind() string { return KindTextStart }
func (TextStartPart) streamPart()  {}

// TextDeltaPart appends text to the open block identified by ID.
type TextDeltaPart struct {
	ID    string `json:"id"`
	Delta string `json:"delta"`
}

func (TextDeltaPart) Kind() string { return KindTextDelta }
func (TextDeltaPart) streamPart()  {}

// TextEndPart closes the text block identified by ID.
type TextEndPart struct {
	ID string `json:"id"`
}

func (TextEndPart) Kind() string { return KindTextEnd }
func (TextEndPart) streamPart()  {}

// ReasoningStartPart opens a reasoning block. Same lifecycle as text.
type ReasoningStartPart struct {
	ID string `json:"id"`
}

func (ReasoningStartPart) Kind() string { return KindReasoningStart }
func (ReasoningStartPart) streamPart()  {}

// ReasoningDeltaPart appends reasoning text to the open block.
type ReasoningDeltaPart struct {
	ID    string `json:"id"`
	Delta string `json:"delta"`
}

func (ReasoningDeltaPart) Kind() string { return KindReasoningDelta }
func (ReasoningDeltaPart) streamPart()  {}

// ReasoningEndPart closes the reasoning block.
type ReasoningEndPart struct {
	ID string `json:"id"`
}

func (ReasoningEndPart) Kind() string { return KindReasoningEnd }
func (ReasoningEndPart) streamPart()  {}

// ToolInputStartPart opens a tool-call argument stream. The model has decided
// to call ToolName but has not finished producing arguments yet.
type ToolInputStartPart struct {
	ID       string `json:"id"`
	ToolName string `json:"tool_name"`
}

func (ToolInputStartPart) Kind() string { return KindToolInputStart }
func (ToolInputStartPart) streamPart()  {}

// ToolInputDeltaPart appends raw JSON text to the open tool-input block.
// Providers stream tool arguments as JSON fragments; the client shows partial
// args to the UI but does not parse until ToolInputEndPart.
type ToolInputDeltaPart struct {
	ID    string `json:"id"`
	Delta string `json:"delta"`
}

func (ToolInputDeltaPart) Kind() string { return KindToolInputDelta }
func (ToolInputDeltaPart) streamPart()  {}

// ToolInputEndPart closes the tool-input stream. The next ToolCallPart with
// the same ID carries the parsed arguments.
type ToolInputEndPart struct {
	ID string `json:"id"`
}

func (ToolInputEndPart) Kind() string { return KindToolInputEnd }
func (ToolInputEndPart) streamPart()  {}

// ToolCallPart_ is a finalized tool call: ID matches the prior ToolInput*
// events, ToolName is the registered tool, Input is the parsed arguments.
//
// Named with trailing underscore to avoid collision with the storage
// ToolCallPart in part.go. They carry the same information but live in
// separate type spaces (event vs. message content) so consumers can tell at
// compile time which context they're in.
type ToolCallPart_ struct {
	ID       string         `json:"id"`
	ToolName string         `json:"tool_name"`
	Input    map[string]any `json:"input"`
}

func (ToolCallPart_) Kind() string { return KindToolCall }
func (ToolCallPart_) streamPart()  {}

// ToolResultPart_ is reserved for provider-executed tools (e.g. MCP-server
// tools the provider runs without round-tripping to the agent). Not used in
// v0.1 since wingharness executes tools client-side; reserved so providers can
// emit it later without a wire-format break.
type ToolResultPart_ struct {
	ID       string `json:"id"`
	ToolName string `json:"tool_name"`
	// Result is the raw JSON output from the provider-executed tool. Left
	// untyped because the provider chooses the shape.
	Result  json.RawMessage `json:"result"`
	IsError bool            `json:"is_error,omitempty"`
}

func (ToolResultPart_) Kind() string { return KindToolResult }
func (ToolResultPart_) streamPart()  {}

// ResponseMetadataPart surfaces provider response identifiers as soon as they
// are available, often before the stream finishes. Optional.
type ResponseMetadataPart struct {
	ResponseMetadata
}

func (ResponseMetadataPart) Kind() string { return KindResponseMetadata }
func (ResponseMetadataPart) streamPart()  {}

// FinishPart is the terminal event. Wingman extension over AI SDK v3:
// includes the assembled Message so consumers don't need to reconstruct it.
type FinishPart struct {
	Reason   FinishReason `json:"reason"`
	Usage    Usage        `json:"usage"`
	Message  *Message     `json:"message"`
	Metadata ResponseMetadata `json:"metadata,omitempty"`
}

func (FinishPart) Kind() string { return KindFinish }
func (FinishPart) streamPart()  {}

// ErrorPart conveys a stream error. Multiple may be emitted before the
// terminal FinishPart, matching AI SDK v3 ("error parts are streamed,
// allowing for multiple errors").
type ErrorPart struct {
	// Message is a human-readable error description.
	Message string `json:"message"`
	// Code is an optional machine-readable classification (e.g.
	// "rate_limited", "context_length_exceeded"). Provider-specific.
	Code string `json:"code,omitempty"`
}

func (ErrorPart) Kind() string { return KindError }
func (ErrorPart) streamPart()  {}

// MarshalStreamPart serializes a StreamPart to JSON with a "type"
// discriminator. Same pattern as MarshalPart.
func MarshalStreamPart(p StreamPart) ([]byte, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal stream part body: %w", err)
	}
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("stream part %T did not marshal to a JSON object", p)
	}
	if len(body) == 2 {
		return []byte(fmt.Sprintf(`{"type":%q}`, p.Kind())), nil
	}
	return []byte(fmt.Sprintf(`{"type":%q,%s`, p.Kind(), string(body[1:]))), nil
}

// UnmarshalStreamPart decodes a JSON object with a "type" discriminator into
// the matching concrete StreamPart.
func UnmarshalStreamPart(data []byte) (StreamPart, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("read stream part type: %w", err)
	}
	switch head.Type {
	case KindStreamStart:
		var p StreamStartPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindTextStart:
		var p TextStartPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindTextDelta:
		var p TextDeltaPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindTextEnd:
		var p TextEndPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindReasoningStart:
		var p ReasoningStartPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindReasoningDelta:
		var p ReasoningDeltaPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindReasoningEnd:
		var p ReasoningEndPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindToolInputStart:
		var p ToolInputStartPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindToolInputDelta:
		var p ToolInputDeltaPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindToolInputEnd:
		var p ToolInputEndPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindToolCall:
		var p ToolCallPart_
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindToolResult:
		var p ToolResultPart_
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindResponseMetadata:
		var p ResponseMetadataPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindFinish:
		var p FinishPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	case KindError:
		var p ErrorPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, err
		}
		return p, nil
	default:
		return nil, fmt.Errorf("unknown stream part type %q", head.Type)
	}
}
