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
	// ProviderOptions carries per-provider, request-scoped settings keyed by
	// provider id (e.g. "anthropic", "openai"). Providers MUST ignore unknown
	// top-level keys; this preserves forward compatibility when a request
	// crafted for one provider is later replayed against another.
	//
	// Example:
	//
	//   ProviderOptions: ProviderOptions{
	//       "anthropic": {"thinking": map[string]any{"budget_tokens": 1024}},
	//       "openai":    {"reasoning_effort": "high"},
	//   }
	ProviderOptions ProviderOptions
}

// ProviderOptions is a two-level namespaced bag of provider-specific options.
// The outer key is the provider id; the inner map is opaque to wingmodels and
// interpreted by each provider as it sees fit. Mirrors the AI SDK v3 shape
// (bb/ai/packages/provider/src/language-model/v3/language-model-v3-options.ts).
type ProviderOptions map[string]map[string]any

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
	// API identifies the wire-format family this model speaks (e.g.
	// APIAnthropicMessages, APIOpenAICompletions). Used by the transform
	// layer to detect when an assistant message produced by one model can be
	// replayed verbatim against another. Empty for legacy providers.
	API API
	// BaseURL is the HTTPS endpoint the provider was configured with. Empty
	// if the provider uses a hard-coded default. Surfaced for diagnostics
	// and for provider-aggregator routing.
	BaseURL string
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
	// InputCostPerMTok is USD per 1M input tokens. Zero if unknown.
	InputCostPerMTok float64
	// OutputCostPerMTok is USD per 1M output tokens. Zero if unknown.
	OutputCostPerMTok float64
	// CacheReadCostPerMTok is USD per 1M cached input tokens read. Zero if
	// the provider has no prompt cache or cost is unknown.
	CacheReadCostPerMTok float64
	// CacheWriteCostPerMTok is USD per 1M tokens written to the prompt cache.
	// Anthropic-specific in practice.
	CacheWriteCostPerMTok float64
	// Compat is an opaque, API-family-specific quirk descriptor used by the
	// wire-format client to shape requests and parse responses for this
	// particular service. Discriminated by the API field; consumers cast to
	// the matching concrete type (e.g. *openaicompletions.Compat). Nil if
	// the API client's defaults suffice.
	Compat any
}

// API is a wire-format family identifier. Multiple providers can share an
// API (e.g. OpenAI, DeepSeek, Groq, OpenRouter all speak APIOpenAICompletions)
// which lets them share one wire-format client and differ only in Compat
// settings.
type API string

const (
	APIOpenAICompletions API = "openai-completions"
	APIOpenAIResponses   API = "openai-responses"
	APIAnthropicMessages API = "anthropic-messages"
	APIGoogleGenAI       API = "google-genai"
	APIBedrockConverse   API = "bedrock-converse"
)

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
