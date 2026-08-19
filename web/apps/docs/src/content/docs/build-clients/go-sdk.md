---
title: "Go SDK"
description: "Use the generated Go client with a Wingman daemon."
---

# Go SDK

The Go SDK provides typed REST methods, local daemon discovery, and typed
Wingman event streams.

See the [Go Client API](/reference/go-client-api/) for the complete public
method index.

## Install

Install the SDK version that matches the Wingman daemon:

```bash
go get github.com/chaserensberger/wingman/client@v0.1.41
```

## Connect

### Local Managed Daemon

If the application runs as the daemon user, use `NewLocal`. It reads the
private daemon registration from the XDG state directory. On Linux, it reads
the managed-service state directory too. It accepts only a loopback origin.

```go
wingman, err := client.NewLocal(context.Background())
if err != nil {
	return err
}
```

Use `NewLocal` only in a local application that can read daemon state. It does
not connect to a remote daemon.

### Explicit Server

For a foreground or remote server with a known URL, use `WithBasicAuth`:

```go
wingman, err := client.New(
	"https://wingman.example",
	client.WithBasicAuth("wingman", os.Getenv("WINGMAN_PASSWORD")),
	client.WithClientID("cli_example"),
)
if err != nil {
	return err
}
```

Set `WINGMAN_PASSWORD` from that server's credentials before you run the
application. `WINGMAN_USERNAME` defaults to `wingman` on the server. Before you
send Basic Auth credentials to a remote server, use TLS or an SSH tunnel. Read
[Authentication](/concepts/authentication) for credential and security details.

## Client Identity

At application startup, call `EnsureClient`. It creates the client identity on
the first start. On later starts, it reads and compares the existing identity.

```go
bootstrap, err := client.NewLocal(ctx)
if err != nil {
	return err
}
_, err = bootstrap.EnsureClient(ctx, "cli_wingcode", "Wingcode")
if err != nil {
	return err
}

wingman, err := client.NewLocal(ctx, client.WithClientID("cli_wingcode"))
if err != nil {
	return err
}
```

If the existing client has a different name, `EnsureClient` returns an error.
`WithClientID` sets the default `X-Wingman-Client` header. A request header
overrides this value.

## REST Requests

Generated methods with a `WithResponse` suffix decode successful JSON responses
to typed fields:

```go
ready, err := wingman.GetReadinessWithResponse(context.Background(), nil)
if err != nil {
	return err
}
fmt.Println(ready.JSON200.Version)
```

## Admit Messages Safely

Persistent message admission is idempotent when each retry uses the same
`request_id`. `NewMessageAdmission` adds an ID if the request has none.

Save the returned request before you send the first network request:

```go
request := client.NewMessageAdmission(client.MessageSessionRequest{
	AgentId: "agt_assistant",
	Message: "Summarize this project.",
})
savePendingRequest(sessionID, request)

admission, err := wingman.AdmitMessage(ctx, sessionID, request)
if err != nil {
	return err
}
deletePendingRequest(sessionID, request)
fmt.Println(admission.RunId)
```

If the request result is unknown, run `AdmitMessage` again with the saved
request. Do not create a new request ID for this retry.

## One-Shot Streams

`Run` starts an ephemeral run. It does not create a session or save a
transcript. The returned stream ends after a terminal event.

```go
modelRef := "openai/gpt-5.6-terra"
stream, err := wingman.Run(ctx, client.RunRequest{
	ModelRef: &modelRef,
	Message:  "Summarize this project.",
})
if err != nil {
	return err
}
defer stream.Close()

for stream.Next() {
	event := stream.Event()
	_ = event
}
if err := stream.Err(); err != nil {
	return err
}
```

## Persistent Session Streams

`StreamSessionEvents` opens one SSE connection for a persistent session. The
SDK parses frames and typed event envelopes. The application saves cursors,
reloads state, and reconnects.

Set `LastEventID` to send the saved cursor in the `Last-Event-ID` header. If
both `After` and `LastEventID` are set, `After` takes precedence.

```go
after := loadLastSequence(sessionID)
stream, err := wingman.StreamSessionEvents(ctx, sessionID, &client.SessionEventsOptions{
	After: &after,
})
if err != nil {
	return err
}
defer stream.Close()

for stream.Next() {
	event := stream.Event()
	if event.Cursor != nil && event.Cursor.Seq > after {
		after = event.Cursor.Seq
		saveLastSequence(sessionID, after)
	}
	if event.Type == api.SessionEventEventsResyncRequired {
		reloadSessionAndRun(sessionID)
		break
	}
	applyEvent(event)
}
if err := stream.Err(); err != nil {
	return err
}
```

For raw SSE `id`, `event`, or `data` fields, use `stream.Frame()`. Read
[Streaming Events](/build-clients/streaming-events) for the event recovery contract.

## Handle Errors

Non-success responses return `*client.APIError`:

```go
var apiError *client.APIError
if errors.As(err, &apiError) {
	fmt.Println(apiError.StatusCode)
	fmt.Println(apiError.RequestID)
	fmt.Println(apiError.RetryAfter)
}
```

`APIError` includes response headers, the error payload, the request ID, and
the parsed `Retry-After` value. Use this data for diagnostics and retry logic.

## Version Compatibility

The SDK is generated from the daemon OpenAPI contract. Until the API is stable,
use the SDK version with the same tag as the daemon.
