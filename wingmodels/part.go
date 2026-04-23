package wingmodels

import (
	"encoding/json"
	"fmt"
)

// Part is one element of a Message's Content. It is a discriminated union;
// concrete implementations are TextPart, ReasoningPart, ImagePart, ToolCallPart,
// and ToolResultPart. The Type() method returns the discriminator string used
// in JSON serialization (see MarshalPart / UnmarshalPart).
//
// Naming: "reasoning" matches Vercel AI SDK v3 (bb/ai/packages/provider/src/
// language-model/v3/language-model-v3-reasoning.ts) and generalizes across
// Anthropic extended thinking, OpenAI o1/o3 reasoning, and DeepSeek R1
// reasoning. It is the same concept pi-mono calls "thinking".
type Part interface {
	// Type returns the part discriminator (text, reasoning, image, tool_call,
	// tool_result). Stable on the wire and in storage.
	Type() string
	// isPart is an unexported marker keeping the union sealed to this package.
	isPart()
}

// Part type discriminators. Stable; persisted in storage and on the SSE wire.
const (
	PartTypeText       = "text"
	PartTypeReasoning  = "reasoning"
	PartTypeImage      = "image"
	PartTypeToolCall   = "tool_call"
	PartTypeToolResult = "tool_result"
)

// TextPart carries plain assistant or user text.
type TextPart struct {
	// Text is the textual content. May be empty for an in-progress streaming
	// part that has not yet emitted a delta.
	Text string `json:"text"`
	// Signature is provider-opaque metadata required to round-trip the part
	// on subsequent turns (e.g. OpenAI Responses item id). Empty if unused.
	Signature string `json:"signature,omitempty"`
}

func (TextPart) Type() string { return PartTypeText }
func (TextPart) isPart()      {}

// ReasoningPart carries reasoning / chain-of-thought content. Some providers
// emit these only when explicitly enabled (e.g. Anthropic extended thinking).
//
// Pi-mono and Anthropic call this "thinking"; AI SDK v3 calls it "reasoning".
// We adopt the AI SDK name as more provider-neutral.
type ReasoningPart struct {
	// Reasoning is the visible reasoning text. Empty if Redacted is true.
	Reasoning string `json:"reasoning"`
	// Signature is provider-opaque metadata required to replay the reasoning
	// on subsequent turns: Anthropic redacted-thinking encrypted payload,
	// OpenAI reasoning item id, etc.
	Signature string `json:"signature,omitempty"`
	// Redacted is true when the upstream API redacted the reasoning content
	// for safety. The opaque encrypted payload (if any) lives in Signature.
	Redacted bool `json:"redacted,omitempty"`
}

func (ReasoningPart) Type() string { return PartTypeReasoning }
func (ReasoningPart) isPart()      {}

// ImagePart carries inline image data. Used for both user-supplied images and
// tool results that include screenshots.
type ImagePart struct {
	// Data is base64-encoded image bytes.
	Data string `json:"data"`
	// MimeType is e.g. "image/png", "image/jpeg".
	MimeType string `json:"mime_type"`
}

func (ImagePart) Type() string { return PartTypeImage }
func (ImagePart) isPart()      {}

// ToolCallPart is a model-emitted request to invoke a tool. Input is the
// JSON-decoded arguments object the model produced.
type ToolCallPart struct {
	// CallID uniquely identifies this invocation. ToolResultPart.CallID must
	// match exactly to be paired with the call.
	CallID string `json:"call_id"`
	// Name is the tool name as registered with the agent.
	Name string `json:"name"`
	// Input is the parsed JSON arguments object. Nil-valued keys are
	// preserved; missing keys are absent.
	Input map[string]any `json:"input"`
	// Signature is provider-opaque metadata required to replay the call
	// (e.g. Google thoughtSignature). Empty if unused.
	Signature string `json:"signature,omitempty"`
}

func (ToolCallPart) Type() string { return PartTypeToolCall }
func (ToolCallPart) isPart()      {}

