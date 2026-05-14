---
title: "WingModels"
group: "Core"
order: 103
draft: true
---

# WingModels

WingModels is Wingman's Go-native model layer. It gives the agent runtime one request shape, one message shape, and one stream shape while keeping provider wire formats behind the model client.

The current implementation is intentionally small. It supports three catalog model refs:

- `openai/gpt-5.5`
- `anthropic/claude-sonnet-4-6`
- `opencode/claude-sonnet-4-6`

OpenCode here means OpenCode Zen, exposed through provider ID `opencode`.

## Why It Exists

Provider APIs are similar enough to normalize, but not similar enough to hide carelessly.

WingModels exists because the agent runtime needs:

- One conversation shape for storage and replay.
- One stream shape for UI, plugins, and HTTP events.
- Provider-specific request lowering and SSE parsing behind one client.
- Local model metadata without depending on a hosted metadata service.
- Model refs that can change per message without binding a session to one provider.

The goal is not a generic LLM proxy. The goal is a small model layer that lets Wingman run sessions without provider details leaking into the loop.

## Public Shape

The loop talks to a `models.Client`:

```go
type Client interface {
    Prepare(context.Context, Request) (*PreparedRequest, error)
    Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
    Generate(context.Context, Request) (*Message, error)
}
```

`Prepare` lowers a WingModels request into provider-native JSON without sending it. This is useful for debugging, tests, and UI previews.

`Stream` sends the request and returns normalized stream parts.

`Generate` drains the stream and returns the final assembled assistant message.

Requests carry a provider-qualified model ref:

```go
type Request struct {
    Model           ModelRef
    System          string
    Messages        []Message
    Tools           []ToolDef
    ToolChoice      ToolChoice
    Generation      Generation
    Capabilities    Capabilities
    ProviderOptions ProviderBag
    HTTP            HTTPOptions
    ResponseFormat  ResponseFormat
    OutputSchema    *OutputSchema
    MaxOutputTokens int
}
```

`ModelRef` is the stable model identity used by callers:

```go
type ModelRef struct {
    Provider string
    ID       string
    API      API
    BaseURL  string
}
```

Use refs like `openai/gpt-5.5`, not separate conceptual provider and model choices in new code. The store still has provider/model columns for now, but server handlers compute and expose `model_ref` while the storage migration remains incremental.

## Messages And Parts

`Message` is the provider-neutral stored conversation shape:

```go
type Message struct {
    Role         Role
    Content      Content
    FinishReason FinishReason
    Origin       *MessageOrigin
    Metadata     Meta
}
```

`Content` contains typed `Part` values:

- `TextPart` for plain text.
- `ImagePart` for image references.
- `ReasoningPart` for provider reasoning text.
- `ToolCallPart` for assistant-emitted tool calls.
- `ToolResultPart` for tool outputs.
- `OpaquePart` for plugin-defined persisted parts.

The store persists this common shape. Providers lower it into their wire formats at request time.

## Streaming

Every provider emits `models.StreamPart` values.

The current lifecycle is:

```text
StreamStartPart
(TextStartPart | TextDeltaPart | TextEndPart | ToolInputStartPart | ToolInputDeltaPart | ToolInputEndPart | ToolCallPart_ | ResponseMetadataPart | ErrorPart)*
FinishPart
```

`FinishPart` carries usage, finish reason, and the final assembled assistant message. Consumers can also call `EventStream.Final()` after draining the stream.

Example:

