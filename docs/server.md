---
title: "Server"
group: "Usage"
order: 10
---
# Server

The HTTP server is the primary way to use Wingman. Unliked [the SDK](https://wingman.actor/sdk) it comes batteries included with object persistence (via sqlite3) and a config file at `~/.config/wingman/`.

## Installation

## Starting the Server

```bash
wingman serve
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 2323 | Port to listen on |
| `--host` | 127.0.0.1 | Host to bind to |
| `--db` | ~/.local/share/wingman/wingman.db | Database path |

## Authentication

Before making inference requests, configure your provider API keys:

```bash
curl -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'
```

#### Streaming

For streaming responses, use the `/message/stream` endpoint. Events are sent as SSE:

```bash
curl -X POST http://localhost:2323/sessions/{id}/message/stream \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "01ABC...", "prompt": "Hello"}'
```

Events: `text`, `tool_use`, `tool_result`, `done`, `error`

## Example Workflow

```bash
# 1. Configure auth
curl -X PUT http://localhost:2323/provider/auth \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'

# 2. Create an agent
curl -X POST http://localhost:2323/agents \
  -d '{"name": "Assistant", "instructions": "Be helpful", "tools": ["bash"]}'

# 3. Create a session
curl -X POST http://localhost:2323/sessions \
  -d '{"work_dir": "/tmp"}'

# 4. Send messages
curl -X POST http://localhost:2323/sessions/01XYZ.../message \
  -d '{"agent_id": "01ABC...", "prompt": "What OS am I on?"}'
```
