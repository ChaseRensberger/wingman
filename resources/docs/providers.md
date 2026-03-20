---
title: "Providers"
group: "Concepts"
draft: false
order: 100
---

# Providers

Providers are Wingman's abstraction over model backends. A provider turns a generic `core.InferenceRequest` into the wire format expected by a specific API and returns a generic `core.InferenceResponse` back to the runtime.

## Interface

Every provider implements the same two methods:

```go
type Provider interface {
    RunInference(ctx context.Context, req core.InferenceRequest) (*core.InferenceResponse, error)
    StreamInference(ctx context.Context, req core.InferenceRequest) (core.Stream, error)
}
```

This lets the rest of Wingman stay provider-agnostic.

## Provider and model are separate

Wingman stores `provider` and `model` as separate fields rather than a combined string. That distinction matters because the same model family can be exposed through different providers with different auth, limits, or capabilities.

In practice, an agent definition looks like this:

```json
{
  "provider": "anthropic",
  "model": "claude-opus-4-6",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.2
  }
}
```

## SDK usage

Most SDK users construct providers directly from provider packages.

```go
p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})
```

```go
p, err := ollama.New(ollama.Config{
    Options: map[string]any{
        "model":    "llama3.2",
        "base_url": "http://localhost:11434",
    },
})
```

You can also use the registry factory path:

```go
import _ "github.com/chaserensberger/wingman/provider/anthropic"
import "github.com/chaserensberger/wingman/provider"

p, err := provider.New("anthropic", map[string]any{
    "model":      "claude-opus-4-6",
    "max_tokens": 4096,
    "api_key":    "sk-...",
})
```

## The `options` map

Inference settings flow through a free-form `options` map. This keeps provider-specific parameters out of Wingman's core type system.

Common keys include:

| Key | Anthropic | Ollama |
|---|---|---|
| `model` | required by provider config | required by provider config |
| `max_tokens` | supported | maps to `num_predict` |
| `temperature` | supported | supported |
| `api_key` | SDK convenience | not used |
| `base_url` | not used | supported |

## Server behavior

On the server, providers are rebuilt at request time from the agent's `provider`, `model`, and `options` fields. Credentials are loaded from the SQLite auth store and injected before the provider factory is called.

The server does not read provider credentials from environment variables.

## Provider metadata endpoints

The server exposes provider metadata and model lookup routes:

- `GET /provider`
- `GET /provider/{id}`
- `GET /provider/{id}/models`
- `GET /provider/{id}/models/{model}`

Model metadata is fetched from `models.dev` and cached by the server. That lookup path is useful for discovery, but it is separate from the core inference runtime.
