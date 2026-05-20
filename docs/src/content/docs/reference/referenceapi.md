---
title: "API"
group: "Reference"
order: 1000
---

# API

Base URL: `http://localhost:2323` (configurable via `--host` and `--port`).

All endpoints accept and return JSON unless noted. Error responses use the shape `{"error": "..."}`.

## Conventions

- Request bodies are JSON.
- Standard request timeout is 60 seconds.
- `POST /sessions/{id}/message/stream` bypasses the standard timeout and returns `text/event-stream`.
- ID prefixes are stable: `agt_` (agent), `ses_` (session), `msg_` (message), `prt_` (part), `tlu_` (tool use).

## Health

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Health check |

```json
{ "status": "ok" }
```

## Provider endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/provider` | List registered providers |
| `GET` | `/provider/{name}` | Get provider metadata |
| `GET` | `/provider/{name}/models` | List models for a provider |
| `GET` | `/provider/{name}/models/{model}` | Get model metadata |
| `GET` | `/provider/auth` | Get configured credential status |
| `PUT` | `/provider/auth` | Set credentials for one or more providers |
| `DELETE` | `/provider/auth/{provider}` | Remove credentials for a provider |

### Set auth

```json
{
  "providers": {
    "anthropic": { "type": "api_key", "key": "sk-ant-..." }
  }
}
```

### Auth response

`GET /provider/auth` returns a `configured` flag per provider without leaking the secret:

```json
{
  "providers": {
    "anthropic": { "type": "api_key", "configured": true }
  },
  "updated_at": "2026-04-25T00:00:00Z"
}
```

## Agent endpoints

| Method | Path | Description |
|---|---|---|
| `POST` | `/agents` | Create agent |
| `GET` | `/agents` | List agents |
| `GET` | `/agents/{id}` | Get agent |
| `PUT` | `/agents/{id}` | Update agent (omitted fields unchanged) |
| `DELETE` | `/agents/{id}` | Delete agent |

### Create request

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "tools": ["bash", "read", "write", "edit", "glob", "grep"],
  "model_ref": "anthropic/claude-sonnet-4-6",
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
| `PUT` | `/sessions/{id}` | Update session metadata (title, work_dir) |
| `DELETE` | `/sessions/{id}` | Delete session |
| `POST` | `/sessions/{id}/message` | Send a message and wait for the final result |
| `POST` | `/sessions/{id}/message/stream` | Send a message and stream SSE events |
| `POST` | `/sessions/{id}/abort` | Cancel every in-flight run for the session |
| `POST` | `/run` | Run one ephemeral session without persisting it |

`PUT /sessions/{id}` is metadata-only. Use the message endpoints to add content; rebuilding history is done by reposting messages, not by PUT.

`POST /sessions/{id}/message` and `POST /sessions/{id}/message/stream` require the session to exist. Unknown IDs return `404`; message endpoints do not create sessions implicitly.

### Create request

```json
{
  "title": "Explore repo",
  "working_directory": "/home/me/project"
}
```

### Message request

```json
{
  "agent_id": "agt_...",
  "message": "Write a Python script"
}
```

### Blocking response

```json
{
  "response": "Here is the script...",
  "tool_calls": [
    { "tool_name": "write", "input": {"path": "x.py"}, "output": "" }
  ],
  "usage": { "input_tokens": 120, "output_tokens": 45 },
  "steps": 2
}
```

### Streaming

`POST /sessions/{id}/message/stream` returns `text/event-stream`. Each event is:

```text
event: <type>
data: <json>

```

Where `<json>` is the envelope:

```json
{ "type": "tool_start", "version": 1, "data": { ... } }
```

The `type` is one of `iteration_start`, `iteration_end`, `message`, `tool_start`, `tool_end`, `stream_part`, `compaction`, `context_transformed`, `error`. After the loop returns, the server writes one terminal envelope:

```text
event: done
data: {"type":"done","version":1,"data":{"usage":{...},"steps":N}}
```

See [Streaming Events](/build-clients/streaming-events) for client-side streaming guidance.

### Abort response

```json
{ "session_id": "ses_...", "aborted": 2 }
```

`aborted` is the number of in-flight runs cancelled. Aborts are idempotent — a 200 with `aborted: 0` is returned when no run is in flight. A 404 is returned only when the session id is unknown.

### Ephemeral run request

`POST /run` creates an in-memory session, streams the run, and does not persist the session or its messages.

In normal persistent mode, pass either `agent_id` or an inline `agent`:

```json
{
  "agent_id": "agt_...",
  "message": "Summarize this project",
  "working_directory": "/home/me/project"
}
```

When the server is started with `--ephemeral`, persisted agents are unavailable, so pass an inline agent:

```json
{
  "agent": {
    "name": "One-shot Assistant",
    "instructions": "Be concise.",
    "tools": ["webfetch"],
    "model_ref": "anthropic/claude-sonnet-4-6"
  },
  "message": "Explain Wingman in one paragraph."
}
```
