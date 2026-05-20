---
title: "HTTP API Basics"
description: "Use Wingman from your own client over HTTP."
---

# HTTP API Basics

Wingman is designed to be driven by clients. A client can be a web app, CLI, TUI, editor extension, script, or internal service.

The default local base URL is:

```text
http://localhost:2323
```

## Basic Flow

Most clients follow this sequence:

1. Check health with `GET /health`.
2. Configure provider auth with `PUT /provider/auth`.
3. Create or reuse an agent with `/agents`.
4. Create a session with `POST /sessions`.
5. Send messages with `POST /sessions/{id}/message` or `POST /sessions/{id}/message/stream`.

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
