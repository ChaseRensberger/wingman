---
title: "Go Client API"
description: "Reference for the Wingman Go client public API."
---

# Go Client API

This page lists the public API in `github.com/chaserensberger/wingman/client`.
Use the [Go SDK guide](/build-clients/go-sdk/) to connect to a daemon and use
persistent streams correctly.

`SDK` embeds the generated client. Use generated methods with the
`WithResponse` suffix for JSON endpoints. These methods return an HTTP response
struct with typed fields for each success status. They return `*APIError` for a
non-success response.

```go
wingman, err := client.New(
	"http://localhost:2323",
	client.WithBasicAuth("wingman", os.Getenv("WINGMAN_PASSWORD")),
	client.WithClientID("cli_example"),
)
if err != nil {
	return err
}
```

The package also generates `WithBodyWithResponse` variants for JSON write
methods. Use them only when the request body is an `io.Reader`. Use the JSON
methods in this reference for normal Go values.

## Create a Client

| Function or option | Description |
| --- | --- |
| `New(baseURL, options...)` | Create an SDK for an HTTP or HTTPS daemon origin. The URL cannot include a path, query, fragment, or credentials. |
| `NewLocal(ctx, options...)` | Discover and verify the managed local daemon for the current user. It reads the public registration and private generated credentials. |
| `NewLocalFromState(ctx, stateDir, options...)` | Discover a managed daemon from an explicit state directory. |
| `WithBasicAuth(username, password)` | Set HTTP Basic authentication credentials. |
| `WithClientID(id)` | Set `X-Wingman-Client` on requests that do not set it explicitly. |
| `WithTransport(doer)` | Set the `HttpRequestDoer` for requests. |
| `WithMaxSSEEventBytes(size)` | Set the maximum accepted SSE frame size. The default is `1048576`. |

