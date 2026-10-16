---
title: "HTTP API Basics"
description: "Use Wingman from your own client over HTTP."
---

# HTTP API Basics

Use Wingman's HTTP API from an application or script. The examples use `curl` to send requests and `jq` to read JSON.
A daemon is a server process that runs in the background.

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

1. Set the server URL and HTTP Basic Auth credentials.
2. Make sure that the server is ready with `GET /ready`.
3. Store provider credentials with `PUT /provider/auth`.
4. Create or reuse an agent with `/agents`.
5. Create a session with `POST /sessions`.
6. Send messages with `POST /sessions/{id}/message`.
7. Read updates with `GET /sessions/{id}/events`.

## Authentication

`GET /health` is public. All other routes require HTTP Basic authentication.
Complete the [direct HTTP setup](/concepts/authentication#direct-http-requests)
before you run the examples in this guide. They use `WINGMAN_URL` and
`WINGMAN_AUTH` from that setup.

For an explicit foreground or remote server, set those variables from that
server's credentials. Before you send credentials to a remote daemon, use TLS
or an SSH tunnel. The `X-Wingman-Client` header selects a client identity. It is not a
credential.

## OpenAPI and SDKs

OpenAPI describes HTTP operations and their data types.
The server publishes its OpenAPI 3.1 schema at `/openapi.json`. It includes request, response, error, and event types.

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

For file and shell tools, create the session with `working_directory` or `workspace_id`.
See [Workspaces](/concepts/workspaces) for grouping sessions by project.

## Handle Errors

JSON error responses use this format:

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

For one run without a saved conversation, use an ephemeral run:

```text
POST /run
```

See [API](/reference/referenceapi) for endpoint shapes.
