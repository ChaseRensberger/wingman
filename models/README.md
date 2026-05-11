# WingModels

WingModels is the provider-agnostic model API for [Wingman](https://github.com/chaserensberger/wingman).

## What it provides

- **Common abstractions** — `Model`, `Message`, `Part`, and `EventStream` that every provider satisfies.
- **Streaming wire format** — `StreamPart` events (text, reasoning, tool calls, errors, finish) based on Vercel AI SDK v3 with a few ergonomic additions (e.g. `FinishPart` carries the assembled `*Message`).
- **Provider registry** — a small registry (`providers/`) that maps provider ids like `"anthropic"` or `"ollama"` to factory functions.
- **Model catalog** — a bundled TOML-backed catalog (`catalog/`) for model metadata: context windows, capabilities, pricing, and provider model lists.
- **Message normalization** — `transform/` cleans conversation history before each request (drops failed turns, strips reasoning blocks when switching models, reconciles tool calls, etc.).

## Package layout

```
models/
  models.go   # Roles, FinishReason, ProviderOptions
  model.go        # Model interface, Request, Capabilities, ToolDef, Run()
  message.go      # Message, Content, Part JSON helpers, Usage
  event.go        # StreamPart union (text/reasoning/tool/finish/error events)
  part.go         # Part union (Text/Reasoning/Image/ToolCall/ToolResult)
  stream.go       # EventStream — producer/consumer event channel with final result
  accumulator.go  # Accumulate() rebuilds a running message snapshot from events
  catalog/        # Bundled TOML-backed model catalog
  providers/      # Provider registry + built-in provider implementations
  transform/      # Cross-model message normalization
```

## Concepts

- **Model** — the interface every provider implements. It exposes `Info()`, `Stream()`, and `CountTokens()`.
- **Request** — what you send: system prompt, conversation history (`[]Message`), available tools, and knobs like `ToolChoice` and `Thinking`.
- **Message** — one turn in the conversation. `Role` is `user`, `assistant`, or `tool`. `Content` is a slice of `Part`.
- **Part** — a discriminated union of `TextPart`, `ReasoningPart`, `ImagePart`, `ToolCallPart`, and `ToolResultPart`.
- **StreamPart** — events emitted during a streaming response. Text, reasoning, and tool calls arrive as start/delta/end triples. The stream always terminates with a `FinishPart`.
- **EventStream** — one-producer, one-consumer stream of `StreamPart` events. Range over `Iter()`, then call `Final()` to get the assembled `*Message`.

## Quick start

### Direct provider (sync)

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/chaserensberger/wingman/models"
    "github.com/chaserensberger/wingman/models/providers/anthropic"
)

func main() {
    // Create a provider client directly.
    client, err := anthropic.New(anthropic.Config{
        APIKey: "sk-ant-...",
        Model:  "claude-sonnet-4.5",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Build a request.
    req := models.Request{
        System: "You are a helpful assistant.",
        Messages: []models.Message{
            models.NewUserText("What is 2 + 2?"),
        },
    }

    // Run synchronously. For streaming, use client.Stream() instead.
    msg, err := models.Run(context.Background(), client, req)
    if err != nil {
        log.Fatal(err)
    }

    // Extract the assistant's text reply.
    if t, ok := msg.Content[0].(models.TextPart); ok {
        fmt.Println(t.Text)
    }
}
```

### Direct provider (streaming)

```go
stream, err := client.Stream(ctx, req)
if err != nil {
    log.Fatal(err)
}

for ev := range stream.Iter() {
    switch p := ev.(type) {
    case models.TextDeltaPart:
        fmt.Print(p.Delta)
    case models.FinishPart:
        fmt.Println("\nDone — reason:", p.Reason)
    }
}

msg, err := stream.Final()
```

### Using the provider registry

The registry lets you construct a model by provider id without importing the package directly (useful when the provider is chosen at runtime).

```go
import (
    _ "github.com/chaserensberger/wingman/models/providers/anthropic" // registers itself
    _ "github.com/chaserensberger/wingman/models/providers/ollama"
    provider "github.com/chaserensberger/wingman/models/providers"
)

m, err := provider.New("anthropic", map[string]any{
    "api_key": "sk-ant-...",
    "model":   "claude-sonnet-4.5",
})
```

### Using the catalog

Look up static model metadata (context window, capabilities, pricing) without making a request:

```go
import "github.com/chaserensberger/wingman/models/catalog"

info, ok := catalog.Get("anthropic", "claude-sonnet-4.5")
if ok {
    fmt.Println("Context window:", info.ContextWindow)
    fmt.Println("Supports tools:", info.Capabilities.Tools)
}
```

The catalog source of truth lives as TOML under `models/catalog/data`. Run `go generate ./models/catalog` after editing the TOML to regenerate the embedded snapshot. Runtime catalog lookups are local and work offline.

## Using tools

Add `ToolDef` values to `Request.Tools` to let the model call functions:

```go
req.Tools = []models.ToolDef{
    {
        Name:        "get_weather",
        Description: "Get the current weather for a city.",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "city": map[string]any{"type": "string"},
            },
            "required": []string{"city"},
        },
    },
}
```

When the model emits a `ToolCallPart`, execute the tool externally and reply with a `ToolResultPart` message:

```go
toolCall := msg.Content[0].(models.ToolCallPart)
result := executeTool(toolCall) // your implementation

reply := models.NewToolResult(toolCall.CallID, []models.Part{
    models.TextPart{Text: result},
}, false)
```

## Accumulator

If you want a running snapshot of the assistant message while streaming (useful for rendering incremental UI), wrap the stream with `Accumulate`:

```go
for snap, ev := range models.Accumulate(stream) {
    // snap.Message holds the assembled message so far
    // ev is the current stream event
}
```

## Built-in providers

- **anthropic** — Anthropic Messages API (`/v1/messages`). Supports streaming, extended thinking, tool calls, and token counting.
- **ollama** — Local Ollama server (`/api/chat`). Supports streaming and tool calls. Token counting uses a chars/4 heuristic.
- **openai** — OpenAI Responses API.
- **opencodezen** — OpenCode Zen's OpenAI-compatible multi-model proxy.
- **openaicompat** — OpenAI-compatible services (DeepSeek, Groq, OpenRouter, etc.).

New providers can be added by implementing `models.Model` and registering a factory via `provider.Register`.
