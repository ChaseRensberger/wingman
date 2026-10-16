---
title: "TypeScript Client API"
description: "Connection, request, and stream methods in the TypeScript client."
---

# TypeScript Client API

Import `createWingmanClient` from `@wingman-actor/client`.
Use the [TypeScript SDK guide](/build-clients/typescript-sdk) for examples.
Match the package version to the server release.

## Connection

`createWingmanClient(options)` accepts these fields:

| Field              | Purpose                                                                                |
| ------------------ | -------------------------------------------------------------------------------------- |
| `baseUrl`          | Required HTTP or HTTPS origin. Do not include a path, query, fragment, or credentials. |
| `username`         | Basic Auth username. Defaults to `wingman` when a password is supplied.                |
| `password`         | Basic Auth password.                                                                   |
| `clientName`       | Client ID for `X-Wingman-Client`, despite the field name.                              |
| `headers`          | Extra headers for requests.                                                            |
| `fetch`            | A replacement for the global `fetch` function.                                         |
| `maxSSEEventBytes` | Maximum size of one event frame. Defaults to 1 MiB.                                    |

`client.clients.ensure(id, name)` creates or reuses a client identity.
It throws an `APIError` with code `conflict` if the saved name differs.

## Resource methods

Methods return response data or throw `APIError` for HTTP failures.
Use the package's TypeScript declarations for exact arguments and return types.

| Resource                                   | Methods                                                                          |
| ------------------------------------------ | -------------------------------------------------------------------------------- |
| `agents`                                   | `list`, `create`, `get`, `update`, `delete`                                      |
| `clients`                                  | `list`, `create`, `get`, `ensure`                                                |
| `sessions`                                 | `list`, `create`, `get`, `delete`, `rename`, `move`, `message`, `admit`, `abort` |
| `sessions.runs`                            | `list`, `get`, `abort`                                                           |
| `sessions.modelCalls`, `sessions.toolUses` | `list`                                                                           |
| `sessions.permissionRequests`              | `list`, `reply`                                                                  |
| `sessions.permissionGrants`                | `list`                                                                           |
| `sessions.macros`                          | `list`, `admit`                                                                  |
| `sessions.actions`                         | `admit`                                                                          |
| `workspaces`                               | `list`, `create`, `get`, `update`, `delete`, `sessions.list`                     |
| `providers`                                | `list`, `get`, `models.list`, `models.get`                                       |
| `providers.auth`                           | `get`, `set`, `delete`                                                           |
| `providers.oauth`                          | `authorize`, `get`, `cancel`                                                     |
| `mcp`                                      | `list`, `connect`, `disconnect`                                                  |
| `plugins`                                  | `list`, `reload`                                                                 |
| `health`                                   | `get`, `ready`                                                                   |
| `current`                                  | `service`, `client`                                                              |

Other methods include `tools.list`, `actions.list`, `catalog.get`, `filesystem.directories`, `logs.list`, `diagnostics.get`, and `service.restart`.
`mcp.authorize` and `mcp.logout` exist, but both return a server error with HTTP status `501`.

For retry-safe submission, create and save the request with `newMessageAdmission`, `newMacroAdmission`, or `newActionAdmission` before calling `admit`.
`newRequestID()` creates a UUID for other callers that need a request ID.

## Streams

`client.sessions.streamEvents(id, options)` opens one connection.
Its optional fields are `after`, `limit`, `lastEventID`, and `signal`.
`after` takes precedence over `lastEventID`. The method does not reconnect automatically.

`client.sessions.listEvents(id, query)` reads a finite page. Its query accepts `after` and `limit`.
`client.run.stream(request, options)` opens a one-shot stream. Its options accept `signal` for cancellation.

Both stream methods return an async iterable of `{ known, event }` values.
Unknown types have `known: false`. Handle a run's `error` event separately from HTTP or stream errors.
See [Streaming Events](/build-clients/streaming-events) for replay and cursor rules.

## Errors and types

`APIError` contains `status`, `code`, `message`, `requestId`, `details`, `headers`, and `retryAfterMs`.
`StreamError.code` is `invalid_stream`, `stream_interrupted`, or `stream_too_large`.

The package exports common resource and request types. It also exports the generated `components`, `operations`, and `paths` types.
For custom stream readers, use `readSSE`, `parseSessionEvent`, and `parseRunStreamEvent`.
