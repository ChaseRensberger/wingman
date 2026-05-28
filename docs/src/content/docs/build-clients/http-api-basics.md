---
title: "HTTP API Basics"
description: "Use Wingman from your own client over HTTP."
---

# HTTP API Basics

Wingman is designed to be driven by clients. A client can be a web app, CLI, TUI, editor extension, script, or internal service.

The default local workspace URL is:

```text
http://localhost:2323
```

## Basic Flow

Most clients follow this sequence:

1. Check health with `GET /health`.
2. Configure provider auth with `PUT /provider/auth`.
3. Create or reuse an agent with `/agents`.
4. Create or reuse a Workspace with `/workspaces` if the session should belong to a saved context.
5. Create a session with `POST /sessions`.
6. Send messages with `POST /sessions/{id}/message` or `POST /sessions/{id}/message/stream`.

## Client Identity

Clients can register themselves with `/clients` and then pass `X-Wingman-Client` when creating sessions. This lets different clients organize their own sessions without treating client identity as an auth boundary.

```bash
CLIENT_ID=$(curl -sS -X POST http://localhost:2323/clients \
  -H "Content-Type: application/json" \
  -d '{"name":"my-client"}' | jq -r .id)
```

Create a session attributed to that client:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d '{"title":"Client session"}'
```

## Workspaces

Workspaces are client-scoped saved contexts with optional directories. `GET /workspaces` lists Workspaces for the active client; create one with `POST /workspaces` when needed.

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces \
  -H "X-Wingman-Client: ${CLIENT_ID}" | jq -r '.[0].id')
```

Create a session in that Workspace:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: ${CLIENT_ID}" \
  -d "{\"title\":\"Client session\",\"workspace_id\":\"${WORKSPACE_ID}\"}"
```

Use `working_directory` instead of `workspace_id` for ad hoc sessions. Do not send both fields.

## Persistent And Ephemeral Runs

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
