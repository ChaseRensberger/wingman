---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is Wingman's provider agnostic model sdk (written in Go). It gives the agent runtime one request shape, one message shape, and one stream shape while keeping provider wire formats behind the model curtain.

## Supported Providers 

- Anthropic
- OpenAI
- OpenCode Zen

with many more coming soon.

## Why It Exists

WingModels exists because the agent runtime needs:

- One conversation shape for storage and replay.
- One stream shape for UI, plugins, and HTTP events.
- Provider-specific request lowering and SSE parsing behind a single model client.
- Local model metadata without depending on a hosted metadata service (Wingman's Catalog).
- Model refs that can change per message without binding a session to one provider (Context handoff). 

## Shape

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
    Provider      string
    ID            string
    API           API
    BaseURL       string
    Env           []string
    ContextWindow int
    MaxOutput     int
    Capabilities  ModelCapabilities
}
```

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

If an API key is not passed explicitly, the client falls back to the first populated `env` value from catalog metadata or explicit route metadata:

- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `OPENCODE_API_KEY`

The provider client resolves `Request.Model` through `models/catalog` first. If the ref is not cataloged, it uses the explicit route metadata on `Request.Model`. A custom model must provide `api` and `base_url`; otherwise the client returns an unknown-model error.

## Supported Protocols

Current protocol support is deliberately narrow:

- OpenAI Responses (`openai_responses`).
- OpenAI Chat Completions (`openai_completions`).
- Anthropic Messages (`anthropic_messages`).

The catalog currently uses OpenAI Responses and Anthropic Messages. OpenAI Chat Completions is available for explicit custom routes.

## Catalog

The catalog is embedded TOML under `models/catalog/providers`.

Current files:

```text
models/catalog/providers/anthropic/provider.toml
models/catalog/providers/anthropic/claude-haiku-4-5.toml
models/catalog/providers/anthropic/claude-opus-4-7.toml
models/catalog/providers/anthropic/claude-sonnet-4-6.toml
models/catalog/providers/openai/provider.toml
models/catalog/providers/openai/gpt-5.5.toml
models/catalog/providers/openai/gpt-5.3-codex.toml
models/catalog/providers/openai/gpt-5.4-mini.toml
models/catalog/providers/openai/gpt-5.4-nano.toml
models/catalog/providers/openai/gpt-5.5-pro.toml
models/catalog/providers/opencode/provider.toml
models/catalog/providers/opencode/claude-sonnet-4-6.toml
models/catalog/providers/opencode/claude-opus-4-7.toml
models/catalog/providers/opencode/deepseek-v4-flash-free.toml
models/catalog/providers/opencode/gpt-5.4-mini.toml
models/catalog/providers/opencode/gpt-5.5.toml
models/catalog/providers/opencode/gpt-5.5-pro.toml
models/catalog/providers/opencode/kimi-k2.6.toml
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

## Custom Models

Catalog membership is not required when the caller supplies route metadata. This is useful when a provider exposes a new model before the local catalog is updated, or when an embedding application wants to target an OpenAI-compatible deployment without adding TOML.

SDK example:

```go
ref := models.ModelRef{
    Provider: "openai",
    ID:       "gpt-4.1",
    API:      models.APIOpenAIResponses,
    BaseURL:  "https://api.openai.com/v1",
    Env:      []string{"OPENAI_API_KEY"},
    Capabilities: models.ModelCapabilities{
        Tools:            true,
        Images:           true,
        StructuredOutput: true,
    },
}

msg, err := provider.NewClient(nil).Generate(ctx, models.Request{
    Model:    ref,
    Messages: []models.Message{models.NewUserText("Say hello.")},
})
```

HTTP/server agents can pass the same metadata as `model_route`; Wingman stores it under the agent `options.model_route` field:

```json
{
  "name": "custom-openai",
  "model_ref": "openai/gpt-4.1",
  "model_route": {
    "api": "openai_responses",
    "base_url": "https://api.openai.com/v1",
    "env": ["OPENAI_API_KEY"],
    "context_window": 1047576,
    "max_output": 32768,
    "capabilities": {
      "tools": true,
      "images": true,
      "structured_output": true
    }
  }
}
```

If `model_ref` is in the catalog, the catalog wins. If it is not in the catalog, `model_route.provider` and `model_route.id` may be omitted; they default to the provider and model ID parsed from `model_ref`. If supplied, they must match `model_ref`.

Custom routes do not add broad provider support by themselves. The target endpoint must speak one of the supported wire protocols.

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
- There is no first-class generic OpenAI-compatible provider catalog or discovery flow; explicit OpenAI-compatible routes can be supplied manually.
- There is no `CountTokens` API. Compaction currently uses a local approximation.
- There is no separate `models/transform` package for cross-provider replay normalization yet.
- Catalog metadata is intentionally tiny and should stay that way until fields have direct consumers.
- Structured output support is represented in metadata, but provider-specific response-format behavior is still minimal.

The important boundary is already in place: the agent loop depends on `models.Client` and `models.ModelRef`, not provider-owned model implementations.
