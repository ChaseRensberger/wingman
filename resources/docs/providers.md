---
title: "Providers"
group: "Primitives"
draft: true
order: 100
---

# Providers

Providers translate Wingman's provider-agnostic request/response types into the wire format expected by a specific model API. Each provider package implements `core.Provider` and is registered in the provider registry.

## SDK

In the SDK, a provider is a typed instance that knows how to connect to a specific API and how to configure inference. Each provider package exports a `Config` struct with a provider-specific field for auth/connection and a generic `Options map[string]any` for inference parameters.

```go
import "github.com/chaserensberger/wingman/provider/anthropic"

p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})
```

```go
import "github.com/chaserensberger/wingman/provider/ollama"

p, err := ollama.New(ollama.Config{
    Options: map[string]any{
        "model":    "llama3.2",
        "base_url": "http://localhost:11434",
    },
})
```

You can also use the registry factory, which is the same path the server uses:

```go
import _ "github.com/chaserensberger/wingman/provider/anthropic"
import "github.com/chaserensberger/wingman/provider"

p, err := provider.New("anthropic", map[string]any{
    "model":      "claude-opus-4-6",
    "max_tokens": 4096,
    "api_key":    os.Getenv("ANTHROPIC_API_KEY"),
})
```

The provider is then attached to an agent:

```go
a := agent.New("MyAgent",
    agent.WithProvider(p),
    agent.WithInstructions("..."),
)
```

## Server

On the server side, provider configuration lives on the agent as separate `provider` and `model` fields plus a free-form `options` map. Auth credentials are managed in SQLite and injected at inference time.

The server does not read credentials from environment variables; only the SQLite auth store is used.

### Provider Discovery

```
GET    /provider                    # List all providers
GET    /provider/{id}               # Get provider info
GET    /provider/{id}/models        # List available models (from models.dev, cached 1hr)
GET    /provider/{id}/models/{model}# Get model details
```

### Auth Management

```
GET    /provider/auth               # Check auth status
PUT    /provider/auth               # Set provider credentials
DELETE /provider/auth/{provider}    # Remove provider credentials
```

```bash
curl -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'
```

### Provider Config on Agents

Agents reference a provider via separate `provider` and `model` fields. The server reads credentials from SQLite and injects them into the provider factory.

See [Agents](./agents) for request and response examples.
