---
title: "Go SDK"
description: "Use the generated Go client with a Wingman daemon."
---

# Go SDK

Use the Go SDK to call Wingman's HTTP API and read event streams.

See the [Go Client API](/reference/go-client-api/) for the complete public
method index.

## Install

Install the SDK version that matches the Wingman daemon:

```bash
go get github.com/chaserensberger/wingman/client@v0.1.60
```

## Connect

### Local Managed Daemon

Use `NewLocal` when the application runs on the same machine and as the same user as the managed service.
It reads the service URL and credentials automatically and accepts only a local loopback address.

```go
wingman, err := client.NewLocal(context.Background())
if err != nil {
	return err
}
```

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

Call `EnsureClient` at startup to create or reuse a client identity:

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

Retrying the same input with the same `request_id` does not create another run.
`NewMessageAdmission` adds an ID if the request has none.

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

`Run` executes without saving a conversation. Its stream ends on completion or failure.

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

`StreamSessionEvents` reads server-sent events (SSE) for a saved session.
Your application must save the last event sequence, reload state, and reconnect after a disconnect.

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

Use the SDK version that matches the server release.
