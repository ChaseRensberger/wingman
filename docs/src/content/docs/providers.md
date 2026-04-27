---
title: "Providers"
group: "Concepts"
draft: false
order: 100
---

# Providers

A provider is any value that implements `wingmodels.Model`. The interface is small and provider-agnostic, so the rest of the runtime is independent of any specific backend.

## Interface

```go
type Model interface {
    Info() ModelInfo
    CountTokens(ctx context.Context, msgs []Message) (int, error)
    GenerateText(ctx context.Context, req InferenceRequest) (*InferenceResponse, error)
    StreamText(ctx context.Context, req InferenceRequest) (Stream, error)
}
```

`Info()` returns a `ModelInfo` with provider id, model id, and (when available) catalog metadata such as context window and pricing. `CountTokens` is exact for providers that expose a counting endpoint and falls back to a chars/4 estimate otherwise — plugins like compaction call it directly.

`StreamText` returns a `Stream` whose `Next/Part/Err` surface emits `wingmodels.StreamPart` values matching Vercel AI SDK v3 exactly. See [Streaming](./streaming).

## Cross-provider context handoff

Each provider call is responsible for rewriting the inbound message slice to fit the model it's about to invoke. At the top of `Stream`, the provider calls `transform.Apply(req.Messages, target)` where `target` describes the destination (`Provider`, `API`, `ModelID`, `Capabilities`). The pure function:

- Drops assistant messages with `FinishReason` `error`/`aborted` and any orphan tool calls/results that depended on them.
- Drops reasoning blocks unless the previous assistant message's `MessageOrigin` matches the new target exactly (same `API`, same `ModelID`).
- Replaces image parts with a text placeholder when the target model can't accept images (`Capabilities.Images == false`).
- Reconciles tool calls that have no matching tool result by injecting a synthetic error result.

After the model finishes, the provider stamps `MessageOrigin` and `FinishReason` on the assembled assistant message inside `FinishPart`, so the next turn — possibly on a different provider entirely — has the metadata it needs.

## Request capabilities

`wingmodels.Request` carries two first-class fields for caller-controlled behaviour:

**`ToolChoice`** — controls whether the model calls tools:

```go
req := wingmodels.Request{
    // ...
    ToolChoice: wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceRequired},
}
```

| `ToolChoiceMode` | Anthropic wire | Ollama wire | OpenAI Responses wire | OpenAI Chat Completions wire |
|---|---|---|---|---|
| `""` / `"auto"` | omitted (default) | omitted (default) | omitted (default) | omitted (default) |
| `"required"` | `tool_choice: {type: "any"}` | `tool_choice: "required"` | `tool_choice: "required"` | `tool_choice: "required"` |
| `"none"` | `tool_choice: {type: "none"}` | `tool_choice: "none"` | `tool_choice: "none"` | `tool_choice: "none"` |
| `"tool"` | `tool_choice: {type: "tool", name: ...}` | not supported (falls back to auto) | `tool_choice: {type: "function", name: ...}` | `tool_choice: {type: "function", function: {name: ...}}` |

`ToolChoice` is silently ignored when `Request.Tools` is empty.

**`Capabilities`** — cross-provider knobs:

```go
req := wingmodels.Request{
    // ...
    Capabilities: wingmodels.Capabilities{
        Thinking: &wingmodels.ThinkingConfig{
            BudgetTokens: 8000,  // for claude-3.x (budget-based)
            // Effort: "high",   // for claude-4+ (adaptive)
        },
    },
}
```

| Field | Effect |
|---|---|
| `Thinking` nil | No thinking block sent |
| `Thinking.BudgetTokens` on claude-3.x | `{"type":"enabled","budget_tokens":N}` + `anthropic-beta: interleaved-thinking` header |
| `Thinking.Effort` on claude-4+ (adaptive) | `{"type":"adaptive","display":"summarized"}` |
| `Thinking.*` on Ollama | Silently ignored |
| `Thinking.Effort` on OpenAI Responses API | `reasoning: {effort: "<value>"}` + `include: ["reasoning.encrypted_content"]` |
| `Thinking` nil on OpenAI reasoning model | `reasoning: {effort: "none"}` (disables reasoning) |
| `Thinking` nil on OpenAI non-reasoning model | `reasoning` field omitted |
| `Thinking.*` on OpenAI Chat Completions | Silently ignored |

