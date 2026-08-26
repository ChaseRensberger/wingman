---
title: "TypeScript Client API"
description: "Reference for the @wingman-actor/client public API."
---

# TypeScript Client API

This page lists the public API in `@wingman-actor/client`. Use the
[TypeScript SDK guide](/build-clients/typescript-sdk/) to connect a client. The
guide also describes one-shot and persistent streams.

All resource methods return response data. A non-success HTTP response throws
`APIError`. The package provides TypeScript types for request and response
fields. The [OpenAPI document](/reference/referenceapi/#api) defines the complete HTTP contract.

```ts
import { createWingmanClient } from "@wingman-actor/client";

const client = createWingmanClient({
  baseUrl: "http://localhost:2424",
  username: process.env.WINGMAN_USERNAME,
  password: process.env.WINGMAN_PASSWORD,
  clientName: "cli_example",
});
```

## Create a Client

`createWingmanClient(options)` creates a typed client.

| Option             | Type           | Description                                                                                      |
| ------------------ | -------------- | ------------------------------------------------------------------------------------------------ |
| `baseUrl`          | `string`       | Required HTTP or HTTPS daemon origin. It cannot include a path, query, fragment, or credentials. |
| `username`         | `string`       | HTTP Basic Auth username. The server defaults it to `wingman`.                                   |
| `password`         | `string`       | HTTP Basic Auth password for protected routes.                                                   |
| `clientName`       | `string`       | Default value for the `X-Wingman-Client` header.                                                 |
| `headers`          | `HeadersInit`  | Extra headers for all requests.                                                                  |
| `fetch`            | `typeof fetch` | Fetch implementation for this client.                                                            |
| `maxSSEEventBytes` | `number`       | Maximum size of one SSE event. The default is `1048576`.                                         |

`clientName` identifies the calling application. It does not authenticate the
request. See [Client Identity](/build-clients/http-api-basics/#client-identity).

## Agents

| Method                              | Description                                                              |
| ----------------------------------- | ------------------------------------------------------------------------ |
| `client.agents.list()`              | List agents.                                                             |
| `client.agents.create(request)`     | Create an agent from `CreateAgentRequest`.                               |
| `client.agents.get(id)`             | Get one agent.                                                           |
| `client.agents.update(id, request)` | Update an agent with `UpdateAgentRequest`. Omitted fields do not change. |
| `client.agents.delete(id)`          | Delete an agent.                                                         |

```ts
const agent = await client.agents.create({
  name: "Assistant",
  instructions: "Be helpful and concise.",
  model_ref: "openai/gpt-5.6-terra",
  tools: ["read", "glob", "grep"],
});
```

## Clients

| Method                            | Description                                                                                                                          |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `client.clients.list()`           | List registered client identities.                                                                                                   |
| `client.clients.create(request)`  | Register a client from `CreateClientRequest`.                                                                                        |
| `client.clients.get(id)`          | Get a registered client.                                                                                                             |
| `client.clients.ensure(id, name)` | Create a client or return the existing client with the same ID and name. It throws `APIError` with `conflict` when the names differ. |

```ts
await client.clients.ensure("cli_example", "Example client");
```

## Sessions

| Method                                                             | Description                                                                                            |
| ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------ |
| `client.sessions.list()`                                           | List sessions for the active client.                                                                   |
| `client.sessions.create(request)`                                  | Create a session from `CreateSessionRequest`.                                                          |
| `client.sessions.get(id)`                                          | Get a session and its current state.                                                                   |
| `client.sessions.delete(id, expectedVersion)`                      | Delete a session if its version matches `expectedVersion`.                                             |
| `client.sessions.abort(id)`                                        | Abort the active run for a session.                                                                    |
| `client.sessions.rename(id, request)`                              | Rename a session with `RenameSessionRequest`.                                                          |
| `client.sessions.move(id, request)`                                | Move a session with `MoveSessionRequest`.                                                              |
| `client.sessions.message(id, request)`                             | Submit a message. Use `admit` for retry-safe persistent work.                                          |
| `client.sessions.admit(id, request)`                               | Submit a persistent message with a required `request_id`. An identical retry returns the existing run. |
| `client.sessions.listEvents(id, query?)`                           | Get a finite page of stored session events. `query` accepts `after` and `limit`.                       |
| `client.sessions.streamEvents(id, options?)`                       | Open a persistent session SSE stream.                                                                  |
| `client.sessions.modelCalls.list(id)`                              | List model calls for a session.                                                                        |
| `client.sessions.permissionGrants.list(id)`                        | List permission grants for a session.                                                                  |
| `client.sessions.permissionRequests.list(id)`                      | List pending and resolved permission requests.                                                         |
| `client.sessions.permissionRequests.reply(id, requestID, request)` | Reply to a permission request with `PermissionReplyRequest`.                                           |
| `client.sessions.runs.list(id)`                                    | List runs for a session.                                                                               |
| `client.sessions.runs.get(id, runID)`                              | Get one run.                                                                                           |
| `client.sessions.runs.abort(id, runID)`                            | Abort one run.                                                                                         |
| `client.sessions.toolUses.list(id)`                                | List tool uses for a session.                                                                          |

Before the first `admit` request, use `newMessageAdmission`. Save the returned
request before you send it. If the result is unknown, reuse the saved request.

```ts
import { newMessageAdmission } from "@wingman-actor/client";

const request = newMessageAdmission({
  agent_id: "agt_assistant",
  message: "Summarize this project.",
});
await client.sessions.admit("ses_example", request);
```

`streamEvents(id, options)` returns an async iterable. `options` accepts
`after`, `limit`, `lastEventID`, and `signal`. Save each durable event cursor.
See [Streaming Events](/build-clients/streaming-events/) for replay and recovery.

## One-Shot Runs

| Method                                 | Description                                                                                                 |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| `client.run.stream(request, options?)` | Start `POST /run` and return an async iterable of one-shot run events. `options.signal` aborts the request. |

The iterable returns parsed event envelopes. It returns `{ known: false, event }`
for a valid event type unknown to this SDK version. Ignore unknown events. If the
server does not return SSE, it throws `StreamError`. It also throws `StreamError`
if the stream ends before `done` or `error`.

```ts
for await (const result of client.run.stream({
  model_ref: "openai/gpt-5.6-terra",
  message: "Summarize this project.",
})) {
  if (!result.known) continue;
  if (result.event.type === "stream_part") console.log(result.event.data);
}
```

## Workspaces

| Method                                  | Description                                       |
| --------------------------------------- | ------------------------------------------------- |
| `client.workspaces.list()`              | List Workspaces for the active client.            |
| `client.workspaces.create(request)`     | Create a Workspace with `CreateWorkspaceRequest`. |
| `client.workspaces.get(id)`             | Get one Workspace.                                |
| `client.workspaces.update(id, request)` | Update a Workspace with `UpdateWorkspaceRequest`. |
| `client.workspaces.delete(id)`          | Delete a Workspace.                               |
| `client.workspaces.sessions.list(id)`   | List sessions in a Workspace.                     |

## Providers and Catalog

| Method                                            | Description                                               |
| ------------------------------------------------- | --------------------------------------------------------- |
| `client.providers.list()`                         | List providers.                                           |
| `client.providers.get(name)`                      | Get provider metadata.                                    |
| `client.providers.models.list(name)`              | List models for a provider.                               |
| `client.providers.models.get(name, model)`        | Get model metadata.                                       |
| `client.providers.auth.get()`                     | Get provider credential status. Secrets are not returned. |
| `client.providers.auth.set(request)`              | Set provider credentials with `SetProvidersAuthRequest`.  |
| `client.providers.auth.delete(provider)`          | Delete credentials for one provider.                      |
| `client.providers.oauth.authorize(name, request)` | Start OAuth authorization with `ProviderOAuthRequest`.    |
| `client.providers.oauth.get(name, attempt)`       | Get an OAuth authorization attempt.                       |
| `client.providers.oauth.cancel(name, attempt)`    | Cancel an OAuth authorization attempt.                    |
| `client.catalog.get()`                            | Get the model catalog.                                    |
| `client.catalog.logo(id)`                         | Get a catalog lab logo.                                   |

## Server and Operations

| Method                                 | Description                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------------ |
| `client.current.service()`             | Get service metadata for the current daemon.                                   |
| `client.current.client()`              | Get the active request client.                                                 |
| `client.health.get()`                  | Get public liveness status.                                                    |
| `client.health.ready(options?)`        | Get protected readiness status. `options.signal` aborts the request.           |
| `client.tools.list()`                  | List the effective tool catalog.                                               |
| `client.plugins.list()`                | List external plugins and load errors.                                         |
| `client.plugins.reload()`              | Reload external plugins.                                                       |
| `client.mcp.list()`                    | List configured MCP servers and status.                                        |
| `client.mcp.authorize(name)`           | Start MCP authorization.                                                       |
| `client.mcp.logout(name)`              | Remove MCP authorization.                                                      |
| `client.mcp.connect(name)`             | Connect an MCP server.                                                         |
| `client.mcp.disconnect(name)`          | Disconnect an MCP server.                                                      |
| `client.filesystem.directories(path?)` | List immediate subdirectories. Omit `path` for the daemon user home directory. |
| `client.logs.list()`                   | Read recent process-local daemon log entries.                                  |
| `client.diagnostics.get()`             | Read a bounded daemon diagnostic snapshot.                                     |

## Errors and Stream Helpers

| Export                              | Description                                                                                                                              |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| `APIError`                          | Error for a non-success HTTP response. It includes `status`, `code`, `message`, `requestId`, `details`, `headers`, and `retryAfterMs`.   |
| `StreamError`                       | Error for an invalid, interrupted, or oversized SSE stream. Its `code` is `invalid_stream`, `stream_interrupted`, or `stream_too_large`. |
| `newMessageAdmission(request)`      | Return the request unchanged when it has `request_id`. Otherwise, return a copy with a new UUID.                                         |
| `readSSE(response, maxEventBytes?)` | Parse raw SSE frames into an async iterable. Use resource stream methods unless you need raw frames.                                     |
| `parseSessionEvent(value)`          | Parse one session event envelope.                                                                                                        |
| `parseRunStreamEvent(value)`        | Parse one one-shot run event envelope.                                                                                                   |

```ts
import { APIError } from "@wingman-actor/client";

try {
  await client.sessions.get("ses_missing");
} catch (error) {
  if (error instanceof APIError) {
    console.error(error.status, error.code, error.requestId);
  }
  throw error;
}
```

## Types

The package exports common resource and request types, including `Agent`,
`Client`, `Session`, `Workspace`, `CreateAgentRequest`,
`CreateSessionRequest`, `MessageSessionRequest`, `RunRequest`, and
`SessionEvent`.

For generated types, import `components`, `operations`, or `paths` from the
package. Use matching daemon and SDK release versions.

```ts
import type { components, operations, paths } from "@wingman-actor/client";
```
