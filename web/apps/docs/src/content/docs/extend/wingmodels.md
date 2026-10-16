---
title: "WingModels"
description: "Call model providers directly from Go."
---

# WingModels

WingModels is Wingman's Go library for model calls. It uses one request and message format across providers.
Use the [HTTP SDK](/build-clients/go-sdk) instead if your application connects to a running Wingman server.

## Call a model

Import the provider package that your application uses. The import registers its authentication and request behavior.
This example reads `OPENAI_API_KEY` from the application environment:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/chaserensberger/wingman/models"
	provider "github.com/chaserensberger/wingman/models/providers"
	"github.com/chaserensberger/wingman/models/providers/openai"
)

func main() {
	client := provider.NewClient(nil)
	message, err := client.Generate(context.Background(), models.Request{
		Model: openai.Model("gpt-5.6-terra"),
		Messages: []models.Message{{
			Role: models.RoleUser,
			Content: models.Content{models.TextPart{Text: "Explain what an HTTP API does."}},
		}},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", message)
}
```

If model references come from configuration, use a blank provider import, such as `import _ "github.com/chaserensberger/wingman/models/providers/openai"`.
Catalog lookup alone does not register provider behavior. In particular, OAuth requests require the provider import.
The standard Wingman server already imports its built-in providers.

## Request methods

| Method on `models.Client` | Purpose                                                     |
| ------------------------- | ----------------------------------------------------------- |
| `Prepare(ctx, request)`   | Build the provider request without sending it.              |
| `Stream(ctx, request)`    | Send the request and return a stream of response parts.     |
| `Generate(ctx, request)`  | Read the stream and return the assembled assistant message. |

Messages contain text, images, reasoning, tool calls, and tool results.
The library converts these parts to the selected provider's format.
See [Models](/configure/models) for model references, variants, and supported protocols.

`Request.ProviderOptions` adds provider-native body fields under the active provider ID.
`Request.HTTP.Body` supplies final body overrides. Required provider values still take precedence, such as `store: false` for Codex OAuth.
Request headers override variant headers, except required provider headers. Header names ignore case.
Request query values override route query values.

## Streams and failures

A successful stream starts with `StreamStartPart` and ends with `FinishPart`.
Intermediate parts contain text, reasoning, tool input, or response metadata.
`FinishPart` includes usage, the finish reason, and the assembled message.

Read the stream to its end, then read `EventStream.Final()` for the message and error.
On failure, it returns any partial message. A failed stream does not emit `FinishPart`.
Cancellation can prevent an `ErrorPart` event. If you stop reading, cancel the request context.

A closed connection or HTTP 200 status does not prove success.
Read the finish reason even when the response succeeds. A model can stop at a token limit without producing a complete answer.

`models.ProviderError` preserves the underlying cause, category, HTTP status, provider request ID, and retry information.
The agent loop makes up to three attempts for eligible failures before a stream starts. Direct model calls do not add that retry loop.
Once a stream starts, the agent loop does not retry a failure automatically during normal execution.

Embedded sessions can use `session.WithRetryPolicy`. Set `MaxAttempts` to `1` to disable retries.
See [Observability](/use-wingman/observability) for failure categories and diagnostics.
