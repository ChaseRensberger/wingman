---
title: "WingModels"
group: "Core"
order: 103
---

# WingModels

WingModels is Wingman's Go library for calling model providers through one request and response format.

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

Use `models.Client` to call a model:

```go
type Client interface {
    Prepare(context.Context, Request) (*PreparedRequest, error)
    Stream(context.Context, Request) (*EventStream[StreamPart, *Message], error)
    Generate(context.Context, Request) (*Message, error)
}
```

`Prepare` builds provider JSON without sending it. `Stream` sends the request and returns response parts.
`Generate` reads the stream and returns the complete assistant message.

A model reference names the provider and model:

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

In an embedded Go application, import each provider package you use. This registers its authentication and routing behavior.

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

Catalog lookup alone does not register a provider. OAuth requests fail without the provider import.
API-key routes can use protocol defaults.

The Wingman daemon already registers its built-in providers. HTTP clients do not need these Go imports.

## Provider-Neutral Messages

Messages contain parts that use the same format across providers:

- Text
- Image
- Reasoning
- Tool call
- Tool result
- Plugin-defined opaque content

WingModels converts these parts to the provider's format when it sends a request.

## Streaming

Every provider returns `models.StreamPart` values in this order:

```text
StreamStartPart
  text, reasoning, tool, and response metadata parts (zero or more)
  ├─ completion: FinishPart
  └─ failure:    ErrorPart
```

`FinishPart` carries usage, a finish reason, and the final assembled assistant message. A failed stream does not emit `FinishPart`.

Read the stream to its end, then check `EventStream.Final()` for an error.
On failure, it returns the partial message and error. Cancellation can prevent an `ErrorPart` event.
If you stop reading, cancel the request context.

A closed connection alone does not prove completion. Interrupted streams fail even if they contain output.
Successful responses can contain no text. Check the finish reason for token limits or content restrictions.

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

The error includes status, provider request ID, retry eligibility, and optional `Retry-After` data.
Public error messages omit provider response bodies.

HTTP 200 does not prove success. A provider can report an error inside the stream.
See [Observability](/use-wingman/observability) for captured failure details.

Quota and content-policy failures are not retryable. Unknown provider failures are retry-eligible before an established stream.
Additional classifications identify context overflow, oversized payloads, and incomplete streams.

The agent loop makes up to three attempts for eligible failures before a stream starts.
It increases the wait between attempts and honors `Retry-After` and `Retry-After-Ms`.
Each attempt has its own model-call record. Failures after a stream starts are not retried.

Embedded Go callers can configure this behavior with `session.WithRetryPolicy`. Set `MaxAttempts` to `1` to disable retries.

## Request Options

`Request.ProviderOptions` supplies top-level provider-native body fields for the active provider ID. `Request.HTTP.Body` is the final caller override. Required deployment values still apply. For example, Codex OAuth always sends `store: false`.

HTTP header names are case-insensitive. Request headers override variant headers, but required deployment headers take precedence. Per-request HTTP query values override configured route query values without changing process configuration.

## Provider Route Overlays

Change a provider's destination in `wingman.json` without changing agent model references.
See [Providers](/configure/providers#route-a-provider-through-a-gateway) for a gateway example.

## Custom Models

For a model outside the catalog, provide `model_route` on an agent or request.
Catalog entries take precedence over `model_route`.
See [Custom Model Routes](/configure/models#custom-model-routes) for examples.

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
