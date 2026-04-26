---
title: "Server"
group: "Usage"
draft: false
order: 11
---

# Server

`wingman serve` exposes the same runtime primitives as the SDK over HTTP, with SQLite-backed persistence for agents, sessions, message history, and provider credentials.

## Start the server

```bash
wingman serve
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--host` | `127.0.0.1` | Host interface to bind |
| `--port` | `2323` | Port to listen on |
| `--db` | `~/.local/share/wingman/wingman.db` | SQLite database path |

## What the server adds

Compared with the SDK, the server provides:

- persisted agents, sessions, and message history
- provider credential storage (per-provider, in SQLite)
- JSON APIs for every runtime primitive
- SSE streaming for `POST /sessions/{id}/message/stream`
- per-session abort via `POST /sessions/{id}/abort`
- graceful shutdown that drains in-flight streaming handlers

## Provider auth

The server reads provider credentials from its SQLite auth store. It does not resolve API keys from environment variables during request handling.

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'
```

`GET /provider/auth` returns a `configured` flag per provider without leaking the secret.

## Persistence model

The server persists definitions and history, then reconstructs live runtime objects on demand.

- **Agents** are stored as configuration records (provider, model, options, instructions, tool names, optional output schema).
- **Sessions** persist `work_dir` and message history (one row per message, with parts as separate rows).

When you post a message, the server:

1. loads the stored agent record and session record
2. constructs the provider via the registry, injecting stored credentials
3. builds a `*session.Session` with the [storage plugin](./storage#the-storage-plugin) installed via `session.WithPlugin(storage.NewPlugin(store, sess.ID))`
4. runs `Run` or `RunStream`
5. returns the response (or streams events) to the client

Steps 1–4 happen inside `buildSession`. The storage plugin handles both sides of persistence: its `BeforeRun` hook loads the session's prior history from SQLite into the loop, and its sink calls `store.AppendMessage` for each new message as the loop emits it. The server itself doesn't talk to the storage layer during a run — that's the plugin's job.

See [Storage](./storage) for the schema and [Sessions](./sessions) for what `Run` actually does.

## Streaming behavior

`POST /sessions/{id}/message/stream` returns `text/event-stream`. Each event is `event: <type>\ndata: <json>\n\n`. The standard 60-second request timeout is bypassed for this path. The server tracks in-flight streams in a `WaitGroup` and waits for them during graceful shutdown (subject to the shutdown context's deadline).

The full envelope schema is in [Streaming](./streaming).

## Aborting a session

`POST /sessions/{id}/abort` cancels every in-flight Run for that session. The loop returns with `StopReasonAborted` and the streaming endpoint emits a final `finish` part with `FinishReasonAborted` before closing. Aborts are idempotent — if no run is in flight, the response still returns 200 with `aborted: 0`. A 404 is only returned when the session id is unknown.

## Typical workflow

```bash
# 1. Configure provider auth
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'

# 2. Create an agent
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Assistant",
    "instructions": "Be helpful and concise.",
    "tools": ["bash", "read", "write"],
    "provider": "anthropic",
    "model": "claude-haiku-4-5",
    "options": {"max_tokens": 4096}
  }'

# 3. Create a session
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/tmp"}'

# 4. Send a message
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "agt_...", "message": "What OS am I on?"}'
```

## Route families

| Resource | Purpose |
|---|---|
| `/health` | Health check |
| `/provider` | Provider registry, model metadata, and auth |
| `/agents` | Agent CRUD |
| `/sessions` | Session CRUD, message execution, streaming, abort |

See [API](./api) for endpoint-level details.
