// Package models defines the model-layer abstractions for wingman: messages,
// parts (the units of message content), streaming events, and the Model
// interface that providers implement.
//
// # Wire format
//
// The streaming event shape is inspired is the Vercel AI SDK v3
// LanguageModelV3StreamPart shape TODO: migrate to v4, hyphenated names
// preserved exactly. We add two things on top of the AI SDK shape:
//
//  1. The "finish" event carries the assembled *Message, not just usage and
//     finish reason. This spares consumers from rebuilding state. The
//     Accumulator helper (accumulator.go) provides snapshot-per-event
//     ergonomics for callers that want them.
//  2. FinishReasonAborted is added to the AI SDK enum because context
//     cancellation is a first-class outcome in our agent loop.
//
// Stored Part shape (Message.Content) is opencode-derived: a discriminated
// union over Text/Reasoning/Image/ToolCall/ToolResult. See part.go.
//
// # Provider error contract
//
// Providers MUST return (nil, error) from Stream() only for synchronous setup
// failures (auth missing, network refused before the first response byte).
// Once the stream begins, all failures terminate via an "error" event followed
// by a "finish" event with FinishReasonError or FinishReasonAborted.
package models

// Role identifies the author of a Message in conversation history.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	// RoleTool wraps a ToolResultPart back into the conversation so the model
	// can see the outcome of a tool it called on the previous assistant turn.
	RoleTool Role = "tool"
)

// FinishReason explains why the assistant turn stopped.
//
// Names match Vercel AI SDK v3 LanguageModelV3FinishReason
// (bb/ai/packages/provider/src/language-model/v3/language-model-v3-finish-reason.ts)
// with one addition: "aborted" for context cancellation, which the AI SDK
// folds into "other".
type FinishReason string

const (
	// FinishReasonStop: model emitted an end-of-turn signal naturally.
	FinishReasonStop FinishReason = "stop"
	// FinishReasonLength: max output token cap was reached mid-generation.
	FinishReasonLength FinishReason = "length"
	// FinishReasonToolCalls: model emitted tool calls and is awaiting results.
	FinishReasonToolCalls FinishReason = "tool-calls"
	// FinishReasonContentFilter: provider safety filter blocked the response.
	FinishReasonContentFilter FinishReason = "content-filter"
	// FinishReasonError: provider or runtime failure mid-stream. The stream
	// will have emitted at least one "error" event before "finish".
	FinishReasonError FinishReason = "error"
	// FinishReasonAborted: context was cancelled by the caller. Wingman
	// addition; not present in the AI SDK enum.
	FinishReasonAborted FinishReason = "aborted"
	// FinishReasonOther: anything else the provider couldn't classify.
	FinishReasonOther FinishReason = "other"
	// FinishReasonUnknown: provider supplied no finish reason. Should be rare.
	FinishReasonUnknown FinishReason = "unknown"
)

// ProviderOptions is a two-level namespaced bag of provider-specific values.
// The outer key is the provider id or API family; the inner map is opaque.
//
// On Parts, providers use it for round-trip preservation of native metadata.
// On Requests, callers use it for provider-native knobs that do not belong in
// the cross-provider API. Providers ignore namespaces they do not own.
type ProviderOptions map[string]map[string]any
