---
title: "HTTP API Basics"
description: "Use Wingman from your own client over HTTP."
---

# HTTP API Basics

Clients control Wingman. A client can be a web app, CLI, TUI, editor extension,
script, or internal service.

## Choose a Client

Use the client that matches where and how you connect:

| Need                                                  | Use                                                                                |
| ----------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Send interactive requests to the local managed daemon | [`wingman api`](/reference/cli#api-command)                                        |
| Build a local Go application for the managed daemon   | [Go SDK](/build-clients/go-sdk) with `NewLocal`                                    |
| Build a Go or TypeScript application for a known URL  | [Go SDK](/build-clients/go-sdk) or [TypeScript SDK](/build-clients/typescript-sdk) |
| Call the API from a script or another language        | Direct HTTP requests in this guide                                                 |

`wingman api` does not connect to foreground or remote servers. Use direct HTTP
or an SDK when you have an explicit server URL.

## Basic Flow

Most clients follow this sequence:

1. Make sure that the daemon is healthy with `GET /health`.
2. Set the server URL and HTTP Basic Auth credentials.
3. Make sure that the daemon is ready with `GET /ready`.
4. Configure provider auth with `PUT /provider/auth`.
5. Create or reuse an agent with `/agents`.
6. If the session needs saved context, create or reuse a Workspace with `/workspaces`.
7. Create a session with `POST /sessions`.
8. Admit messages with `POST /sessions/{id}/message`.
9. Subscribe to updates with `GET /sessions/{id}/events`.

## Authentication

`GET /health` is public. All other routes require HTTP Basic authentication.
Complete the [direct HTTP setup](/concepts/authentication#direct-http-requests)
before you run the examples in this guide. They use `WINGMAN_URL` and
`WINGMAN_AUTH` from that setup.

For an explicit foreground or remote server, set those variables from that
server's credentials. Before you send credentials to a remote daemon, use TLS
or an SSH tunnel. The `X-Wingman-Client` header selects attribution. It is not a
credential.

## OpenAPI and SDKs

The running daemon publishes its OpenAPI 3.1 contract at `/openapi.json`. The
contract includes canonical errors, request and response resources, and typed
unions for persistent-session and one-shot run events.

Use the [Go SDK](/build-clients/go-sdk) or
[TypeScript SDK](/build-clients/typescript-sdk) for generated, typed clients.

## Client Identity

Any caller with daemon access can register clients with `/clients`. The caller
can send `X-Wingman-Client` on client-scoped requests.

```bash
CLIENT_ID=$(curl -sS -X POST "$WINGMAN_URL/clients" \
  -u "$WINGMAN_AUTH" \
  -H "Content-Type: application/json" \
  -d '{"id":"cli_example","name":"Example client"}' | jq -r .client.id)
```

Create a session attributed to that client:

```bash
curl -sS -X POST "$WINGMAN_URL/sessions" \
  -u "$WINGMAN_AUTH" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d '{"title":"Client session"}'
```

## Workspaces

Workspaces are client-scoped saved contexts with optional directories.
`GET /workspaces` lists Workspaces for the active client.

Create one when needed:

```bash
WORKSPACE_ID=$(curl -sS -X POST "$WINGMAN_URL/workspaces" \
  -u "$WINGMAN_AUTH" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "$(jq -n \
    --arg name "$(basename "$PWD")" \
    --arg path "$PWD" \
    '{name: $name, path: $path}')" | jq -r .id)
```

Or reuse an existing Workspace:

```bash
WORKSPACE_ID=$(curl -sS "$WINGMAN_URL/workspaces" \
  -u "$WINGMAN_AUTH" \
  -H "X-Wingman-Client: ${CLIENT_ID}" | jq -r '.[0].id')
```

Create a session in that Workspace:

```bash
curl -sS -X POST "$WINGMAN_URL/sessions" \
  -u "$WINGMAN_AUTH" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "{\"title\":\"Client session\",\"workspace_id\":\"${WORKSPACE_ID}\"}"
```

For ad hoc sessions, use `working_directory` instead of `workspace_id`. Do not
send both fields.

## Handle Errors

Every non-success JSON response uses one envelope:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "message is required",
    "request_id": "6c4f5c41c4c1/abc-000001"
  }
}
```

Use `error.code` for program logic. Use `error.message` for display. The
`X-Request-ID` response header contains the identifier in `error.request_id`.
If you report a server failure, include that value.

Wingman does not return internal failure details for 5xx responses. It records
the details in daemon logs with the request ID.

## Persistent and Ephemeral Runs

If you want history, use persistent sessions:

```text
POST /sessions
POST /sessions/{id}/message
```

If you want one in-memory run and no transcript, use an ephemeral session:

```text
POST /run
```

See [API](/reference/referenceapi) for endpoint shapes.
