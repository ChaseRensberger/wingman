package models

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

// Request is the input to a Model.Stream call.
type Request struct {
	// System is the system prompt. Empty if not used.
	System string
	// Messages is the conversation history. May not be empty.
	Messages []Message
	// Tools are available for the model to call. Schema only; execution lives
	// in agent. Empty if no tools are offered.
	Tools []ToolDef
	// MaxOutputTokens caps the response. Zero means provider default.
	MaxOutputTokens int
	// ToolChoice controls how the model selects tools. Zero value (empty Mode)
	// is treated as ToolChoiceAuto by every provider.
	ToolChoice ToolChoice
	// Capabilities are cross-provider knobs the caller can enable.
	// Providers silently ignore anything they don't support.
	Capabilities Capabilities
	// OutputSchema, when non-nil, instructs the provider to constrain the
	// model's response to the given JSON schema. Providers that do not
	// support native structured output silently ignore this field; callers
	// that need a guarantee should consult ModelInfo.Capabilities.StructuredOutput.
	//
	// When OutputSchema is set, the assistant's text content is guaranteed
	// (by the provider) to be a single JSON document conforming to the
	// schema. Tool calls and structured outputs may not be supported
	// simultaneously by every provider; callers requiring both should
	// reserve OutputSchema for the final no-tool turn.
	OutputSchema *OutputSchema
}

// OutputSchema describes a JSON schema the model's response must conform to.
// Maps onto OpenAI Responses text.format=json_schema, OpenAI Chat Completions
// response_format=json_schema, Anthropic output_config.format=json_schema, and
// Ollama format=<schema>.
type OutputSchema struct {
	// Name identifies the schema. Required by OpenAI; Anthropic and Ollama
	// ignore it. Use a short snake_case identifier.
	Name string
	// Schema is the raw JSON Schema document. Must be a JSON object whose
	// top-level "type" is "object" for maximum cross-provider compatibility.
	Schema map[string]any
	// Strict requests strict schema validation on providers that support a
	// strict mode (OpenAI). Anthropic and Ollama always validate strictly
	// and ignore this field.
	Strict bool
}

// ToolChoiceMode selects the model's tool-use behaviour.
type ToolChoiceMode string

const (
	// ToolChoiceAuto lets the model decide whether to call a tool. This is
	// the default when ToolChoice.Mode is empty.
	ToolChoiceAuto ToolChoiceMode = "auto"
	// ToolChoiceRequired forces the model to call at least one tool.
	ToolChoiceRequired ToolChoiceMode = "required"
	// ToolChoiceNone prevents the model from calling any tool even if Tools
	// is non-empty.
	ToolChoiceNone ToolChoiceMode = "none"
	// ToolChoiceTool forces the model to call a specific named tool.
	// Set ToolChoice.Tool to the target tool name.
	ToolChoiceTool ToolChoiceMode = "tool"
)

// ToolChoice specifies how the model should use tools on a given request.
// The zero value is equivalent to ToolChoiceAuto.
type ToolChoice struct {
	// Mode is the selection strategy. Empty string is treated as ToolChoiceAuto.
	Mode ToolChoiceMode
	// Tool is the name of the tool to force-call. Only meaningful when
	// Mode == ToolChoiceTool.
	Tool string
}

// Capabilities are cross-provider request-level knobs. Each provider reads
// only the fields it supports and silently ignores the rest, so a Request
// targeting Ollama can carry Thinking config without error — it simply has no
// effect.
type Capabilities struct {
	// Thinking activates extended chain-of-thought / reasoning output.
	// Providers that support it (Anthropic claude-3.x via budget_tokens,
	// Anthropic claude-4+ via adaptive effort) consume it. Others ignore it.
	// Nil means thinking is off.
	Thinking *ThinkingConfig
}

// ThinkingConfig controls extended-thinking / reasoning activation.
// Exactly one of BudgetTokens (claude-3.x budget-based) or Effort (claude-4+
// adaptive) should be set; if both are set, Effort takes precedence on models
// that support it.
type ThinkingConfig struct {
	// BudgetTokens is the reasoning token budget for budget-based models
	// (claude-3.5-sonnet, claude-3.7-sonnet, etc.). Zero is treated as a
	// sensible provider default (typically 1024).
	BudgetTokens int
	// Effort is the reasoning effort level for adaptive models
	// (claude-opus-4, claude-sonnet-4-5, etc.).
	// Accepted values: "low", "medium", "high", "max".
	// Empty string defers to the provider's default ("medium").
	Effort string
}

// ToolDef is a tool advertised to the model: name, description, JSON Schema
// for arguments. Execution is the harness layer's responsibility (see
// agent.Tool).
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// ModelCapabilities describes what a model can accept and produce. All fields
// default to false (conservative); providers set them from catalog data or
// live probes at construction time.
type ModelCapabilities struct {
	// Tools is true if the model handles function/tool calls natively.
	Tools bool
	// Images is true if the model accepts ImagePart inputs (vision).
	Images bool
	// Reasoning is true if the model emits ReasoningPart content
	// (Anthropic extended thinking, OpenAI o1/o3, DeepSeek R1).
	Reasoning bool
	// StructuredOutput is true if the model supports native JSON schema
	// enforcement (OpenAI json_schema response_format, Ollama format field).
	StructuredOutput bool
}

// ModelInfo is static metadata about a Model. Most fields come from the
// catalog (models/catalog) but providers can override after probing the
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
	// Capabilities describes what this model can accept and produce.
	Capabilities ModelCapabilities
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
