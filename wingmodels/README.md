# WingModels

WingModels is the provider-agnostic model API for [Wingman](https://github.com/chaserensberger/wingman).

## What it provides

- **Common abstractions** — `Model`, `Message`, `Part`, and `EventStream` that every provider satisfies.
- **Streaming wire format** — `StreamPart` events (text, reasoning, tool calls, errors, finish) based on Vercel AI SDK v3 with a few ergonomic additions (e.g. `FinishPart` carries the assembled `*Message`).
- **Provider registry** — a small registry (`providers/`) that maps provider ids like `"anthropic"` or `"ollama"` to factory functions.
- **Model catalog** — an embedded + live-updated copy of [models.dev](https://models.dev) metadata (context window, capabilities, pricing) via `catalog/`.
- **Message normalization** — `transform/` cleans conversation history before each request (drops failed turns, strips reasoning blocks when switching models, reconciles tool calls, etc.).

## Package layout

```
wingmodels/
  wingmodels.go   # Roles, FinishReason, ProviderOptions
  model.go        # Model interface, Request, Capabilities, ToolDef, Run()
  message.go      # Message, Content, Part JSON helpers, Usage
  event.go        # StreamPart union (text/reasoning/tool/finish/error events)
  part.go         # Part union (Text/Reasoning/Image/ToolCall/ToolResult)
  stream.go       # EventStream — producer/consumer event channel with final result
  accumulator.go  # Accumulate() rebuilds a running message snapshot from events
  catalog/        # Embedded models.dev catalog with live refresh
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

    "github.com/chaserensberger/wingman/wingmodels"
    "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
)

func main() {
    // Create a provider client directly.
    client, err := anthropic.New(anthropic.Config{
        APIKey: "sk-ant-...",
        Model:  "claude-sonnet-4",
    })
    if err != nil {
        log.Fatal(err)
    }

    // Build a request.
    req := wingmodels.Request{
        System: "You are a helpful assistant.",
        Messages: []wingmodels.Message{
            wingmodels.NewUserText("What is 2 + 2?"),
        },
    }

    // Run synchronously. For streaming, use client.Stream() instead.
    msg, err := wingmodels.Run(context.Background(), client, req)
    if err != nil {
        log.Fatal(err)
    }

    // Extract the assistant's text reply.
    if t, ok := msg.Content[0].(wingmodels.TextPart); ok {
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
    case wingmodels.TextDeltaPart:
        fmt.Print(p.Delta)
    case wingmodels.FinishPart:
        fmt.Println("\nDone — reason:", p.Reason)
    }
}

msg, err := stream.Final()
```

### Using the provider registry

The registry lets you construct a model by provider id without importing the package directly (useful when the provider is chosen at runtime).

```go
import (
    _ "github.com/chaserensberger/wingman/wingmodels/providers/anthropic" // registers itself
    _ "github.com/chaserensberger/wingman/wingmodels/providers/ollama"
    provider "github.com/chaserensberger/wingman/wingmodels/providers"
)

m, err := provider.New("anthropic", map[string]any{
    "api_key": "sk-ant-...",
    "model":   "claude-sonnet-4",
})
```

### Using the catalog

Look up static model metadata (context window, capabilities, pricing) without making a request:

```go
import "github.com/chaserensberger/wingman/wingmodels/catalog"

info, ok := catalog.Get("anthropic", "claude-sonnet-4")
if ok {
    fmt.Println("Context window:", info.ContextWindow)
    fmt.Println("Supports tools:", info.Capabilities.Tools)
}
```

The default catalog is preloaded from an embedded snapshot at init and works offline. Call `catalog.Default().StartRefresher(...)` if you want periodic live updates from models.dev.

## Using tools

Add `ToolDef` values to `Request.Tools` to let the model call functions:

```go
req.Tools = []wingmodels.ToolDef{
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
toolCall := msg.Content[0].(wingmodels.ToolCallPart)
result := executeTool(toolCall) // your implementation

reply := wingmodels.NewToolResult(toolCall.CallID, []wingmodels.Part{
    wingmodels.TextPart{Text: result},
}, false)
```

## Accumulator

If you want a running snapshot of the assistant message while streaming (useful for rendering incremental UI), wrap the stream with `Accumulate`:

```go
for snap, ev := range wingmodels.Accumulate(stream) {
    // snap.Message holds the assembled message so far
    // ev is the current stream event
}
```

## Built-in providers

- **anthropic** — Anthropic Messages API (`/v1/messages`). Supports streaming, extended thinking, tool calls, and token counting.
- **ollama** — Local Ollama server (`/api/chat`). Supports streaming and tool calls. Token counting uses a chars/4 heuristic.
- **openai** — OpenAI Chat Completions.
- **openaicompat** — OpenAI-compatible services (DeepSeek, Groq, OpenRouter, etc.).

New providers can be added by implementing `wingmodels.Model` and registering a factory via `provider.Register`.
