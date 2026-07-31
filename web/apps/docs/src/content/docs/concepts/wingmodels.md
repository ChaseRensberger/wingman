---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is Wingman's provider-agnostic model SDK. It gives the agent runtime common request, message, and stream formats while keeping provider wire formats behind the model client.

## Supported Providers

WingModels currently includes catalog entries for:

- Anthropic
- DeepSeek
- Gemini
- OpenAI
- OpenCode Zen
- OpenCode Go
- OpenRouter

Custom routes may target endpoints that speak one of Wingman's supported protocols.

## Why It Exists

The agent runtime uses WingModels for:

- A common conversation format for storage and replay.
- A common stream format for UI, plugins, and HTTP events.
- Provider-specific request lowering and SSE parsing behind a single model client.
- Local model metadata without depending on a hosted metadata service.
- Model refs that can change per message without binding a session to one provider.

## Runtime API

The loop talks to a `models.Client`:

```go
type Client interface {
    Prepare(context.Context, Request) (*PreparedRequest, error)
    Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
    Generate(context.Context, Request) (*Message, error)
}
```

`Prepare` lowers a WingModels request into provider-native JSON without sending it. `Stream` sends the request and returns normalized stream parts. `Generate` drains the stream and returns the final assembled assistant message.

Requests carry a provider-qualified model ref:

```text
provider/model
```

Examples:

```text
anthropic/claude-sonnet-5
openai/gpt-5.6-terra
google/gemini-3.6-flash
openrouter/moonshotai/kimi-k2.7-code
deepseek/deepseek-v4-pro
opencode/claude-sonnet-5
opencode-go/kimi-k3
```

## Provider-Neutral Messages

WingModels stores conversation content as provider-neutral messages with typed parts:

- Text
- Image
- Reasoning
- Tool call
- Tool result
- Plugin-defined opaque content

Providers lower this common format into their native wire formats at request time. This lets the store, HTTP API, UI, and plugins work with one content model instead of provider-specific payloads.

## Streaming

Every provider emits normalized `models.StreamPart` values. The current lifecycle is:

```text
StreamStartPart
(TextStartPart | TextDeltaPart | TextEndPart | ToolInputStartPart | ToolInputDeltaPart | ToolInputEndPart | ToolCallPart_ | ResponseMetadataPart | ErrorPart)*
FinishPart
```

`FinishPart` carries usage, finish reason, and the final assembled assistant message. Consumers can also call `EventStream.Final()` after draining the stream.

Provider-backed streams bind their producer to the request context. Cancellation
unblocks a producer even when the consumer stopped draining a full stream.
Malformed tool arguments fail with a decoding error instead of becoming an empty
object. Parallel OpenAI-compatible tool calls retain provider index order.

## Provider Errors And Retries

WingModels returns `models.ProviderError` for provider and transport failures.
The error preserves its underlying cause and classifies the failure as one of:

```text
authentication
authorization
rate_limit
invalid_request
unavailable
timeout
transport
provider
decoding
cancellation
```

It also carries safe status, provider request ID, retryability, and optional
`Retry-After` data. Provider response bodies are not included in public error
messages.

The agent loop retries retryable dispatch failures up to three physical attempts
by default. It uses cancellation-aware exponential backoff and honors
`Retry-After`. Every attempt receives a separate durable model-call record.
Failures after a stream is established are never retried.

Embedded Go callers can configure this behavior with
`session.WithRetryPolicy`. Set `MaxAttempts` to `1` to disable retries.

## Request Options

`Request.ProviderOptions` supplies top-level provider-native body fields keyed by
the active provider ID. `Request.HTTP.Body` is the final override for advanced
callers. Per-request HTTP query values override configured route query values
without mutating process configuration.

## Catalog

The embedded catalog provides model identity, lab metadata, provider defaults, and executable route metadata. Catalog membership is not an execution gate: callers can still provide explicit route metadata for custom models.

At daemon startup, Wingman copies the embedded catalog and applies configured
provider routes and models to a new immutable generation. The source catalog is
not changed. An invalid candidate fails before Wingman opens storage or starts
plugin and MCP processes.

Catalog files live under:

```text
models/catalog/
  labs/<lab-id>/
    lab.toml
    logo.svg
  models/<namespace>/<model-id>.toml
  providers/<provider-id>/
    provider.toml
    models/<route-id>.toml
```

### Labs

A lab is the organization that develops a canonical model. Labs provide a display name, short description, optional website, and optional SVG logo.

```toml
# models/catalog/labs/anthropic/lab.toml
name = "Anthropic"
description = "Developer of Claude models for reliable, steerable agent work."
website = "https://anthropic.com"
```

### Canonical models

A canonical model describes the underlying model independently of the provider that serves it. It references its developing lab and carries display metadata.

```toml
# models/catalog/models/anthropic/claude-sonnet-5.toml
lab = "anthropic"
name = "Claude Sonnet 5"
description = "Anthropic's balanced Claude model for coding and analysis."
release_date = "2026-07"
last_updated = "2026-07"
```

The canonical model ID is derived from its path: this file is `anthropic/claude-sonnet-5`.

### Provider routes

A provider route describes how Wingman reaches and uses one model through one provider. It contains the runtime metadata needed for request lowering: API protocol, endpoint defaults, authentication environment variables, limits, cost, and capability flags.

```toml
# models/catalog/providers/openrouter/models/claude-sonnet-5.toml
id = "anthropic/claude-sonnet-5"
provider = "openrouter"
base_model = "anthropic/claude-sonnet-5"
api = "openai_compatible_chat"
context_window = 1000000
max_output = 128000
input_cost_per_mtok = 2
output_cost_per_mtok = 10

[capabilities]
tools = true
images = true
reasoning = true
structured_output = true
```

`base_model` links a route to its canonical model. This lets clients show every available route for a model without treating a reseller as the model's lab. It is a validated relationship, not inheritance: route fields remain explicit because they are the runtime source of truth.

Provider defaults apply to every route unless the route overrides them:

```toml
# models/catalog/providers/openrouter/provider.toml
name = "OpenRouter"
doc = "https://openrouter.ai/models"
base_url = "https://openrouter.ai/api/v1"
env = ["OPENROUTER_API_KEY"]
```

### Catalog API

`GET /catalog` returns all labs, canonical models, providers, and routes. `GET /catalog/labs/{id}/logo` returns an embedded lab logo when one exists.

## Provider Route Overlays

Wingman's config can overlay catalog provider routes for the running daemon. The overlay is process configuration, not persisted model metadata.

```json
{
  "provider": {
    "openai": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      }
    }
  }
}
```

This keeps persisted agents simple: `model_ref` remains `openai/gpt-5.6-terra`, while the daemon decides where OpenAI requests are routed.

See [Providers](/configure/providers) for auth and gateway details.

## Custom Models

Use explicit route metadata when the catalog does not know a model or when an agent/request needs a custom endpoint.

HTTP agents use `model_route`:

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

If `model_ref` is in the catalog, the catalog wins. Use `model_route` for uncataloged models and explicit custom deployments.

## Supported Protocols

Custom routes must use one of Wingman's supported protocols:

```text
openai_responses
openai_completions
openai_compatible_chat
anthropic_messages
gemini_generate
```

Choose the protocol that matches the endpoint.
