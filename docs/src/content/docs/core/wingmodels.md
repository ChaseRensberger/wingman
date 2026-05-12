---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is Wingman's model runtime. In code it is the `models/` package; the WingModels name is the docs/product name.

Its job is narrow: give the agent runtime one stable way to call LLM providers, stream normalized events, preserve conversation history across providers, and expose enough model metadata for capability gates, cost display, and compaction decisions.

The core interface is intentionally small:

```go
type Model interface {
    Info() ModelInfo
    Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
    CountTokens(context.Context, []Message) (int, error)
}
```

The rest of Wingman depends on that interface, not on Anthropic, OpenAI, OpenCode Zen, or any provider SDK.

## Why It Exists

Provider APIs are similar enough to normalize, but not similar enough to hide carelessly.

WingModels exists because the agent runtime needs:

- One conversation shape for storage and replay.
- One stream shape for UI, plugins, and HTTP events.
- Cross-provider context handoff when a session changes models.
- Provider-specific request lowering and stream parsing behind small seams.
- Local catalog metadata without depending on a hosted metadata service.

The goal is not a generic LLM proxy. The goal is a Go-native model layer that lets Wingman run sessions without provider details leaking into the session loop.

## Public Shape

Callers send a `models.Request`:

```go
type Request struct {
    System          string
    Messages        []Message
    Tools           []ToolDef
    MaxOutputTokens int
    ToolChoice      ToolChoice
    Capabilities    Capabilities
    OutputSchema    *OutputSchema
    ProviderOptions ProviderOptions
}
```

`Messages` contain typed `Part` values:

- `TextPart` for plain text.
- `ImagePart` for inline image input.
- `ReasoningPart` for provider reasoning/thinking traces and replay signatures.
- `ToolCallPart` for model-emitted tool calls stored in assistant history.
- `ToolResultPart` for tool outputs stored in tool messages.

Providers lower those parts into their own wire format. The session store keeps the common WingModels shape, not provider-native JSON.

## Streaming

Every provider emits `models.StreamPart` values.

The stream lifecycle is:

```text
StreamStartPart?
(Text* | Reasoning* | ToolInput* | ToolCallPart_ | ToolResultPart_ | ResponseMetadataPart | ErrorPart)*
FinishPart
```

The important event families are:

- Text: `TextStartPart`, `TextDeltaPart`, `TextEndPart`.
- Reasoning: `ReasoningStartPart`, `ReasoningDeltaPart`, `ReasoningEndPart`.
- Tool input: `ToolInputStartPart`, `ToolInputDeltaPart`, `ToolInputEndPart`, `ToolCallPart_`.
- Lifecycle: `StreamStartPart`, `ResponseMetadataPart`, `ErrorPart`, `FinishPart`.

`FinishPart` carries the final assembled assistant message. Consumers can also read the final message from `EventStream.Final()`.

```go
stream, err := model.Stream(ctx, models.Request{
    Messages: []models.Message{
        models.NewUserText("Explain Wingman in one paragraph."),
    },
})
if err != nil {
    return err
}

for part := range stream.Iter() {
    _ = part
}

msg, err := stream.Final()
if err != nil {
    return err
}
_ = msg
```

Setup failures return from `Stream`. Once streaming starts, providers should report stream-time failures as `ErrorPart` followed by a terminal `FinishPart` with an error/aborted finish reason.

## Architecture

WingModels is split into four layers:

```text
provider facade
  -> route.Route
       -> route.Protocol
       -> route.Endpoint
       -> route.Auth
       -> route.Transport
```

This split is the main design decision. It keeps public constructors ergonomic while preventing every provider from copying its own HTTP loop, retry policy, request builder, and SSE parser.

### Provider Facades

Provider facades are the user-facing constructors and registry entries under `models/providers`.

They own:

- Default model selection.
- API key and option resolution.
- Provider registry metadata.
- Construction of a concrete `route.Route` and `route.ModelRef`.
- Provider-specific behavior that is not part of streaming, such as Anthropic's exact `count_tokens` endpoint.

The currently registered first-party providers are imported by `cmd/wingman/main.go`:

```go
_ "github.com/chaserensberger/wingman/models/providers/anthropic"
_ "github.com/chaserensberger/wingman/models/providers/ollama"
_ "github.com/chaserensberger/wingman/models/providers/openai"
_ "github.com/chaserensberger/wingman/models/providers/opencodezen"
```

The route-backed providers are:

- `models/providers/anthropic` using Anthropic Messages.
- `models/providers/openai` using OpenAI Responses.
- `models/providers/openaicompat` using OpenAI Chat Completions.
- `models/providers/opencodezen`, a thin OpenAI-compatible facade over OpenCode Zen.

`models/providers/ollama` is still a direct provider implementation.

### Routes

`models/route` composes a runnable deployment:

- `Protocol`: semantic request/stream behavior.
- `Endpoint`: URL construction.
- `Auth`: request authentication.
- `Transport`: HTTP execution, retries, and response handoff.
- `ModelRef`: provider/model/base URL/catalog info for one concrete model.

Routes expose `Prepare`, which lowers a common `models.Request` into a transport-ready HTTP request without sending it. This is important for tests and diagnostics: provider-native JSON can be inspected without spending tokens or depending on a network call.

### Protocols

Protocols are API-family adapters. They know how to turn WingModels messages into one provider-family request shape and how to parse that family back into `StreamPart` events.

Current common protocols:

- `models/protocols/openaichat`: OpenAI Chat Completions, used by OpenAI-compatible providers.
- `models/protocols/openairesponses`: OpenAI Responses, used by the OpenAI provider.
- `models/protocols/anthropicmessages`: Anthropic Messages, used by the Anthropic provider.

