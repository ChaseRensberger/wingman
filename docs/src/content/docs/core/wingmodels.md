---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is the provider-agnostic model SDK inside Wingman. In code it is the `models/` package; the WingModels name is the product/docs name.

It gives the agent runtime one stable interface for talking to different model providers:

```go
type Model interface {
    Info() ModelInfo
    CountTokens(context.Context, []Message) (int, error)
    Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
}
```

The public surface is intentionally small: callers build a `models.Request`, receive normalized `models.StreamPart` values, and get a final `models.Message` when the stream closes.

## Use a model

Most callers construct providers through the provider registry used by the server:

```go
import provider "github.com/chaserensberger/wingman/models/providers"

m, err := provider.New("anthropic", map[string]any{
    "model": "claude-haiku-4-5",
})
if err != nil {
    return err
}
```

You can also import a provider directly when embedding Wingman in Go:

```go
import "github.com/chaserensberger/wingman/models/providers/anthropic"

m, err := anthropic.New(anthropic.Config{
    Model: "claude-haiku-4-5",
})
if err != nil {
    return err
}
```

Then stream a request:

```go
stream, err := m.Stream(ctx, models.Request{
    Messages: []models.Message{
        {
            Role: models.RoleUser,
            Content: models.Content{
                models.TextPart{Text: "Explain Wingman in one paragraph."},
            },
        },
    },
})
if err != nil {
    return err
}

for stream.Next() {
    part := stream.Event()
    _ = part
}
if err := stream.Err(); err != nil {
    return err
}
msg := stream.Result()
_ = msg
```

The agent loop consumes the same stream shape, so provider-specific details do not leak into sessions, plugins, storage, or HTTP responses.

## Request shape

`models.Request` carries the provider-neutral pieces a run can send to a model:

- `System` for the system prompt.
- `Messages` for conversation history.
- `Tools` and `ToolChoice` for tool use.
- `OutputSchema` for structured output when the selected model supports it.
- `MaxOutputTokens` for per-call output limits.

Messages are built from typed content parts such as `TextPart`, `ImagePart`, `ToolCallPart`, `ToolResultPart`, and `ReasoningPart`. Providers lower those parts into their own wire format before making the upstream request.

## Stream shape

All providers normalize their streaming responses into `models.StreamPart` events. The important categories are:

- Stream lifecycle: `StreamStartPart`, `FinishPart`, `ErrorPart`.
- Text: `TextStartPart`, `TextDeltaPart`, `TextEndPart`.
- Reasoning: `ReasoningStartPart`, `ReasoningDeltaPart`, `ReasoningEndPart`.
- Tool input: `ToolInputStartPart`, `ToolInputDeltaPart`, `ToolInputEndPart`, `ToolCallPart_`.
- Metadata and usage: `ResponseMetadataPart`, `Usage` on `FinishPart`.

The final result is a normalized assistant `models.Message` with content, origin metadata, finish reason, and usage.

## Internal architecture

WingModels is moving toward a route/protocol/transport split. This keeps provider constructors ergonomic while making shared provider families reusable.

```text
provider facade
  -> route.Route
       -> route.Protocol
       -> route.Endpoint
       -> route.Auth
       -> route.Transport
```

### Protocol

A protocol owns semantic API-family behavior:

- Lower `models.Request` into the provider family's wire request.
- Parse the provider family's streaming response into `models.StreamPart`.
- Count tokens or provide a local estimate.
- Advertise the API family, such as `openai-chat`.

The first reusable protocol is `models/protocols/openaichat`, which implements the OpenAI Chat Completions request and SSE stream shape.

### Route

A route combines protocol semantics with deployment details:

- Which endpoint path to call.
- Which auth strategy to apply.
- Which transport executes the request.
- Which `ModelRef` describes the selected provider/model/base URL.

Routes expose a `Prepare` seam that lowers a request into a transport-ready HTTP request without doing network I/O. Tests and diagnostics can inspect the final URL, headers, and JSON body directly.

### Transport

The default HTTP transport owns request execution mechanics:

- HTTP request construction.
- Retry behavior for transient failures.
- `Retry-After` handling.
- Streaming response handoff to the protocol parser.

Protocols do not know about retry policy. Provider facades do not hand-roll HTTP streaming loops.

### Provider facade

A provider facade is the user-facing configuration layer. It keeps the current constructor and registry behavior, resolves auth and options, builds a route, and returns a `models.Model`.

For example, `models/providers/openaicompat` is now a thin facade over `openaichat.Protocol` plus a route configured with `/chat/completions`, bearer auth, and HTTP transport. Future OpenAI-compatible providers can reuse that protocol instead of copying a full client.

## Catalog

The model catalog lives in `models/catalog`. Its source data is TOML under `models/catalog/data`, and `go generate ./models/catalog` compiles that data into an embedded snapshot.

Runtime catalog reads are local and deterministic. Wingman does not need a hosted metadata service to know model IDs, capabilities, API family, or default base URLs.

Providers use catalog metadata for `ModelInfo`, but callers still choose concrete providers and models explicitly.

## Why this design

The split keeps the public SDK stable while reducing provider duplication internally.

- The agent loop depends only on `models.Model`, not on provider clients.
- Protocols are reusable across compatible providers.
- Routes isolate endpoint, auth, and transport concerns from semantic request lowering.
- Request preparation can be tested without network calls.
- New provider families can be added without changing sessions, plugins, or storage.

The goal is not to build a generic LLM proxy abstraction. The goal is a Go-native model runtime that gives Wingman one normalized stream and keeps provider-specific complexity behind narrow seams.
