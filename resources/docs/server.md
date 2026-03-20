---
title: "Server"
group: "Usage"
draft: false
order: 11
---

# Server

`wingman serve` exposes the same runtime primitives as the SDK over HTTP and adds SQLite-backed persistence for agents, sessions, fleets, formations, and provider credentials.

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

- persisted agents, sessions, fleets, and formations
- provider credential management in SQLite
- JSON APIs for every runtime primitive
- SSE endpoints for streaming session, fleet, and formation execution

## Provider auth

The server reads provider credentials from its SQLite auth store. It does not resolve API keys from environment variables during request handling.

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'
```

## Persistence model

The server persists definitions and history, then reconstructs live runtime objects on demand.

- **Agents** are stored as configuration records.
- **Sessions** persist `work_dir` and message history.
- **Fleets** persist a template configuration and accept tasks at run time.
- **Formations** persist workflow definitions and execute runs ephemerally.

For example, when you post a message to a session, the server rebuilds a `session.Session`, replays history into it, runs the agent loop, and writes the updated history back to SQLite.

## Streaming behavior

Streaming endpoints use Server-Sent Events.

- `POST /sessions/{id}/message/stream`
- `POST /fleets/{id}/run/stream`
- `POST /formations/{id}/run/stream`

The standard 60 second request timeout is bypassed for these streaming paths.

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
    "model": "claude-sonnet-4-5",
    "options": {"max_tokens": 4096}
  }'

# 3. Create a session
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/tmp"}'

# 4. Send a message
curl -sS -X POST http://localhost:2323/sessions/01XYZ.../message \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "01ABC...", "message": "What OS am I on?"}'
```

## Route families

| Resource | Purpose |
|---|---|
| `/health` | Health check |
| `/provider` | Provider registry, auth, and model metadata |
| `/agents` | Agent CRUD |
| `/sessions` | Session CRUD and message execution |
| `/fleets` | Fleet CRUD and fan-out execution |
| `/formations` | Formation CRUD, export, execution, and report retrieval |

See [API](./api) for endpoint-level details.