```go
ref, _ := models.ParseModelRef("opencode/claude-sonnet-4-6")
info, _ := catalog.Get(ref.Provider, ref.ID)
ref.API = info.API
ref.BaseURL = info.BaseURL

client := provider.NewClient(nil)

stream, err := client.Stream(ctx, models.Request{
    Model:  ref,
    System: "You are concise.",
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

For a complete runnable example, see `examples/models/main.go`.

## Provider Client

Provider packages register provider metadata:

```go
import (
    _ "github.com/chaserensberger/wingman/models/providers/anthropic"
    _ "github.com/chaserensberger/wingman/models/providers/openai"
    _ "github.com/chaserensberger/wingman/models/providers/opencode"
)
```

Runtime calls go through `provider.NewClient`:

```go
client := provider.NewClient(map[string]string{
    "opencode": os.Getenv("OPENCODE_API_KEY"),
})
```

If an API key is not passed explicitly, the client falls back to the first populated provider catalog `env` value:

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `OPENCODE_API_KEY`

The provider client resolves `Request.Model` through `models/catalog`, selects the right protocol, and streams through the shared HTTP/SSE implementation.

## Supported Protocols

Current protocol support is deliberately narrow:

- OpenAI Responses for `openai/gpt-5.5`.
- Anthropic Messages for `anthropic/claude-sonnet-4-6`.
- Anthropic Messages over OpenCode Zen for `opencode/claude-sonnet-4-6`.

OpenAI Chat Completions support exists in the HTTP model implementation, but no catalog model currently uses it.

Unsupported today:

- Ollama.
- `opencodezen` as a provider ID.
- Generic OpenAI-compatible providers.
- Gemini.
- Bedrock.
- OpenRouter or other gateway providers.

## Catalog

The catalog is embedded TOML under `models/catalog/providers`.

Current files:

```text
models/catalog/providers/anthropic/provider.toml
models/catalog/providers/anthropic/claude-sonnet-4-6.toml
models/catalog/providers/openai/provider.toml
models/catalog/providers/openai/GPT-5.5.toml
models/catalog/providers/opencode/provider.toml
models/catalog/providers/opencode/claude-sonnet-4-6.toml
```

There is no generated snapshot and no lab/provider split. The catalog only contains fields that current code uses.

Example provider entry:

```toml
base_url = "https://opencode.ai/zen/v1"
env = ["OPENCODE_API_KEY"]
```

Example model entry:

```toml
id = "claude-sonnet-4-6"
provider = "opencode"
api = "anthropic_messages"
context_window = 200000
max_output = 8192

[capabilities]
tools = true
images = true
reasoning = true
structured_output = true
```

Current catalog fields:

Provider fields:

- `base_url`: default provider API base URL.
- `env`: environment variables required by the provider. The current API-key client uses the first populated value as the fallback API key.

Model fields:

- `id`: provider-local model ID.
- `provider`: provider ID used in model refs.
- `api`: protocol selector (`openai_responses`, `openai_completions`, or `anthropic_messages`).
- `base_url`: optional provider API base URL override.
- `env`: optional provider environment variable override.
- `context_window`: coarse context limit used by runtime gates/plugins.
- `max_output`: default maximum output tokens where needed.
- `capabilities`: booleans used for runtime/API capability checks.

Do not add catalog fields speculatively. A field belongs here only when runtime code, API responses, or docs use it now.

## Prepare

`Prepare` shows the exact provider-native request body without making a network call:

```go
prepared, err := client.Prepare(ctx, models.Request{
    Model: ref,
    Messages: []models.Message{
        models.NewUserText("Say hello."),
    },
})
if err != nil {
    return err
}

fmt.Println(prepared.URL)
fmt.Println(prepared.Body)
```

This is the main inspection seam while the protocol layer is still small.

## Sessions And Model Switching

Wingman sessions are still not bound to one model. The server stores agent defaults, but the loop itself receives a `models.ModelRef` for the current run. That keeps the architecture open for per-message model switching.

The loop no longer owns provider-specific model objects. It only knows:

- `models.Client`
- `models.ModelRef`
- `models.ModelInfo`
- provider-neutral messages/tools/stream parts

## Current Limitations

WingModels is usable for the narrow supported path, but it is not a broad provider SDK yet.

Known limitations:

- The protocol implementation handles the common text/tool/usage streaming paths, not every provider event type.
- There is no generic OpenAI-compatible provider.
- There is no `CountTokens` API. Compaction currently uses a local approximation.
- There is no separate `models/transform` package for cross-provider replay normalization yet.
- Catalog metadata is intentionally tiny and should stay that way until fields have direct consumers.
- Structured output support is represented in metadata, but provider-specific response-format behavior is still minimal.

The important boundary is already in place: the agent loop depends on `models.Client` and `models.ModelRef`, not provider-owned model implementations.
