---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is Wingman's provider-agnostic model SDK. It gives the agent runtime
common request, message, and stream formats. The model client hides provider
wire formats.

## Supported Providers

WingModels includes catalog entries for:

- Anthropic
- DeepSeek
- Gemini
- OpenAI
- OpenCode Zen
- OpenCode Go
- OpenRouter

Custom routes can target endpoints that use one of Wingman's supported protocols.

## Runtime API

The loop uses a `models.Client`:

```go
type Client interface {
    Prepare(context.Context, Request) (*PreparedRequest, error)
    Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
    Generate(context.Context, Request) (*Message, error)
}
```

`Prepare` lowers a WingModels request into provider-native JSON without sending
it. `Stream` sends the request and returns normalized stream parts. `Generate`
drains the stream and returns the final assembled assistant message.

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

Providers lower this common format into their native wire formats at request
time. This lets the store, HTTP API, UI, and plugins use one content model
instead of provider-specific payloads.

## Streaming

Every provider emits normalized `models.StreamPart` values. The current lifecycle is:

```text
StreamStartPart
(TextStartPart | TextDeltaPart | TextEndPart | ToolInputStartPart | ToolInputDeltaPart | ToolInputEndPart | ToolCallPart_ | ResponseMetadataPart | ErrorPart)*
FinishPart
```

`FinishPart` carries usage, finish reason, and the final assembled assistant
message. Consumers can also call `EventStream.Final()` after they drain the
stream.

Provider-backed streams bind their producer to the request context. Cancellation
unblocks a producer if the consumer stops draining a full stream. Malformed tool
arguments fail with a decoding error instead of becoming an empty object.
Parallel OpenAI-compatible tool calls retain provider index order.

## Provider Errors And Retries

WingModels returns `models.ProviderError` for provider and transport failures.
The error preserves its underlying cause. It classifies the failure as one of:

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
by default. It uses cancellation-aware exponential backoff. It honors
`Retry-After`. Every attempt receives a separate durable model-call record.
WingModels never retries failures after a stream is established.

Embedded Go callers can configure this behavior with
`session.WithRetryPolicy`. Set `MaxAttempts` to `1` to disable retries.

## Request Options

`Request.ProviderOptions` supplies top-level provider-native body fields keyed
by the active provider ID. `Request.HTTP.Body` is the final override for
advanced callers. Per-request HTTP query values override configured route query
values without changing process configuration.

## Provider Route Overlays

Wingman's configuration can override catalog provider routes for the running
daemon.

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

The agent's `model_ref` remains `openai/gpt-5.6-terra`. The daemon routes
OpenAI requests to the configured endpoint.

See [Providers](/configure/providers) for auth and gateway details.

## Custom Models

Use explicit route metadata if the catalog does not know a model. Also use it
if an agent/request needs a custom endpoint.

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

If `model_ref` is in the catalog, the catalog wins. Use `model_route` for
uncataloged models and explicit custom deployments.

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
