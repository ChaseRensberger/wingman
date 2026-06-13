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
- OpenAI
- OpenCode Zen

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
anthropic/claude-sonnet-4-6
openai/gpt-5.5
opencode/claude-sonnet-4-6
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

## Catalog

The embedded catalog provides provider defaults, model metadata, and capability flags.

Catalog files live under:

```text
models/catalog/providers
```

Provider entries define defaults such as `base_url` and environment variable names. Model entries define fields such as protocol, context window, max output, and capability flags.

The catalog is not the only way to call a model. Callers can provide explicit route metadata for custom models.

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

This keeps persisted agents simple: `model_ref` remains `openai/gpt-5.5`, while the daemon decides where OpenAI requests are routed.

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
anthropic_messages
```

Choose the protocol that matches the endpoint.
