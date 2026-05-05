package models

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Part is one element of a Message's Content. It is a discriminated union;
// concrete implementations supplied by this package are TextPart,
// ReasoningPart, ImagePart, ToolCallPart, and ToolResultPart. Plugins may
// add their own Part types via RegisterPart.
//
// The Type() method returns the discriminator string used in JSON
// serialization (see MarshalPart / UnmarshalPart).
//
// Naming: "reasoning" matches Vercel AI SDK v3 (bb/ai/packages/provider/src/
// language-model/v3/language-model-v3-reasoning.ts) and generalizes across
// Anthropic extended thinking, OpenAI o1/o3, and DeepSeek R1. It is the
// same concept pi-mono calls "thinking".
type Part interface {
	// Type returns the part discriminator (text, reasoning, image,
	// tool_call, tool_result, or any plugin-registered name). Stable
	// on the wire and in store.
	Type() string
	// isPart is an unexported marker keeping the *core* union sealed
	// to this package. Plugin parts compose by embedding a struct that
	// satisfies isPart via OpaquePart-like patterns (see RegisterPart).
	isPart()
}

// Part type discriminators for the core types. Stable; persisted in
// storage and on the SSE wire. Plugin part types use their own constants
// inside their own packages.
const (
	PartTypeText       = "text"
	PartTypeReasoning  = "reasoning"
	PartTypeImage      = "image"
	PartTypeToolCall   = "tool_call"
	PartTypeToolResult = "tool_result"
)

// TextPart carries plain assistant or user text.
type TextPart struct {
	Text            string          `json:"text"`
	Signature       string          `json:"signature,omitempty"`
	ProviderOptions ProviderOptions `json:"provider_options,omitempty"`
}

func (TextPart) Type() string { return PartTypeText }
func (TextPart) isPart()      {}

// ReasoningPart carries reasoning / chain-of-thought content. Some providers
// emit these only when explicitly enabled (e.g. Anthropic extended thinking).
type ReasoningPart struct {
	Reasoning       string          `json:"reasoning"`
	Signature       string          `json:"signature,omitempty"`
	Redacted        bool            `json:"redacted,omitempty"`
	ProviderOptions ProviderOptions `json:"provider_options,omitempty"`
}

func (ReasoningPart) Type() string { return PartTypeReasoning }
func (ReasoningPart) isPart()      {}

// ImagePart carries inline image data.
type ImagePart struct {
	Data            string          `json:"data"`
	MimeType        string          `json:"mime_type"`
	ProviderOptions ProviderOptions `json:"provider_options,omitempty"`
}

func (ImagePart) Type() string { return PartTypeImage }
func (ImagePart) isPart()      {}

// ToolCallPart is a model-emitted request to invoke a tool.
type ToolCallPart struct {
	CallID          string          `json:"call_id"`
	Name            string          `json:"name"`
	Input           map[string]any  `json:"input"`
	Signature       string          `json:"signature,omitempty"`
	ProviderOptions ProviderOptions `json:"provider_options,omitempty"`
}

func (ToolCallPart) Type() string { return PartTypeToolCall }
func (ToolCallPart) isPart()      {}

// ToolResultPart is the outcome of executing a ToolCallPart.
type ToolResultPart struct {
	CallID          string          `json:"call_id"`
	Output          []Part          `json:"output"`
	IsError         bool            `json:"is_error,omitempty"`
	ProviderOptions ProviderOptions `json:"provider_options,omitempty"`
}

func (ToolResultPart) Type() string { return PartTypeToolResult }
func (ToolResultPart) isPart()      {}

// OpaquePart preserves an unknown part type's payload through a
// storage round-trip without losing data. UnmarshalPart returns an
// OpaquePart for any discriminator that has not been registered via
// RegisterPart; MarshalPart re-emits the original JSON object verbatim.
//
// This means a session created when plugin X was installed can still
// be read after plugin X is removed: its custom parts come back as
// opaque values rather than failing the load. UIs that don't recognize
// the type can render a placeholder or skip it.
type OpaquePart struct {
	// TypeName is the discriminator string the part was stored with.
	TypeName string
	// Raw is the original JSON object including the "type" field.
	Raw json.RawMessage
}

func (o OpaquePart) Type() string { return o.TypeName }
func (OpaquePart) isPart()        {}

// MarshalJSON for OpaquePart returns the stored raw bytes so the
// round-trip is exact.
func (o OpaquePart) MarshalJSON() ([]byte, error) {
	if len(o.Raw) == 0 {
		return []byte(fmt.Sprintf(`{"type":%q}`, o.TypeName)), nil
	}
	return o.Raw, nil
}

// PartUnmarshaler decodes a JSON object into a concrete Part. The input
// includes the "type" discriminator field; implementations typically
// json.Unmarshal into a typed struct that ignores the discriminator.
type PartUnmarshaler func(data []byte) (Part, error)