Protocols do not decide API keys, default model IDs, base URLs, or registry names. Those belong to provider facades and routes.

### Transport

The default transport is `route.HTTPTransport`. It owns:

- HTTP request execution.
- Retry behavior for transient failures.
- `Retry-After` handling.
- Returning the streaming response to the protocol parser.

This keeps protocol code focused on semantics. A protocol should not need to know how retries work, and a provider facade should not need to hand-roll an SSE loop.

## Catalog

The catalog lives under `models/catalog`.

Source data is TOML in `models/catalog/data`. `go generate ./models/catalog` compiles that source into `models/catalog/wingmodels_snapshot.json`, which is embedded into the binary.

There are three catalog layers:

```text
labs/<lab>/models/*.toml
providers/<provider>/provider.toml
providers/<provider>/models/*.toml
```

### Lab Models

Lab model files describe durable model identity: who made the model and what the model is.

Example path:

```text
models/catalog/data/labs/openai/models/gpt-5.5.toml
```

Lab model fields include:

- `id`
- `lab_id`
- `display_name`
- `family`
- `release_date`
- `last_updated`
- `knowledge_cutoff`
- `open_weights`
- baseline `modalities`

Lab models should not describe provider request quirks.

### Providers

Provider files describe a deployment surface:

```text
models/catalog/data/providers/openai/provider.toml
models/catalog/data/providers/anthropic/provider.toml
models/catalog/data/providers/opencode-zen/provider.toml
```

Provider fields include:

- provider ID
- display name
- base URLs
- auth environment variables

### Provider Models

Provider model files describe how one provider exposes one model.

Example path:

```text
models/catalog/data/providers/openai/models/gpt-5.5.toml
```

Provider model fields include:

- `id`, always namespaced as `<provider>/<model>`.
- `route_profile`, such as `openai.responses`, `anthropic.messages`, or `opencode-zen.openai-chat`.
- `capabilities`.
- `limits`.
- `pricing`.
- provider-specific `modalities` when they differ from the lab model.

This is where operational support belongs. For example, reasoning/thinking support is represented as a provider-model capability, not as a canonical lab-model boolean.

```toml
[[capabilities]]
id = "reasoning"
```

That distinction matters because reasoning is exposed differently by different APIs: OpenAI Responses, Anthropic Messages, Gemini, Bedrock, and OpenAI-compatible proxies all use different request and replay semantics.

## Bundled Catalog Set

The current bundled first-party catalog is intentionally small.

Anthropic provider:

- `claude-haiku-4.5`
- `claude-sonnet-4.6`
- `claude-opus-4.7`

OpenAI provider:

- `gpt-5.5`

OpenCode Zen provider:

- `gpt-5.5`
- `claude-haiku-4.5`
- `claude-sonnet-4.6`
- `claude-opus-4.7`

OpenCode Zen exposes those models through an OpenAI-compatible Chat Completions surface, so its provider-model entries use the `opencode-zen.openai-chat` route profile even when the underlying model family is Claude.

## Context Handoff

Wingman sessions can change models mid-conversation. That means stored history must be replayable across providers.

`models/transform` normalizes history before a protocol builds the wire request. It handles provider boundaries such as:

- Dropping failed or aborted assistant turns.
- Dropping cross-model reasoning signatures that are only valid for the original model.
- Replacing images with placeholders when the target model lacks image input.
- Inserting synthetic error tool results for orphaned tool calls.
- Removing messages that become empty after normalization.

Assistant messages carry `MessageOrigin` so the transform layer can tell whether a replay targets the same provider/API/model or a different one. Same-model replay can preserve provider-specific reasoning signatures; cross-model replay must be conservative.

## Counting Tokens

`CountTokens` is part of `models.Model` because the agent loop needs a model-specific context-pressure estimate for compaction and budgeting.

The semantics are best effort:

- Anthropic uses the exact `/v1/messages/count_tokens` endpoint.
- OpenAI Responses currently uses a local approximation.
- OpenAI-compatible providers currently use a local approximation.
- Ollama may use provider-local behavior where available.

This is good enough for compaction decisions, but it is not a billing-grade API. A future refinement should return a richer token count with an `Exact` flag instead of only `int`.

`Info()` remains required because sessions, provider listings, and capability gates need cheap access to provider/model ID, API family, context limits, costs, and capabilities.

## Compatibility

Different providers can speak the same broad protocol with small dialect differences. The rule of thumb is:

- Different wire family: add a protocol.
- Same wire family with field quirks: reuse the protocol with compatibility/profile data.
- Different URL/auth/retry behavior: use a different route.
- Different model capabilities/pricing/limits: catalog provider-model data.

Today, OpenCode Zen and generic OpenAI-compatible providers reuse `openaichat.Protocol` with explicit compatibility profiles. Profiles cover small dialect differences such as `max_tokens` vs. `max_completion_tokens`, system vs. developer role, optional `store`, and reasoning field names. Request-level `ProviderOptions` remains available for provider-native knobs that should not become cross-provider API fields.

## What Is Still In Flight

WingModels is usable today, but the architecture is still settling.

Known follow-ups:

- Decide whether the generated catalog snapshot should stay committed or be replaced by a generated Go source file.
- Add richer tiered pricing support, such as over-200k-token pricing.
- Add stream fixture tests for the route-backed protocols.
- Add a dedicated framing seam before bringing back Bedrock Converse support.
- Reintroduce Gemini support through the route layer when a first-party provider facade is ready.

The important part is the boundary: the agent loop should only know `models.Model`; providers should be small facades; protocols should be reusable; catalog data should describe model availability without encoding runtime behavior.
