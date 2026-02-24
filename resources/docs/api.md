---
title: "API"
group: "Reference"
order: 1000
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
    "auth_types": ["api_key"]
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
{"status": "ok"}
```

### DELETE /provider/auth/{provider}

**Request:**
```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic | jq .
```

**Example Response:**
```json
{"status": "deleted"}
```

### GET /provider/{id}

**Request:**
```bash
curl -sS -X GET http://localhost:2323/provider/anthropic | jq .
```

**Example Response:**
```json
{
  "id": "anthropic",
  "name": "Anthropic",
  "auth_types": ["api_key"]
}
```

### GET /provider/{id}/models

Models are fetched from models.dev and cached for 1 hour.

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
    "id": "claude-opus-4-6",
    "name": "Claude Opus 4.6"
  }
]
```

### GET /provider/{id}/models/{model}

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
    "provider": "anthropic",
    "model": "claude-sonnet-4-5",
    "options": {
      "max_tokens": 4096,
      "temperature": 0.7
    },
    "output_schema": null,
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
    "provider": "anthropic",
    "model": "claude-sonnet-4-5",
    "options": {
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
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "output_schema": null,
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
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "output_schema": null,
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
  "provider": "anthropic",
  "model": "claude-sonnet-4-5",
  "options": {
    "max_tokens": 4096,
    "temperature": 0.7
  },
  "output_schema": null,
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
{"status": "deleted"}
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
{"status": "deleted"}
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
  "tool_calls": [
    { "tool_name": "toolu_abc123", "output": "...", "steps": 1 }
  ],
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
event: text_delta
data: {"type":"text_delta","text":"Hello ","index":0}

event: text_delta
data: {"type":"text_delta","text":"world","index":0}

event: message_stop
data: {"type":"message_stop"}

event: done
data: {"usage":{"input_tokens":120,"output_tokens":240},"steps":1}
```

## Fleets

For the conceptual/runtime guide, see [Fleets](./fleets).

### POST /fleets

Create a fleet definition.

### GET /fleets

List fleet definitions.

### GET /fleets/{id}

Get a fleet definition.

### PUT /fleets/{id}

Update a fleet definition.

### DELETE /fleets/{id}

Delete a fleet definition.

### POST /fleets/{id}/run

**Request:**
```json
{
  "tasks": [
    { "message": "Explore this dir", "work_dir": "/src/auth", "data": "auth" },
    { "message": "Explore this dir", "work_dir": "/src/api",  "data": "api" }
  ]
}
```

**Example Response:**
```json
[
  { "task_index": 0, "worker_name": "worker-0", "response": "...", "steps": 1, "data": "auth" },
  { "task_index": 1, "worker_name": "worker-1", "response": "...", "steps": 1, "data": "api" }
]
```

### POST /fleets/{id}/run/stream

Streams one `event: result` per worker, then `event: done`.

## Formations

For the conceptual/runtime guide, see [Formations](./formations).

### POST /formations

Create a formation definition (JSON or YAML).

### GET /formations

List formation definitions.

### GET /formations/{id}

Get a formation definition.

### PUT /formations/{id}

Update a formation definition.

### DELETE /formations/{id}

Delete a formation definition.

### GET /formations/{id}/export

Export stored definition as JSON or YAML (`?format=yaml`).

### POST /formations/{id}/run

Run a formation and return final outputs.

**Request:**
```json
{
  "inputs": {
    "topic": "State of local inference in 2026"
  }
}
```

**Example Response:**
```json
{
  "status": "ok",
  "outputs": {
    "planner": {"sections": []}
  },
  "stats": {
    "nodes_executed": 1,
    "duration_ms": 1234
  }
}
```

### POST /formations/{id}/run/stream

Stream formation lifecycle events over SSE.

Event types include: `run_start`, `node_start`, `tool_call`, `node_output`, `edge_emit`, `node_end`, `node_error`, `run_end`.

### GET /formations/{id}/report

Read `report.md` from the formation work directory.

**Example Response:**
```json
{
  "path": "./report.md",
  "content": "# Report\n..."
}
```