`ToolChoice` and `Capabilities` are also exposed on `loop.Config` and forwarded to every `Request` the loop builds:

```go
ses := session.New(model, session.WithHooks(hooks))
// then drive via RunConfig:
result, err := loop.Run(ctx, loop.Config{
    Model:        model,
    Messages:     history,
    ToolChoice:   wingmodels.ToolChoice{Mode: wingmodels.ToolChoiceAuto},
    Capabilities: wingmodels.Capabilities{
        Thinking: &wingmodels.ThinkingConfig{BudgetTokens: 4096},
    },
})
```

## Provider and model are separate

Wingman stores `provider` and `model` as separate fields. The same model family can be exposed through different providers (different auth, different limits, different capabilities), so a combined string would lose information.

```json
{
  "provider": "anthropic",
  "model": "claude-haiku-4-5",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.2
  }
}
```

## Built-in providers

| ID | Package | Wire format | Notes |
|---|---|---|---|
| `anthropic` | `wingmodels/providers/anthropic` | Anthropic Messages API | Exact `CountTokens` via `/v1/messages/count_tokens`. |
| `ollama` | `wingmodels/providers/ollama` | Ollama Chat API | Local inference; `CountTokens` falls back to chars/4. |
| `openai` | `wingmodels/providers/openai` | OpenAI Responses API | `store: false`; full input each turn (stateless). `CountTokens` falls back to chars/4. |
| `opencodezen` | `wingmodels/providers/opencodezen` | OpenAI Chat Completions (proxy) | Multi-model proxy at `https://opencode.ai/zen/v1`. Auth: `OPENCODE_API_KEY`. Catalog key `opencode`. |

## SDK usage

Most SDK callers construct providers directly from their packages.

```go
import "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"

p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-haiku-4-5",
        "max_tokens": 4096,
    },
})
```

```go
import "github.com/chaserensberger/wingman/wingmodels/providers/ollama"

p, err := ollama.New(ollama.Config{
    Options: map[string]any{
        "model":    "llama3.2",
        "base_url": "http://localhost:11434",
    },
})
```

```go
import "github.com/chaserensberger/wingman/wingmodels/providers/openai"

p, err := openai.New(openai.Config{
    Options: map[string]any{
        "model":      "gpt-4o",
        "max_tokens": 4096,
    },
})
// API key: Config.APIKey → Options["api_key"] → OPENAI_API_KEY
```

```go
import "github.com/chaserensberger/wingman/wingmodels/providers/opencodezen"

p, err := opencodezen.New(opencodezen.Config{
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})
// API key: Config.APIKey → Options["api_key"] → OPENCODE_API_KEY
```

API key resolution order for `anthropic.New` is `Config.APIKey` → `Options["api_key"]` → `ANTHROPIC_API_KEY`.

## Registry path

The server uses a small registry to map a provider id to a factory. SDK callers can use the same path:

```go
import (
    provider "github.com/chaserensberger/wingman/wingmodels/providers"
    _ "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
)

m, err := provider.New("anthropic", map[string]any{
    "model":      "claude-haiku-4-5",
    "max_tokens": 4096,
    "api_key":    "sk-ant-...",
})
```

Each provider package's `init()` registers itself. Blank-import every provider you want available.

## The `options` map

Inference settings flow through a free-form `options` map. This keeps provider-specific parameters out of the core type system.

| Key | Anthropic | Ollama | OpenAI (Responses) | OpenCode Zen |
|---|---|---|---|---|
| `model` | required | required | required | required |
| `max_tokens` | supported | maps to `num_predict` | supported | supported |
| `temperature` | supported | supported | supported | supported |
| `api_key` | SDK convenience | not used | SDK convenience | SDK convenience |
| `base_url` | not used | supported | not used | not used |
| `max_retries` | supported (default 3) | not used | not used | not used |

## Server behavior

On the server, providers are rebuilt at request time from the agent's `provider`, `model`, and `options` fields. Credentials are loaded from the SQLite auth store and injected into the options map before the provider factory runs. The server does not consult environment variables at request time.

## Provider metadata endpoints

The server exposes provider metadata and model lookup routes. Model metadata is fetched from `models.dev` and cached.

- `GET /provider`
- `GET /provider/{name}`
- `GET /provider/{name}/models`
- `GET /provider/{name}/models/{model}`
