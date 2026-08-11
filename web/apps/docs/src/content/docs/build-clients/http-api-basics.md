---
title: "HTTP API Basics"
description: "Use Wingman from your own client over HTTP."
---

# HTTP API Basics

Wingman is designed to be driven by clients. A client can be a web app, CLI, TUI, editor extension, script, or internal service.

## Basic Flow

Most clients follow this sequence:

1. Check health with `GET /health`.
2. Load the daemon password, then check readiness with `GET /ready`.
3. Configure provider auth with `PUT /provider/auth`.
4. Create or reuse an agent with `/agents`.
5. Create or reuse a Workspace with `/workspaces` if the session needs a saved context.
6. Create a session with `POST /sessions`.
7. Admit messages with `POST /sessions/{id}/message`.
8. Subscribe to updates with `GET /sessions/{id}/events`.

## Authentication

`GET /health` is public. Other API routes require the daemon password.

Obtain the daemon password through a secure channel, then set it in your client
environment. For a local managed service:

```bash
export WINGMAN_DAEMON_PASSWORD="$(wingman service password)"
```

For a foreground server started with `WINGMAN_PASSWORD`, the client uses the
same value but must set `WINGMAN_DAEMON_PASSWORD` itself.

Send it with HTTP Basic authentication and the username `wingman`:

```text
Authorization: Basic <base64("wingman:<password>")>
```

For example, `curl -u "wingman:${WINGMAN_DAEMON_PASSWORD}" http://localhost:2323/ready`
sends the required credentials. Use TLS or an SSH tunnel before sending this
password to a remote daemon. The
`X-Wingman-Client` header is an attribution selector, not a credential.

See [Authentication](/concepts/authentication) for Console sessions.

## Test a Remote Server

The repository includes a headless Go reference client and an exe.dev deployment
helper. Deploy the current checkout to a VM:

```bash
./scripts/deploy-exe ratchet-mews
```

The helper builds a Linux `amd64` binary, installs it on
`ratchet-mews.exe.xyz`, starts `wingman service start` on loopback port `2323`, and sets
the exe.dev HTTPS proxy port. Set `WINGMAN_EXE_ARCH=arm64` before the command
for an arm64 VM.

Use the managed service password to register a client on the VM:

```bash
ssh ratchet-mews.exe.xyz 'wingman clients create --id cli_reference --name "Reference client"'
```

Check the remote daemon with HTTP Basic authentication:

```bash
curl -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  https://ratchet-mews.exe.xyz/ready
```

The deployment helper does not change the VM share's public/private setting.

## Go SDK

The Go SDK is generated from the same OpenAPI 3.1 contract as the TypeScript
client. Add the current release to your application:

```bash
go get github.com/chaserensberger/wingman/client@latest
```

Create an authenticated client, then call generated endpoint methods:

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/chaserensberger/wingman/client"
)

func main() {
	wingman, err := client.New(
		"http://localhost:2323",
		client.WithPassword(os.Getenv("WINGMAN_DAEMON_PASSWORD")),
		client.WithClientID("cli_example"),
	)
	if err != nil {
		panic(err)
	}

	ready, err := wingman.GetReadinessWithResponse(context.Background(), nil)
	if err != nil {
		panic(err)
	}
	fmt.Println(ready.JSON200.Version)
}
```

Generated methods have a `WithResponse` suffix and expose typed JSON response
fields. `Run` and `StreamSessionEvents` provide typed SSE streams. Non-success
responses return `*client.APIError`.

The SDK is released with the Wingman module. A new public `v*` Git tag is
automatically available through the Go module proxy and indexed at
[`pkg.go.dev`](https://pkg.go.dev/github.com/chaserensberger/wingman/client).

## OpenAPI and TypeScript

The running daemon publishes its OpenAPI 3.1 contract at `/openapi.json`. The
contract includes canonical errors, request and response resources, and typed
unions for persistent-session and one-shot run events.

## Client Identity

Any caller with daemon access can register clients with `/clients` and pass
`X-Wingman-Client` on client-scoped requests.

```bash
CLIENT_ID=$(curl -sS -X POST http://localhost:2323/clients \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d '{"id":"cli_example","name":"Example client"}' | jq -r .client.id)
```

Create a session attributed to that client:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d '{"title":"Client session"}'
```

## Workspaces

Workspaces are client-scoped saved contexts with optional directories. `GET /workspaces` lists Workspaces for the active client.

Create one when needed:

```bash
WORKSPACE_ID=$(curl -sS -X POST http://localhost:2323/workspaces \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
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
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "X-Wingman-Client: ${CLIENT_ID}" | jq -r '.[0].id')
```

Create a session in that Workspace:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
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
