---
title: "Server"
group: "Usage"
order: 10
---

# Server

The HTTP server is the primary way to use Wingman. Unlike [the SDK](/docs/sdk), it comes batteries-included with SQLite persistence and a config file at `~/.config/wingman/`.

## Installation

```bash
curl -fsSL https://wingman.actor/install.sh | sh
```

## Starting the Server

```bash
wingman serve
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 2323 | Port to listen on |
| `--host` | 127.0.0.1 | Host to bind to |
| `--db` | ~/.local/share/wingman/wingman.db | Database path |

## Quick Start

```bash
# 1. Configure provider auth
curl -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'

# 2. Create an agent
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Assistant",
    "instructions": "Be helpful",
    "tools": ["bash"],
    "provider": {
      "id": "anthropic",
      "model": "claude-sonnet-4-5",
      "max_tokens": 4096
    }
  }'

# 3. Create a session
curl -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/tmp"}'

# 4. Send a message (use the IDs returned from steps 2 and 3)
curl -X POST http://localhost:2323/sessions/{session_id}/message \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "{agent_id}", "message": "What OS am I on?"}'
```

## Streaming

Use the `/message/stream` endpoint for Server-Sent Events:

```bash
curl -X POST http://localhost:2323/sessions/{id}/message/stream \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "01ABC...", "message": "Hello"}'
```

Event types: `text`, `tool_use`, `tool_result`, `done`, `error`

## Route Reference

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |

### Providers

See [Providers](/docs/providers) for details on discovery and auth management.

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/provider` | List available providers |
| `GET` | `/provider/auth` | Get auth status |
| `PUT` | `/provider/auth` | Set provider credentials |
| `DELETE` | `/provider/auth/{provider}` | Remove provider credentials |
| `GET` | `/provider/{name}` | Get provider details |
| `GET` | `/provider/{name}/models` | List models for a provider |
| `GET` | `/provider/{name}/models/{model}` | Get model details |

### Agents

See [Agents](/docs/agents) for payload details and options.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/agents` | Create agent |
| `GET` | `/agents` | List agents |
| `GET` | `/agents/{id}` | Get agent |
| `PUT` | `/agents/{id}` | Update agent |
| `DELETE` | `/agents/{id}` | Delete agent |

### Sessions

See [Sessions](/docs/sessions) for payload details and message handling.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/sessions` | Create session |
| `GET` | `/sessions` | List sessions |
| `GET` | `/sessions/{id}` | Get session |
| `PUT` | `/sessions/{id}` | Update session |
| `DELETE` | `/sessions/{id}` | Delete session |
| `POST` | `/sessions/{id}/message` | Send message (blocking) |
| `POST` | `/sessions/{id}/message/stream` | Send message (SSE) |
