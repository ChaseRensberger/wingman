---
title: "API"
group: "Reference"
draft: false
order: 1000
---

# API

Base URL: `http://localhost:2323`

All endpoints return JSON unless noted otherwise. Error responses use the shape `{"error": "..."}`.

## Conventions

- request bodies are JSON unless a formation definition is being sent as YAML
- standard request timeout is 60 seconds
- SSE endpoints bypass the standard timeout
- streaming endpoints return `text/event-stream`

## Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |

Example response:

```json
{
  "status": "ok"
}
```

## Provider endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/provider` | List registered providers |
| `GET` | `/provider/{id}` | Get provider metadata |
| `GET` | `/provider/{id}/models` | List models for a provider |
| `GET` | `/provider/{id}/models/{model}` | Get model metadata |
| `GET` | `/provider/auth` | Get configured credential status |
| `PUT` | `/provider/auth` | Set credentials for one or more providers |
| `DELETE` | `/provider/auth/{provider}` | Remove credentials for a provider |

Example auth request:

```json
{
  "providers": {
    "anthropic": {
      "type": "api_key",
      "key": "sk-ant-..."
    }
  }
}
```

Example auth response:

```json
{
  "providers": {
    "anthropic": {
      "type": "api_key",
      "configured": true
    }
  },
  "updated_at": "2026-02-21T00:00:00Z"
}
```

## Agent endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/agents` | Create agent |
| `GET` | `/agents` | List agents |
| `GET` | `/agents/{id}` | Get agent |
| `PUT` | `/agents/{id}` | Update agent |
| `DELETE` | `/agents/{id}` | Delete agent |

Create request example:

```json
{
  "name": "Assistant",
  "instructions": "Be helpful",
  "tools": ["bash", "read", "write", "edit", "glob", "grep"],
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "output_schema": null
}
```

## Session endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/sessions` | Create session |
| `GET` | `/sessions` | List sessions |
| `GET` | `/sessions/{id}` | Get session including history |
| `PUT` | `/sessions/{id}` | Update session |
| `DELETE` | `/sessions/{id}` | Delete session |
| `POST` | `/sessions/{id}/message` | Send message and wait for final result |
| `POST` | `/sessions/{id}/message/stream` | Send message and stream SSE events |

Message request example:

```json
{
  "agent_id": "01ABC...",
  "message": "Write a Python script"
}
```

Blocking response example:

```json
{
  "response": "Here is the script...",
  "tool_calls": [
    {
      "tool_name": "toolu_abc123",
      "output": "...",
      "steps": 1
    }
  ],
  "usage": {
    "input_tokens": 120,
    "output_tokens": 45
  },
  "steps": 2
}
```

SSE events are serialized `StreamEvent` payloads such as `text_delta`, `message_stop`, and `done`.

## Fleet endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/fleets` | Create fleet definition |
| `GET` | `/fleets` | List fleets |
| `GET` | `/fleets/{id}` | Get fleet definition |
| `PUT` | `/fleets/{id}` | Update fleet definition |
| `DELETE` | `/fleets/{id}` | Delete fleet definition |
| `POST` | `/fleets/{id}/run` | Run fleet and return all results |
| `POST` | `/fleets/{id}/run/stream` | Run fleet and stream worker results |

Run request example:

```json
{
  "tasks": [
    {"message": "Explore this dir", "work_dir": "/src/auth", "data": "auth"},
    {"message": "Explore this dir", "work_dir": "/src/api", "data": "api"}
  ]
}
```

Streaming emits one `result` event per completed worker followed by `done`.

## Formation endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/formations` | Create formation definition |
| `GET` | `/formations` | List formations |
| `GET` | `/formations/{id}` | Get formation |
| `PUT` | `/formations/{id}` | Update formation |
| `DELETE` | `/formations/{id}` | Delete formation |
| `GET` | `/formations/{id}/export` | Export definition as JSON or YAML |
| `POST` | `/formations/{id}/run` | Run formation |
| `POST` | `/formations/{id}/run/stream` | Run formation with SSE |
| `GET` | `/formations/{id}/report` | Read `report.md` from the formation work dir |

Create accepts either `application/json` or `application/x-yaml`.

Run request example:

```json
{
  "inputs": {
    "topic": "State of local inference in 2026"
  }
}
```

Streaming events include `run_start`, `node_start`, `tool_call`, `node_output`, `edge_emit`, `node_end`, `node_error`, and `run_end`.
