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

| ID | Package | Notes |
|---|---|---|
| `anthropic` | `wingmodels/providers/anthropic` | Exact `CountTokens` via `/v1/messages/count_tokens`. |
| `ollama` | `wingmodels/providers/ollama` | Local inference; `CountTokens` falls back to chars/4. |

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

| Key | Anthropic | Ollama |
|---|---|---|
| `model` | required | required |
| `max_tokens` | supported | maps to `num_predict` |
| `temperature` | supported | supported |
| `api_key` | SDK convenience | not used |
| `base_url` | not used | supported |
| `max_retries` | supported (default 3) | not used |

## Server behavior

On the server, providers are rebuilt at request time from the agent's `provider`, `model`, and `options` fields. Credentials are loaded from the SQLite auth store and injected into the options map before the provider factory runs. The server does not consult environment variables at request time.

## Provider metadata endpoints

The server exposes provider metadata and model lookup routes. Model metadata is fetched from `models.dev` and cached.

- `GET /provider`
- `GET /provider/{name}`
- `GET /provider/{name}/models`
- `GET /provider/{name}/models/{model}`
