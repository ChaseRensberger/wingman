---
title: "Providers"
group: "Primitives"
order: 100
---

# Providers

A provider is an interface that translates between a model provider's API and Wingman's internal types. Instead of relying on a third-party SDK, Wingman defines provider translation internally. The tradeoff is that Wingman won't have the breadth of [models.dev](https://models.dev), but its core dependencies stay minimal.

Currently supported: **Anthropic**, **Ollama**

## Provider Interface

```go
type Provider interface {
    RunInference(ctx context.Context, req WingmanInferenceRequest) (*WingmanInferenceResponse, error)
    StreamInference(ctx context.Context, req WingmanInferenceRequest) (Stream, error)
}
```

Each provider package exports its own `Config` struct with provider-specific fields, giving you full type safety when using the SDK. See [Architecture](/docs/architecture) for more on the design rationale.

## SDK Usage

```go
import "wingman/provider/anthropic"

p := anthropic.New(anthropic.Config{
    Model:     "claude-sonnet-4-5",
    MaxTokens: 4096,
})
```

```go
import "wingman/provider/ollama"

p := ollama.New(ollama.Config{
    Model:   "llama3.2",
    BaseURL: "http://localhost:11434",
})
```

The provider is then passed to an [agent](/docs/agents) via `agent.WithProvider(p)`.

## Server Usage

On the server, provider configuration lives on the agent as a JSON object and credentials are managed separately.

### Discovery

```
GET    /provider                    # List all providers
GET    /provider/{name}             # Get provider info
GET    /provider/{name}/models      # List available models
GET    /provider/{name}/models/{id} # Get model details
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

When creating an agent via the API, the `provider` field specifies which provider to use and how to configure inference. See [Agents — Server Usage](/docs/agents) for the full agent creation payload.