// ToolResultPart is the outcome of executing a ToolCallPart. The agent loop
// wraps it in a Message{Role: RoleTool} when appending to history.
type ToolResultPart struct {
	// CallID matches the ToolCallPart that produced this result.
	CallID string `json:"call_id"`
	// Output carries the tool's output. Multiple parts allow returning text +
	// images (e.g. a browser tool returning markdown plus a screenshot).
	Output []Part `json:"output"`
	// IsError indicates the tool execution failed. Output typically contains
	// an error message in this case.
	IsError bool `json:"is_error,omitempty"`
}

func (ToolResultPart) Type() string { return PartTypeToolResult }
func (ToolResultPart) isPart()      {}

// MarshalJSON for ToolResultPart hand-rolls the Output array so each child
// Part gets its discriminator tag. Default struct marshaling would invoke
// json.Marshal on each Part interface value, which produces an untagged
// concrete-type encoding.
func (t ToolResultPart) MarshalJSON() ([]byte, error) {
	out := make([]json.RawMessage, len(t.Output))
	for i, p := range t.Output {
		raw, err := MarshalPart(p)
		if err != nil {
			return nil, fmt.Errorf("tool_result output[%d]: %w", i, err)
		}
		out[i] = raw
	}
	type alias struct {
		CallID  string            `json:"call_id"`
		Output  []json.RawMessage `json:"output"`
		IsError bool              `json:"is_error,omitempty"`
	}
	return json.Marshal(alias{CallID: t.CallID, Output: out, IsError: t.IsError})
}

// MarshalPart serializes a Part to JSON with a "type" discriminator field.
//
// Why a free function and not Part.MarshalJSON: the sealed-interface pattern
// means concrete types live in this package, and Go cannot dispatch
// MarshalJSON through an interface variable to inject the type tag without
// each concrete type duplicating the logic. Centralizing here keeps the
// envelope shape consistent.
func MarshalPart(p Part) ([]byte, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal part body: %w", err)
	}
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("part %T did not marshal to a JSON object", p)
	}
	// Splice "type" as the first field. We avoid an intermediate map decode
	// to keep field order deterministic and skip a redundant marshal pass.
	if len(body) == 2 { // "{}"
		return []byte(fmt.Sprintf(`{"type":%q}`, p.Type())), nil
	}
	return []byte(fmt.Sprintf(`{"type":%q,%s`, p.Type(), string(body[1:]))), nil
}

// UnmarshalPart decodes a JSON object with a "type" discriminator into the
// matching concrete Part. Unknown discriminators yield an error rather than
// silently dropping data.
func UnmarshalPart(data []byte) (Part, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("read part type: %w", err)
	}
	switch head.Type {
	case PartTypeText:
		var p TextPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode text part: %w", err)
		}
		return p, nil
	case PartTypeReasoning:
		var p ReasoningPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode reasoning part: %w", err)
		}
		return p, nil
	case PartTypeImage:
		var p ImagePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode image part: %w", err)
		}
		return p, nil
	case PartTypeToolCall:
		var p ToolCallPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode tool_call part: %w", err)
		}
		return p, nil
	case PartTypeToolResult:
		// ToolResultPart contains nested Parts; decode in two phases so each
		// child goes through this same dispatcher.
		var raw struct {
			CallID  string            `json:"call_id"`
			Output  []json.RawMessage `json:"output"`
			IsError bool              `json:"is_error,omitempty"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("decode tool_result part: %w", err)
		}
		out := make([]Part, 0, len(raw.Output))
		for i, item := range raw.Output {
			child, err := UnmarshalPart(item)
			if err != nil {
				return nil, fmt.Errorf("decode tool_result output[%d]: %w", i, err)
			}
			out = append(out, child)
		}
		return ToolResultPart{CallID: raw.CallID, Output: out, IsError: raw.IsError}, nil
	default:
		return nil, fmt.Errorf("unknown part type %q", head.Type)
	}
}
