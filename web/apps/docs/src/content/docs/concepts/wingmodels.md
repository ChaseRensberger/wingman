---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is the provider-agnostic model SDK for Wingman. It gives the agent runtime common request, message, and stream formats. The model client hides provider wire formats.

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

`Prepare` converts a WingModels request to provider-native JSON without sending it. `Stream` sends the request and returns normalized stream parts. `Generate` drains the stream and returns the final assembled assistant message.

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

### Provider Imports In Go

For embedded Go use, import the provider package for each provider-specific deployment you use. The import registers its authentication and routing behavior.

For example, the OpenAI model helper registers the OpenAI deployment, including Codex OAuth:

```go
import (
    "github.com/chaserensberger/wingman/models/providers"
    "github.com/chaserensberger/wingman/models/providers/openai"
)

client := provider.NewClient(nil)
model := openai.Model("gpt-6-luna")
```

If your application constructs model references from configuration, use a blank import instead:

```go
import _ "github.com/chaserensberger/wingman/models/providers/openai"
```

Catalog lookup does not register a provider deployment. Without its registered deployment, an OAuth request fails before dispatch instead of using an API-key endpoint. API-key routes can use the supported protocol defaults.

The Wingman daemon already registers its built-in providers. HTTP clients do not need these Go imports.

## Provider-Neutral Messages

WingModels stores conversation content as provider-neutral messages with typed parts:

- Text
- Image
- Reasoning
- Tool call
- Tool result
- Plugin-defined opaque content

Providers convert this common format to native wire formats at request time. This lets the store, HTTP API, UI, and plugins use one content model instead of provider-specific payloads.

## Streaming

Every provider emits normalized `models.StreamPart` values. After `Stream` returns a stream, its lifecycle is:

```text
StreamStartPart
  text, reasoning, tool, and response metadata parts (zero or more)
  ├─ completion: FinishPart
  └─ failure:    ErrorPart
```

`FinishPart` carries usage, a finish reason, and the final assembled assistant message. A failed stream does not emit `FinishPart`.

Drain the stream, then check the error from `EventStream.Final()` before you treat the response as complete. `Final()` returns the partial message and the error when a stream fails. Cancellation can prevent delivery of `ErrorPart`, so that event is not a substitute for the final error.

A closed provider connection does not establish completion. WingModels requires completion evidence from the provider protocol. An interrupted stream fails even when it contains partial output. A completed response can contain no text. Token limits and content restrictions appear in the finish reason.

Provider-backed streams bind their producer to the request context. If the consumer stops draining a full stream, cancellation unblocks the producer. Malformed tool arguments fail with a decoding error instead of becoming an empty object. Parallel OpenAI-compatible tool calls retain provider index order.

## Provider Errors And Retries

WingModels returns `models.ProviderError` for provider and transport failures. The error preserves its underlying cause for Go callers. It classifies the failure as one of:

```text
authentication
authorization
rate_limit
quota
content_policy
invalid_request
unavailable
timeout
transport
provider
decoding
cancellation
```

It also carries safe status, provider request ID, retryability, and optional `Retry-After` data. Provider response bodies are not included in public error messages.

HTTP 200 means that the provider accepted the streaming connection, not that generation succeeded.
A provider can report a failure inside that stream.
Model-call records retain bounded native failure evidence with known credentials redacted.
The Console inspector can copy this evidence, including provider messages, bodies, response headers, and native causes.
Read [Observability](/use-wingman/observability) for diagnostic fields and capture limits.

Quota and content-policy failures are not retryable. Unknown provider failures are retry-eligible before an established stream.
Additional classifications identify context overflow, oversized payloads, and incomplete streams.

The agent loop retries retryable dispatch failures up to three physical attempts by default. It uses cancellation-aware exponential backoff. It honors `Retry-After` and `Retry-After-Ms`. Every attempt receives a separate durable model-call record. WingModels never retries failures after a stream is established.

Embedded Go callers can configure this behavior with `session.WithRetryPolicy`. Set `MaxAttempts` to `1` to disable retries.

## Request Options

`Request.ProviderOptions` supplies top-level provider-native body fields for the active provider ID. `Request.HTTP.Body` is the final caller override. Required deployment values still apply. For example, Codex OAuth always sends `store: false`.

HTTP header names are case-insensitive. Request headers override variant headers, but required deployment headers take precedence. Per-request HTTP query values override configured route query values without changing process configuration.

## Provider Route Overlays

Wingman configuration can override catalog provider routes for the running daemon.

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

The agent `model_ref` remains `openai/gpt-5.6-terra`. The daemon routes OpenAI requests to the configured endpoint.

See [Providers](/configure/providers) for auth and gateway details.

## Custom Models

Use explicit route metadata if the catalog does not know a model. Also use it if an agent/request needs a custom endpoint.

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
