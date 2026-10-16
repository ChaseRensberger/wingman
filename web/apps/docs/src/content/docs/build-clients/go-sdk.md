---
title: "Go SDK"
description: "Use the generated Go client with a Wingman daemon."
---

# Go SDK

The Go SDK is a library for calling Wingman's HTTP API. It also reads live event streams.
Import `github.com/chaserensberger/wingman/client` as `client` in your application.

See [Go Client API](/reference/go-client-api/) for connection options and method conventions.

## Install

Read the server version with `wingman version`. Replace `VERSION` below with the same version, including its leading `v`:

```bash
go get github.com/chaserensberger/wingman/client@VERSION
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

## Submit messages safely

Retrying the same input with the same `request_id` does not create another run.
`NewMessageAdmission` adds an ID if the request has none.

Save the returned request before you send the first network request:

The examples below use application-defined functions such as `savePendingRequest` and `applyEvent`.
Implement these functions in your client. Replace example agent and session IDs with existing IDs.

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

`Run` executes without saving a conversation. Supply an inline agent or an existing `agent_id`.
Its stream ends on completion or failure. This example needs OpenAI credentials on the server:

Import `github.com/chaserensberger/wingman/api` as `api` for event types.

```go
modelRef := "openai/gpt-5.6-terra"
stream, err := wingman.Run(ctx, client.RunRequest{
	Agent: &client.AgentSpec{
		Name:     "Assistant",
		ModelRef: &modelRef,
	},
	Message: "Explain what an HTTP API does.",
})
if err != nil {
	return err
}
defer stream.Close()

for stream.Next() {
	event := stream.Event()
	if failure, ok := event.Data.(*api.RunErrorEventData); ok {
		return fmt.Errorf("run failed: %s", failure.Message)
	}
	fmt.Println(event)
}
if err := stream.Err(); err != nil {
	return err
}
```

## Persistent Session Streams

Server-sent events (SSE) send updates over an open HTTP connection.
`StreamSessionEvents` reads these updates for a saved session.
Your application must save the last processed event sequence and reconnect after a disconnect.

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
	if event.Type == api.SessionEventEventsResyncRequired {
		reloadSessionAndRun(sessionID)
		break
	}
	if event.Type != api.SessionEventEventsSynchronized {
		applyEvent(event)
	}
	if event.Cursor != nil && event.Cursor.Seq > after {
		after = event.Cursor.Seq
		saveLastSequence(sessionID, after)
	}
}
if err := stream.Err(); err != nil {
	return err
}
```

Save state before its cursor, or save both in one transaction. Otherwise, a crash can skip an event that your application did not apply.
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
