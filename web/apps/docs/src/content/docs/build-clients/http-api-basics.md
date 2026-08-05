---
title: "HTTP API Basics"
description: "Use Wingman from your own client over HTTP."
---

# HTTP API Basics

Wingman is designed to be driven by clients. A client can be a web app, CLI, TUI, editor extension, script, or internal service.

## Basic Flow

Most clients follow this sequence:

1. Check health with `GET /health`.
2. Load an owner credential or client bearer token, then check readiness with `GET /ready`.
3. Configure provider auth with `PUT /provider/auth`.
4. Create or reuse an agent with `/agents`.
5. Create or reuse a Workspace with `/workspaces` if the session needs a saved context.
6. Create a session with `POST /sessions`.
7. Admit messages with `POST /sessions/{id}/message`.
8. Subscribe to updates with `GET /sessions/{id}/events`.

## Authentication

`GET /health` is public. Protected API routes accept an owner credential or a
client bearer token.

Trusted local administration can load the owner credential from daemon state:

```bash
export WINGMAN_TOKEN=$(cat "${XDG_STATE_HOME:-$HOME/.local/state}/wingman/credential")
```

Send it as a bearer token:

```text
Authorization: Bearer <token>
```

Do not copy the owner credential into a remote client. Create a registered client
from the local owner Console or with `POST /clients`, then provide the returned
token to that client:

```text
Authorization: Bearer <client-token>
```

Store the token in the client secret store. Wingman stores only its hash. Rotate
it with `POST /clients/{id}/token` when it must be replaced.

A client bearer token is bound to one Wingman Client. A client-authenticated
request cannot select another client with `X-Wingman-Client`.

The owner credential can use `X-Wingman-Client` for local administration. The
header is not an authentication credential.

See [Authentication](/concepts/authentication) for the local Console session and
client bearer tokens.

## Test a Remote Server

The repository includes a headless Go reference client and an exe.dev deployment
helper. Deploy the current checkout to a VM:

```bash
./scripts/deploy-exe ratchet-mews
```

The helper builds a Linux `amd64` binary, installs it on
`ratchet-mews.exe.xyz`, starts `wingman up` on loopback port `2323`, and sets
the exe.dev HTTPS proxy port. Set `WINGMAN_EXE_ARCH=arm64` before the command
for an arm64 VM.

Create a client and print its access token on the VM:

```bash
ssh ratchet-mews.exe.xyz 'wingman clients create --id cli_reference --name "Reference client"'
```

Connect the reference client with the printed access token:

```bash
go run ./examples/client connect \
  --server https://ratchet-mews.exe.xyz \
  --token '<access-token>' \
  --token-file ./ratchet-mews.token
```

The client verifies `GET /ready` with the bearer token and writes it to the
supplied `0600` file. Verify that the saved token remains valid later:

```bash
go run ./examples/client status \
  --server https://ratchet-mews.exe.xyz \
  --token-file ./ratchet-mews.token
```

Remove the local token when it is no longer needed:

```bash
go run ./examples/client disconnect --token-file ./ratchet-mews.token
```

The deployment helper does not change the VM share's public/private setting.

## OpenAPI and TypeScript

The running daemon publishes its OpenAPI 3.1 contract at `/openapi.json`. The
contract includes canonical errors, request and response resources, and typed
unions for persistent-session and one-shot run events.

## Client Identity

The owner can register clients with `/clients` and pass `X-Wingman-Client` on
client-scoped requests. A bearer access token is bound to its client automatically.

```bash
CLIENT_ID=$(curl -sS -X POST http://localhost:2323/clients \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"id":"cli_example","name":"Example client"}' | jq -r .client.id)
```

Create a session attributed to that client:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d '{"title":"Client session"}'
```

## Workspaces

Workspaces are client-scoped saved contexts with optional directories. `GET /workspaces` lists Workspaces for the active client.

Create one when needed:

```bash
WORKSPACE_ID=$(curl -sS -X POST http://localhost:2323/workspaces \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "$(jq -n \
    --arg name "$(basename "$PWD")" \
    --arg path "$PWD" \
    '{name: $name, path: $path}')" | jq -r .id)
```

Or reuse an existing Workspace:

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "X-Wingman-Client: ${CLIENT_ID}" | jq -r '.[0].id')
```

Create a session in that Workspace:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "{\"title\":\"Client session\",\"workspace_id\":\"${WORKSPACE_ID}\"}"
```

Use `working_directory` instead of `workspace_id` for ad hoc sessions. Do not send both fields.

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

Use `error.code` for program logic and `error.message` for display. The
`X-Request-ID` response header contains the same identifier as
`error.request_id`. Include that value when you report a server failure.

Wingman does not return internal failure details for 5xx responses. It records
those details in daemon logs with the request ID.

## Persistent and Ephemeral Runs

Use persistent sessions when you want history:

```text
POST /sessions
POST /sessions/{id}/message
```

Use an ephemeral session when you want one in-memory run and no transcript:

```text
POST /run
```

See [API](/reference/referenceapi) for endpoint shapes.
