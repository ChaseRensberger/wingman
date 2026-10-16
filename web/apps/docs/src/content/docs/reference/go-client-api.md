---
title: "Go Client API"
description: "Connection, request, and stream methods in the Go client."
---

# Go Client API

Import `github.com/chaserensberger/wingman/client` for the HTTP SDK.
Use the [Go SDK guide](/build-clients/go-sdk) for examples.
The complete type and method reference is on [pkg.go.dev](https://pkg.go.dev/github.com/chaserensberger/wingman/client).
Match the SDK version to the server release.

## Connection

| Function                                       | Purpose                                                                                     |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------- |
| `New(baseURL, options...)`                     | Connect to an HTTP or HTTPS origin. Do not include a path, query, fragment, or credentials. |
| `NewLocal(ctx, options...)`                    | Discover the current user's managed local server and read its credentials.                  |
| `NewLocalFromState(ctx, stateDir, options...)` | Use an explicit directory for service discovery.                                            |
| `WithBasicAuth(username, password)`            | Set server credentials.                                                                     |
| `WithClientID(id)`                             | Set `X-Wingman-Client` unless a request sets it.                                            |
| `WithTransport(doer)`                          | Supply an HTTP transport.                                                                   |
| `WithMaxSSEEventBytes(size)`                   | Limit one event frame. The default is 1 MiB.                                                |

`EnsureClient(ctx, id, name)` creates or reuses a client identity. It fails if the saved name differs.
`RegisterClient(ctx, id, name)` creates a new identity.

## JSON requests

Use generated methods with the `WithResponse` suffix. They decode successful responses into typed fields such as `JSON200` and `JSON201`.
HTTP failures return `*APIError`. Methods with `WithBodyWithResponse` accept an `io.Reader` instead of a typed JSON body.

The method name follows the OpenAPI operation ID. For example, `createSession` becomes `CreateSessionWithResponse`.
Read `GET /openapi.json` for operation IDs and request fields.

| Example method                                  | Purpose                             |
| ----------------------------------------------- | ----------------------------------- |
| `GetReadinessWithResponse(ctx, nil)`            | Read server readiness.              |
| `CreateAgentWithResponse(ctx, nil, body)`       | Create an agent.                    |
| `CreateSessionWithResponse(ctx, nil, body)`     | Create a session.                   |
| `GetSessionWithResponse(ctx, id, nil)`          | Read its history and current state. |
| `ListSessionRunsWithResponse(ctx, id, nil)`     | Read saved runs.                    |
| `ListSessionToolUsesWithResponse(ctx, id, nil)` | Read tool execution records.        |

For retry-safe message submission, use `NewMessageAdmission` and `AdmitMessage` as shown in the guide.
Save the request before sending it. Reuse it when a network failure leaves the result unknown.

## Streams

Server-sent events (SSE) send updates over an open HTTP connection.

| Method                                         | Purpose                                       |
| ---------------------------------------------- | --------------------------------------------- |
| `Run(ctx, body)`                               | Open a one-shot run stream.                   |
| `StreamSessionEvents(ctx, sessionID, options)` | Open one connection for saved session events. |
| `ListSessionEvents(ctx, sessionID, options)`   | Read one finite page of saved events.         |

`SessionEventsOptions` accepts `After`, `LastEventID`, and `Limit`.
For streams, `After` takes precedence over `LastEventID`. The finite-page method uses `After` and `Limit` only.

Both stream types provide `Next`, `Event`, `Frame`, `Err`, and `Close`.
`Event` returns the typed event. `Frame` returns raw SSE fields.
`Err` reports read or decode failures. A run's `error` event remains event data, so handle it separately.

Close streams when you stop reading. Session streams do not reconnect automatically.
See [Streaming Events](/build-clients/streaming-events) for replay and cursor rules.

## Errors

`APIError` contains `StatusCode`, parsed `Response`, response `Headers`, `RequestID`, and `RetryAfter`.
Use the error code for program logic. Include the request ID when reporting a failure.

MCP authorization and logout methods exist, but the server returns `501 Not Implemented` for both.
