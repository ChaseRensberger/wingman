---
title: "Providers"
group: "Primitives"
order: 100
---

# Providers

Providers are just an interface so that it's easy to translate between a model provider's specific typing and the typing Wingman uses. If you read the *Introduction*, this project was largely inspired by OpenCode's server. Instead of using Vercel's AI SDK, I've opted to define provider translation within Wingman. The con of this pattern (assuming it doesn't change) is that Wingman will likely never have the comprehensive support of the models you'll find on [models.dev](https://models.dev), the pro is that Wingman's core dependencies are pretty limited.

## SDK

In the SDK, a provider is a typed instance that knows how to connect to a specific API and how to configure inference. Each provider package exports its own `Config` struct with provider-specific fields.

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

The provider is then attached to an agent:

```go
a := agent.New("MyAgent",
    agent.WithProvider(p),
    agent.WithInstructions("..."),
)
```

## Server

On the server side, the provider configuration lives on the agent as a JSON object. Auth credentials are managed separately.

### Provider Discovery

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

When creating an agent via the API, use `provider_id` to specify which provider to use and `provider_options` to configure inference. `provider_options` is a free-form map — use any keys supported by the provider.

**Common options (all providers):**

| Key | Type | Description |
|-----|------|-------------|
| `model` | string | Model ID to use |
| `max_tokens` | number | Maximum tokens to generate |
| `temperature` | number | Sampling temperature |

**Provider-specific options:**

| Key | Provider | Description |
|-----|----------|-------------|
| `base_url` | ollama | Custom Ollama server URL (default: `http://localhost:11434`) |
| `api_key` | anthropic | Override the API key set in auth (optional) |

```bash
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Assistant",
    "instructions": "Be helpful",
    "provider_id": "anthropic",
    "provider_options": {
      "model": "claude-sonnet-4-5",
      "max_tokens": 4096,
      "temperature": 0.7
    }
  }'
```

```bash
# Ollama with a custom server URL
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "LocalAgent",
    "instructions": "Be helpful",
    "provider_id": "ollama",
    "provider_options": {
      "model": "llama3.2",
      "base_url": "http://my-ollama-host:11434"
    }
  }'
```

The server looks up the API key from the auth store and constructs the provider instance at inference time.