`WithClientID` identifies the calling application. It does not authenticate the
request. See [Client Identity](/build-clients/http-api-basics/#client-identity).

## Agents

| Method | Description |
| --- | --- |
| `ListAgentsWithResponse(ctx, nil)` | List agents. |
| `CreateAgentWithResponse(ctx, nil, body)` | Create an agent. |
| `GetAgentWithResponse(ctx, id, nil)` | Get one agent. |
| `UpdateAgentWithResponse(ctx, id, nil, body)` | Update an agent. Omitted fields do not change. |
| `DeleteAgentWithResponse(ctx, id, nil)` | Delete an agent. |

```go
instructions := "Be helpful and concise."
modelRef := "openai/gpt-5.6-terra"
tools := []string{"read", "glob", "grep"}
response, err := wingman.CreateAgentWithResponse(ctx, nil, client.CreateAgentRequest{
	Name:         "Assistant",
	Instructions: &instructions,
	ModelRef:     &modelRef,
	Tools:        &tools,
})
if err != nil {
	return err
}
agent := response.JSON201
```

## Client Identities

| Method | Description |
| --- | --- |
| `RegisterClient(ctx, id, name)` | Create a client identity and return the client. |
| `EnsureClient(ctx, id, name)` | Create a client or return the existing client with the same ID and name. It returns an error when the names differ. |
| `ListClientsWithResponse(ctx, nil)` | List registered client identities. |
| `CreateClientWithResponse(ctx, nil, body)` | Register a client identity. |
| `GetClientWithResponse(ctx, id, nil)` | Get a client identity. |
| `GetCurrentClientWithResponse(ctx, nil)` | Get the active request client. |

```go
_, err := wingman.EnsureClient(ctx, "cli_example", "Example client")
if err != nil {
	return err
}
```

## Sessions

| Method | Description |
| --- | --- |
| `ListSessionsWithResponse(ctx, nil)` | List sessions for the active client. |
| `CreateSessionWithResponse(ctx, nil, body)` | Create a session. |
| `GetSessionWithResponse(ctx, id, nil)` | Get a session and its current state. |
| `DeleteSessionWithResponse(ctx, id, params)` | Delete a session if `params.ExpectedVersion` matches. |
| `AbortSessionWithResponse(ctx, id, nil)` | Abort the active run for a session. |
| `RenameSessionWithResponse(ctx, id, nil, body)` | Rename a session. |
| `MoveSessionWithResponse(ctx, id, nil, body)` | Move a session. |
| `MessageSessionWithResponse(ctx, id, nil, body)` | Submit a message. Use `AdmitMessage` for retry-safe persistent work. |
| `AdmitMessage(ctx, sessionID, request)` | Submit a persistent message with a required request ID. An identical retry returns the existing run. |
| `ListSessionModelCallsWithResponse(ctx, id, nil)` | List model calls for a session. |
| `ListPermissionGrantsWithResponse(ctx, id, nil)` | List permission grants for a session. |
| `ListPermissionRequestsWithResponse(ctx, id, nil)` | List pending and resolved permission requests. |
| `ReplyPermissionRequestWithResponse(ctx, id, requestID, nil, body)` | Reply to a permission request. |
| `ListSessionRunsWithResponse(ctx, id, nil)` | List runs for a session. |
| `GetSessionRunWithResponse(ctx, id, runID, nil)` | Get one run. |
| `AbortSessionRunWithResponse(ctx, id, runID, nil)` | Abort one run. |
| `ListSessionToolUsesWithResponse(ctx, id, nil)` | List tool uses for a session. |

Use `NewMessageAdmission` before the first `AdmitMessage` request. Save the
returned request before you send it. Reuse the saved request if the result is
unknown.

```go
request := client.NewMessageAdmission(client.MessageSessionRequest{
	AgentId: "agt_assistant",
	Message: "Summarize this project.",
})
admission, err := wingman.AdmitMessage(ctx, "ses_example", request)
if err != nil {
	return err
}
_ = admission
```

## Session Events

| Method or type | Description |
| --- | --- |
| `ListSessionEvents(ctx, sessionID, options)` | Get one finite page of durable session events. |
| `StreamSessionEvents(ctx, sessionID, options)` | Open a persistent session SSE stream. |
| `SessionEventsOptions` | Set `After`, `LastEventID`, or `Limit`. `After` takes precedence when both cursor fields are set. |
| `SessionEventStream.Next()` | Advance to the next session event. |
| `SessionEventStream.Event()` | Get the most recent typed event. |
| `SessionEventStream.Frame()` | Get the raw SSE frame for the most recent event. |
| `SessionEventStream.Err()` | Get the terminal stream error. |
| `SessionEventStream.Close()` | Close the stream. |

Save each durable event cursor. See [Streaming Events](/build-clients/streaming-events/)
for replay and recovery.

## One-Shot Runs

| Method or type | Description |
| --- | --- |
| `Run(ctx, body)` | Start `POST /run` and return a one-shot SSE stream. |
| `RunStream.Next()` | Advance to the next run event. It returns false after a terminal error or stream end. |
| `RunStream.Event()` | Get the most recent typed run event. |
| `RunStream.Frame()` | Get the raw SSE frame for the most recent event. |
| `RunStream.Err()` | Get the terminal stream error. |
| `RunStream.Close()` | Close the stream before it ends. |
| `SSEFrame` | Raw SSE frame with `ID`, `Event`, and `Data` fields. |

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

## Workspaces

| Method | Description |
| --- | --- |
| `ListWorkspacesWithResponse(ctx, nil)` | List Workspaces for the active client. |
| `CreateWorkspaceWithResponse(ctx, nil, body)` | Create a Workspace. |
| `GetWorkspaceWithResponse(ctx, id, nil)` | Get one Workspace. |
| `UpdateWorkspaceWithResponse(ctx, id, nil, body)` | Update a Workspace. |
| `DeleteWorkspaceWithResponse(ctx, id, nil)` | Delete a Workspace. |
| `ListWorkspaceSessionsWithResponse(ctx, id, nil)` | List sessions in a Workspace. |

## Providers and Catalog

| Method | Description |
| --- | --- |
| `ListProvidersWithResponse(ctx, nil)` | List providers. |
| `GetProviderWithResponse(ctx, name, nil)` | Get provider metadata. |
| `ListProviderModelsWithResponse(ctx, name, nil)` | List models for a provider. |
| `GetProviderModelWithResponse(ctx, name, model, nil)` | Get model metadata. |
| `GetProviderAuthWithResponse(ctx, nil)` | Get provider credential status. Secrets are not returned. |
| `SetProviderAuthWithResponse(ctx, nil, body)` | Set provider credentials. |
| `DeleteProviderAuthWithResponse(ctx, provider, nil)` | Delete credentials for one provider. |
| `AuthorizeProviderOAuthWithResponse(ctx, name, nil, body)` | Start OAuth authorization. |
| `GetProviderOAuthAttemptWithResponse(ctx, name, attempt, nil)` | Get an OAuth authorization attempt. |
| `CancelProviderOAuthAttemptWithResponse(ctx, name, attempt, nil)` | Cancel an OAuth authorization attempt. |
| `GetModelCatalogWithResponse(ctx, nil)` | Get the model catalog. |
| `GetCatalogLabLogoWithResponse(ctx, id, nil)` | Get a catalog lab logo. |

## Server and Operations

| Method | Description |
| --- | --- |
| `GetServiceWithResponse(ctx, nil)` | Get service metadata for the current daemon. |
| `GetHealthWithResponse(ctx, nil)` | Get public liveness status. |
| `GetReadinessWithResponse(ctx, nil)` | Get protected readiness status. |
| `ListToolsWithResponse(ctx, nil)` | List the effective tool catalog. |
| `ListPluginsWithResponse(ctx, nil)` | List external plugins and load errors. |
| `ReloadPluginsWithResponse(ctx, nil)` | Reload external plugins. |
| `ListMCPServersWithResponse(ctx, nil)` | List configured MCP servers and status. |
| `AuthorizeMCPServerWithResponse(ctx, name, nil)` | Start MCP authorization. |
| `LogoutMCPServerWithResponse(ctx, name, nil)` | Remove MCP authorization. |
| `ConnectMCPServerWithResponse(ctx, name, nil)` | Connect an MCP server. |
| `DisconnectMCPServerWithResponse(ctx, name, nil)` | Disconnect an MCP server. |
| `ListDirectoriesWithResponse(ctx, params)` | List immediate subdirectories. Omit `params.Path` for the daemon user home directory. |
| `ListLogsWithResponse(ctx, nil)` | Read recent process-local daemon log entries. |
| `GetDiagnosticsWithResponse(ctx, nil)` | Read a bounded daemon diagnostic snapshot. |

## Errors and Types

`APIError` reports a non-success response. It includes `StatusCode`, the parsed
`Response`, response `Headers`, `RequestID`, and `RetryAfter`.

```go
var apiError *client.APIError
if errors.As(err, &apiError) {
	fmt.Println(apiError.StatusCode, apiError.RequestID)
}
```

The `client` package exports generated request, response, and resource types.
Use [pkg.go.dev](https://pkg.go.dev/github.com/chaserensberger/wingman/client)
for the complete Go type reference. Use the matching daemon and SDK release
versions.
