package models

import (
	"encoding/json"
	"fmt"
)

// Message is one entry in conversation history. Content is a list of Parts
// representing the message's payload (text, reasoning, images, tool calls, or
// tool results, in any combination).
//
// The shape mirrors AI SDK v3 LanguageModelV3Prompt entries but uses our
// canonical Part union (see part.go) rather than v3's separate per-role
// content types. One union, one storage shape, one wire shape.
type Message struct {
	Role     Role    `json:"role"`
	Content  Content `json:"content"`
	Metadata Meta    `json:"metadata,omitempty"`
	// Origin records the provider/API/model that produced this message.
	// Populated by providers on assistant messages emitted via FinishPart;
	// nil for user/tool messages and for assistant messages loaded from
	// older sessions. The transform layer uses Origin to detect same-model
	// replays and skip lossy normalizations (reasoning blocks, tool-call
	// IDs) when the next request targets the same provider+API+model.
	Origin *MessageOrigin `json:"origin,omitempty"`
	// FinishReason records why an assistant turn stopped. Populated by the
	// accumulator from the terminal FinishPart; empty for user/tool messages
	// and for assistant messages loaded from older sessions. The transform
	// layer uses this to drop turns that ended in error/aborted state — their
	// content is typically half-streamed (empty text blocks, tool calls with
	// no input) and providers reject it. In-memory only in v0.1; not yet
	// persisted to storage (parallel to Origin).
	FinishReason FinishReason `json:"finish_reason,omitempty"`
}

// MessageOrigin identifies the model that produced an assistant message.
// All three fields are required when set; partial origins are not allowed.
type MessageOrigin struct {
	Provider string `json:"provider"`
	API      API    `json:"api"`
	ModelID  string `json:"model_id"`
}

// SameModel reports whether two origins refer to the same provider+API+model.
// Nil-safe: a nil receiver or argument returns false (cannot prove same).
func (o *MessageOrigin) SameModel(other *MessageOrigin) bool {
	if o == nil || other == nil {
		return false
	}
	return o.Provider == other.Provider && o.API == other.API && o.ModelID == other.ModelID
}

// Meta carries provider-agnostic per-message metadata. Kept open-ended so
// providers can stash response ids, request ids, timing data, etc.
type Meta map[string]any

// Content is a slice of Parts with custom JSON marshaling so each element
// gets the correct discriminator tag (see part.go).
//
// Why a named type and not []Part: encoding/json cannot marshal an interface
// slice via a top-level helper without hand-rolling MarshalJSON on the
// container.
type Content []Part

// MarshalJSON encodes the slice as a JSON array of tagged Part objects.
func (c Content) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	parts := make([]json.RawMessage, len(c))
	for i, p := range c {
		raw, err := MarshalPart(p)
		if err != nil {
			return nil, fmt.Errorf("content[%d]: %w", i, err)
		}
		parts[i] = raw
	}
	return json.Marshal(parts)
}

// UnmarshalJSON decodes a JSON array of tagged Part objects into the slice.
func (c *Content) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*c = nil
		return nil
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("decode content array: %w", err)
	}
	out := make(Content, 0, len(raws))
	for i, raw := range raws {
		p, err := UnmarshalPart(raw)
		if err != nil {
			return fmt.Errorf("content[%d]: %w", i, err)
		}
		out = append(out, p)
	}
	*c = out
	return nil
}

// Constructors for the common cases. Use these instead of struct literals
// to avoid forgetting Role.

// NewUserText builds a user message containing a single TextPart.
func NewUserText(text string) Message {
	return Message{Role: RoleUser, Content: Content{TextPart{Text: text}}}
}

// NewAssistantText builds an assistant message containing a single TextPart.
// Useful in tests; real assistant messages come from the model.
func NewAssistantText(text string) Message {
	return Message{Role: RoleAssistant, Content: Content{TextPart{Text: text}}}
}

// NewToolResult builds a RoleTool message wrapping a ToolResultPart.
func NewToolResult(callID string, output []Part, isError bool) Message {
	return Message{
		Role: RoleTool,
		Content: Content{
			ToolResultPart{CallID: callID, Output: output, IsError: isError},
		},
	}
}

// Usage tracks token consumption for a single model turn.
//
// Field names match Vercel AI SDK v3 LanguageModelV3Usage
// (bb/ai/packages/provider/src/language-model/v3/language-model-v3-usage.ts):
// inputTokens, outputTokens, totalTokens, reasoningTokens, cachedInputTokens.
// Wire JSON keys are snake_case to match wingman convention.
type Usage struct {
	InputTokens       int `json:"input_tokens"`
	OutputTokens      int `json:"output_tokens"`
	TotalTokens       int `json:"total_tokens"`
	ReasoningTokens   int `json:"reasoning_tokens,omitempty"`
	CachedInputTokens int `json:"cached_input_tokens,omitempty"`
	// CacheWriteTokens is Anthropic-specific (prompt cache write cost). Kept
	// optional rather than provider-bucketed metadata since it surfaces in
	// pricing calculations frequently enough to warrant a typed field.
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// ResponseMetadata captures per-response identifiers from the provider.
// Mirrors AI SDK v3 LanguageModelV3ResponseMetadata.
type ResponseMetadata struct {
	// ID is the provider's response identifier (Anthropic message id, OpenAI
	// chat.completion id, etc.). Empty if the provider didn't supply one.
	ID string `json:"id,omitempty"`
	// ModelID is the resolved model identifier the provider actually used.
	// May differ from the requested ID (e.g. provider-side aliases).
	ModelID string `json:"model_id,omitempty"`
	// Timestamp is the provider-reported response time, RFC3339. Empty if
	// not supplied.
	Timestamp string `json:"timestamp,omitempty"`
}
