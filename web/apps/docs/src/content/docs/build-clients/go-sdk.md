---
title: "Go SDK"
description: "Use the generated Go client with a Wingman daemon."
---

# Go SDK

The Go SDK provides typed REST methods. It also provides local daemon discovery
and helpers for Wingman SSE streams.

See the [Go Client API](/reference/go-client-api/) for the complete public
method index.

Install the SDK version with the same tag as the Wingman daemon:

```bash
go get github.com/chaserensberger/wingman/client@v0.1.41
```

## Connect to a Local Daemon

If the application runs as the daemon user, use `NewLocal`. It reads the
private daemon registration from the XDG state directory. On Linux, it also
checks the managed service state directory. It accepts only a loopback origin.
It makes sure that the daemon is ready before it returns a client.

```go
wingman, err := client.NewLocal(context.Background())
if err != nil {
	return err
}
```

Use `NewLocal` only in a local application that can read the daemon state. It
does not connect to a remote daemon.

If the daemon has a known URL, use `WithPassword`:

```go
wingman, err := client.New(
	"https://wingman.example",
	client.WithPassword(os.Getenv("WINGMAN_DAEMON_PASSWORD")),
	client.WithClientID("cli_example"),
)
if err != nil {
	return err
}
```

Before you send a daemon password to a remote server, use TLS or an SSH tunnel.

## Call REST Endpoints

Generated methods with a `WithResponse` suffix decode successful JSON responses
into typed fields:

```go
ready, err := wingman.GetReadinessWithResponse(context.Background(), nil)
if err != nil {
	return err
}
fmt.Println(ready.JSON200.Version)
```

If requests use a registered client identity, set `WithClientID`. A
per-request `X-Wingman-Client` header overrides the SDK default.

## Set Up a Client Identity

At application startup, call `EnsureClient`. It creates the client identity at
the first start. On later starts, it gets and compares the existing identity.

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

## Admit Messages Safely

Persistent message admission is idempotent if each retry uses the same
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

## Consume Session Events

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

If you need the raw SSE `id`, `event`, or `data` fields, use `stream.Frame()`.
Read [Streaming Events](/build-clients/streaming-events) for the event recovery
contract.

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

`APIError` keeps the response headers, error payload, request ID, and parsed
`Retry-After` value. Use this data for diagnostics and retry decisions.

## Version Compatibility

The SDK is generated from the daemon OpenAPI contract. Until the API is stable,
use the SDK version with the same tag as the daemon.