var (
	partRegistryMu sync.RWMutex
	partRegistry   = map[string]PartUnmarshaler{}
)

// RegisterPart adds a Part type discriminator and its decoder to the
// global registry. Safe to call from init(); plugin packages typically
// register their part types this way so loaded sessions decode them
// correctly.
//
// Re-registering an existing name overwrites the previous decoder. This
// lets a plugin override a built-in if it really wants to (use sparingly).
func RegisterPart(typeName string, fn PartUnmarshaler) {
	if typeName == "" {
		panic("models.RegisterPart: empty type name")
	}
	if fn == nil {
		panic("models.RegisterPart: nil unmarshaler")
	}
	partRegistryMu.Lock()
	partRegistry[typeName] = fn
	partRegistryMu.Unlock()
}

// lookupPartUnmarshaler returns the registered decoder for a type, or
// nil if none is registered.
func lookupPartUnmarshaler(typeName string) PartUnmarshaler {
	partRegistryMu.RLock()
	fn := partRegistry[typeName]
	partRegistryMu.RUnlock()
	return fn
}

// init registers the core part types. Plugin packages register their
// own types from their own init() functions.
func init() {
	RegisterPart(PartTypeText, func(data []byte) (Part, error) {
		var p TextPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode text part: %w", err)
		}
		return p, nil
	})
	RegisterPart(PartTypeReasoning, func(data []byte) (Part, error) {
		var p ReasoningPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode reasoning part: %w", err)
		}
		return p, nil
	})
	RegisterPart(PartTypeImage, func(data []byte) (Part, error) {
		var p ImagePart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode image part: %w", err)
		}
		return p, nil
	})
	RegisterPart(PartTypeToolCall, func(data []byte) (Part, error) {
		var p ToolCallPart
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("decode tool_call part: %w", err)
		}
		return p, nil
	})
	RegisterPart(PartTypeToolResult, func(data []byte) (Part, error) {
		// ToolResultPart contains nested Parts; decode in two phases so
		// each child goes through the registry dispatcher.
		var raw struct {
			CallID          string            `json:"call_id"`
			Output          []json.RawMessage `json:"output"`
			IsError         bool              `json:"is_error,omitempty"`
			ProviderOptions ProviderOptions   `json:"provider_options,omitempty"`
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
		return ToolResultPart{
			CallID:          raw.CallID,
			Output:          out,
			IsError:         raw.IsError,
			ProviderOptions: raw.ProviderOptions,
		}, nil
	})
}

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
		CallID          string            `json:"call_id"`
		Output          []json.RawMessage `json:"output"`
		IsError         bool              `json:"is_error,omitempty"`
		ProviderOptions ProviderOptions   `json:"provider_options,omitempty"`
	}
	return json.Marshal(alias{
		CallID:          t.CallID,
		Output:          out,
		IsError:         t.IsError,
		ProviderOptions: t.ProviderOptions,
	})
}

// MarshalPart serializes a Part to JSON with a "type" discriminator field.
//
// OpaquePart short-circuits to its stored raw bytes so unknown plugin
// parts round-trip exactly even when the plugin isn't loaded.
//
// Other types: marshal the body, then splice "type" as the first field.
// We avoid an intermediate map decode to keep field order deterministic
// and skip a redundant marshal pass.
func MarshalPart(p Part) ([]byte, error) {
	if op, ok := p.(OpaquePart); ok {
		return op.MarshalJSON()
	}
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal part body: %w", err)
	}
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("part %T did not marshal to a JSON object", p)
	}
	if len(body) == 2 { // "{}"
		return []byte(fmt.Sprintf(`{"type":%q}`, p.Type())), nil
	}
	return []byte(fmt.Sprintf(`{"type":%q,%s`, p.Type(), string(body[1:]))), nil
}

// UnmarshalPart decodes a JSON object with a "type" discriminator into the
// matching concrete Part. Unknown discriminators yield an OpaquePart
// preserving the original payload, so a session that includes plugin
// part types still loads when the plugin is uninstalled.
func UnmarshalPart(data []byte) (Part, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, fmt.Errorf("read part type: %w", err)
	}
	if head.Type == "" {
		return nil, fmt.Errorf("part missing type discriminator")
	}
	if fn := lookupPartUnmarshaler(head.Type); fn != nil {
		return fn(data)
	}
	// Fallback: keep the bytes so a re-marshal yields the original
	// payload. UIs may render this as "[unknown part: <type>]" or
	// drop it; storage round-trips it losslessly.
	raw := make(json.RawMessage, len(data))
	copy(raw, data)
	return OpaquePart{TypeName: head.Type, Raw: raw}, nil
}
