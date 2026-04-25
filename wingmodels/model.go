package wingmodels

import (
	"context"
	"fmt"
)

// Model is the provider abstraction. One concrete Model represents one
// provider/model pairing (e.g. Anthropic claude-sonnet-4, Ollama llama3.1).
//
// The contract:
//
//   - Stream begins a streaming request. Returns (nil, error) only for
//     synchronous setup failures (auth missing, request validation, network
//     refused before first byte). Once the stream begins, all failures are
//     emitted as ErrorPart events followed by a terminal FinishPart with
//     FinishReasonError or FinishReasonAborted.
//
//   - Info returns static metadata about this Model (provider id, model id,
//     context window, capabilities). Pulled from catalog at construction time
//     in most cases.
//
//   - CountTokens returns an exact (Anthropic /v1/messages/count_tokens) or
//     approximate (Ollama char-based heuristic) input-token count for the
//     given messages. Used by the agent loop to decide compaction. Providers
//     MUST document which kind they return.
type Model interface {
	Info() ModelInfo
	Stream(ctx context.Context, req Request) (*EventStream[StreamPart, *Message], error)
	CountTokens(ctx context.Context, msgs []Message) (int, error)
}

// Request is the input to a Model.Stream call. Fields are kept minimal in
// v0.1; structured outputs, sampling controls (temperature, top_p), tool
// choice, response format, and provider-specific options are deferred.
type Request struct {
	// System is the system prompt. Empty if not used.
	System string
	// Messages is the conversation history. May not be empty.
	Messages []Message
	// Tools are available for the model to call. Schema only; execution lives
	// in wingagent. Empty if no tools are offered.
	Tools []ToolDef
	// MaxOutputTokens caps the response. Zero means provider default.
	MaxOutputTokens int
}

// ToolDef is a tool advertised to the model: name, description, JSON Schema
// for arguments. Execution is the agent layer's responsibility (see
// wingagent.Tool).
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ModelInfo is static metadata about a Model. Most fields come from the
// catalog (wingmodels/catalog) but providers can override after probing the
// live model (e.g. Ollama /api/show).
type ModelInfo struct {
	// Provider is the catalog provider id (e.g. "anthropic", "ollama").
	Provider string
	// ID is the model id within the provider (e.g. "claude-sonnet-4-20250514").
	ID string
	// ContextWindow is the total in+out token budget the model accepts.
	ContextWindow int
	// MaxOutput is the model's hard cap on output tokens. Zero if unknown.
	MaxOutput int
	// SupportsTools is true if the model handles tool calls natively.
	SupportsTools bool
	// SupportsImages is true if the model accepts ImagePart inputs.
	SupportsImages bool
	// SupportsReasoning is true if the model emits ReasoningPart content
	// (Anthropic extended thinking, OpenAI o1/o3, DeepSeek R1).
	SupportsReasoning bool
}

// Run drives a Model.Stream call to completion synchronously and returns the
// final assembled message. It is the trivial sync-mode helper: range the
// stream to drain it, then read Final.
//
// Errors come from two sources:
//   - Stream setup failure: returned directly with no message.
//   - In-stream error: returned as an error wrapping the FinishPart's reason,
//     with the assembled (partial) message.
//
// Callers that need to observe individual stream parts should use Stream
// directly and Accumulate for snapshot ergonomics.
func Run(ctx context.Context, m Model, req Request) (*Message, error) {
	stream, err := m.Stream(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("stream setup: %w", err)
	}
	// Drain. We don't inspect events; FinishPart carries the assembled
	// message and the result slot mirrors it.
	for range stream.Iter() {
	}
	return stream.Final()
}
