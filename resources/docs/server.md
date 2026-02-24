---
title: "Server"
group: "Usage"
order: 10
---
# Server

The HTTP server is one way to use Wingman. Unlike [the SDK](./sdk), it includes SQLite-backed persistence for agents, sessions, and fleets.

## Installation

See the project README for installation instructions.

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

The server reads credentials only from its SQLite auth store (not environment variables).

```bash
curl -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'
```

## Streaming

For streaming responses, use the `/message/stream` endpoint. Events are sent as SSE:

```bash
curl -X POST http://localhost:2323/sessions/{id}/message/stream \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "01ABC...", "message": "Hello"}'
```

Events: `message_start`, `content_block_start`, `text_delta`, `input_json_delta`, `content_block_stop`, `message_delta`, `message_stop`, `ping`, `error`, `done`

## Example Workflow

```bash
# 1. Configure auth
curl -X PUT http://localhost:2323/provider/auth \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'

# 2. Create an agent
curl -X POST http://localhost:2323/agents \
  -d '{
    "name": "Assistant",
    "instructions": "Be helpful",
    "tools": ["bash"],
    "provider": "anthropic",
    "model": "claude-sonnet-4-5",
    "options": {
      "max_tokens": 4096
    }
  }'

# 3. Create a session
curl -X POST http://localhost:2323/sessions \
  -d '{"work_dir": "/tmp"}'

# 4. Send messages
curl -X POST http://localhost:2323/sessions/01XYZ.../message \
  -d '{"agent_id": "01ABC...", "message": "What OS am I on?"}'
```

---

## Routes

### Health

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |

### Provider

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/provider` | List all available providers from wingman registry |
| `GET` | `/provider/auth` | Get authentication status for configured providers |
| `PUT` | `/provider/auth` | Set provider authentication credential(s) |
| `DELETE` | `/provider/auth/{provider}` | Remove authentication for a provider |
| `GET` | `/provider/{name}` | Get provider details |
| `GET` | `/provider/{name}/models` | List all models for a provider |
| `GET` | `/provider/{name}/models/{model}` | Get details for a specific model |

### Agents

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/agents` | Create a new agent |
| `GET` | `/agents` | List all agents |
| `GET` | `/agents/{id}` | Get an agent by ID |
| `PUT` | `/agents/{id}` | Update an agent |
| `DELETE` | `/agents/{id}` | Delete an agent |

### Sessions

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/sessions` | Create a new session |
| `GET` | `/sessions` | List all sessions |
| `GET` | `/sessions/{id}` | Get a session by ID |
| `PUT` | `/sessions/{id}` | Update a session |
| `DELETE` | `/sessions/{id}` | Delete a session |
| `POST` | `/sessions/{id}/message` | Send a message and get a response |
| `POST` | `/sessions/{id}/message/stream` | Send a message and stream the response (SSE) |
