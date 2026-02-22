---
title: "API"
order: 0
draft: true
---
# API

Base URL: `http://localhost:2323`

## Health

### GET /health

**Request:**
```bash
curl -sS -X GET http://localhost:2323/health | jq .
```

**Example Response:**
```json
{
  "status": "ok"
}
```

## Provider

### GET /provider

**Request:**
```bash
curl -sS -X GET http://localhost:2323/provider | jq .
```

**Example Response:**
```json
[
  {
    "id": "anthropic",
    "name": "Anthropic",
    "auth_types": [
        "api_key"
    ]
  },
  {
    "id": "ollama",
    "name": "Ollama",
    "auth_types": []
  }
]
```

### GET /provider/auth

**Request:**
```bash
curl -sS -X GET http://localhost:2323/provider/auth | jq .
```

**Example Response:**
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

### PUT /provider/auth

**Request:**
```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}' | jq .
```

**Example Response:**
```json
{"status":"ok"}
```

### DELETE /provider/auth/{provider}

**Request:**
```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic | jq .
```

**Example Response:**
```json
{"status":"deleted"}
```

### GET /provider/{name}

**Request:**
```bash
curl -sS -X GET http://localhost:2323/provider/anthropic | jq .
```

**Example Response:**
```json
{
  "id": "anthropic",
  "name": "Anthropic",
  "type": "chat",
  "models": true
}
```

### GET /provider/{name}/models

**Request:**
```bash
curl -sS -X GET http://localhost:2323/provider/anthropic/models | jq .
```

**Example Response:**
```json
[
  {
    "id": "claude-sonnet-4-5",
    "name": "Claude Sonnet 4.5"
  },
  {
    "id": "claude-opus-4-1",
    "name": "Claude Opus 4.1"
  }
]
```

### GET /provider/{name}/models/{model}

**Request:**
```bash
curl -sS -X GET http://localhost:2323/provider/anthropic/models/claude-sonnet-4-5 | jq .
```

**Example Response:**
```json
{
  "id": "claude-sonnet-4-5",
  "name": "Claude Sonnet 4.5",
  "context_window": 200000
}
```

## Agents

### GET /agents

**Request:**
```bash
curl -sS -X GET http://localhost:2323/agents | jq .
```

**Example Response:**
```json
[
  {
    "id": "01ABC...",
    "name": "Assistant",
    "instructions": "Be helpful",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": {
      "id": "anthropic",
      "model": "claude-sonnet-4-5",
      "max_tokens": 4096,
      "temperature": 0.7
    },
    "created_at": "2026-02-21T00:00:00Z",
    "updated_at": "2026-02-21T00:00:00Z"
  }
]
```

### POST /agents

**Request:**
```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Assistant",
    "instructions": "Be helpful",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": {
      "id": "anthropic",
      "model": "claude-sonnet-4-5",
      "max_tokens": 4096,
      "temperature": 0.7
    }
  }' | jq .
```

**Example Response:**
```json
{
  "id": "01ABC...",
  "name": "Assistant",
  "instructions": "Be helpful",
  "tools": ["bash", "read", "write", "edit", "glob", "grep"],
  "provider": {
    "id": "anthropic",
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "created_at": "2026-02-21T00:00:00Z",
  "updated_at": "2026-02-21T00:00:00Z"
}
```

### GET /agents/{id}

**Request:**
```bash
curl -sS -X GET http://localhost:2323/agents/01ABC... | jq .
```

**Example Response:**
```json
{
  "id": "01ABC...",
  "name": "Assistant",
  "instructions": "Be helpful",
  "tools": ["bash", "read", "write", "edit", "glob", "grep"],
  "provider": {
    "id": "anthropic",
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "created_at": "2026-02-21T00:00:00Z",
  "updated_at": "2026-02-21T00:00:00Z"
}
```

### PUT /agents/{id}

**Request:**
```bash
curl -sS -X PUT http://localhost:2323/agents/01ABC... \
  -H "Content-Type: application/json" \
  -d '{
    "instructions": "You are a fast, practical coding assistant.",
    "tools": ["bash", "read", "edit", "glob", "grep"]
  }' | jq .
```

**Example Response:**
```json
{
  "id": "01ABC...",
  "name": "Assistant",
  "instructions": "You are a fast, practical coding assistant.",
  "tools": ["bash", "read", "edit", "glob", "grep"],
  "provider": {
    "id": "anthropic",
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "created_at": "2026-02-21T00:00:00Z",
  "updated_at": "2026-02-21T00:00:00Z"
}
```

### DELETE /agents/{id}

**Request:**
```bash
curl -sS -X DELETE http://localhost:2323/agents/01ABC... | jq .
```

**Example Response:**
```json
{"status":"deleted"}
```

## Sessions

### POST /sessions

**Request:**
```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}' | jq .
```

**Example Response:**
```json
{
  "id": "01XYZ...",
  "work_dir": "/path/to/project",
  "history": [],
  "created_at": "2026-02-21T00:00:00Z",
  "updated_at": "2026-02-21T00:00:00Z"
}
```

### GET /sessions

**Request:**
```bash
curl -sS -X GET http://localhost:2323/sessions | jq .
```

**Example Response:**
```json
[
  {
    "id": "01XYZ...",
    "work_dir": "/path/to/project",
    "history": [],
    "created_at": "2026-02-21T00:00:00Z",
    "updated_at": "2026-02-21T00:00:00Z"
  }
]
```

### GET /sessions/{id}

**Request:**
```bash
curl -sS -X GET http://localhost:2323/sessions/01XYZ... | jq .
```

**Example Response:**
```json
{
  "id": "01XYZ...",
  "work_dir": "/path/to/project",
  "history": [],
  "created_at": "2026-02-21T00:00:00Z",
  "updated_at": "2026-02-21T00:00:00Z"
}
```

### PUT /sessions/{id}

**Request:**
```bash
curl -sS -X PUT http://localhost:2323/sessions/01XYZ... \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/new/workdir"}' | jq .
```

**Example Response:**
```json
{
  "id": "01XYZ...",
  "work_dir": "/path/to/new/workdir",
  "history": [],
  "created_at": "2026-02-21T00:00:00Z",
  "updated_at": "2026-02-21T00:00:00Z"
}
```

### DELETE /sessions/{id}

**Request:**
```bash
curl -sS -X DELETE http://localhost:2323/sessions/01XYZ... | jq .
```

**Example Response:**
```json
{"status":"deleted"}
```

### POST /sessions/{id}/message

**Request:**
```bash
curl -sS -X POST http://localhost:2323/sessions/01XYZ.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "01ABC...",
    "message": "What files are in this directory?"
  }' | jq .
```

**Example Response:**
```json
{
  "response": "Here are the files in the directory...",
  "tool_calls": [],
  "usage": {
    "input_tokens": 120,
    "output_tokens": 240
  },
  "steps": 1
}
```

### POST /sessions/{id}/message/stream

**Request:**
```bash
curl -N -sS -X POST http://localhost:2323/sessions/01XYZ.../message/stream \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent_id": "01ABC...",
    "message": "Stream a response."
  }' | jq -R .
```

**Example Response:**
```text
event: text
data: {"type":"text","content":"Hello"}

event: text
data: {"type":"text","content":" world"}

event: done
data: {"usage":{"input_tokens":120,"output_tokens":240},"steps":1}
```
