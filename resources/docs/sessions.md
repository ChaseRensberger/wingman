---
title: "Sessions"
group: "Primitives"
draft: true
order: 102
---
# Session

A session is a stateful container that maintains conversation history and executes the agentic loop. The agent (and its provider) define what model to use and how to run inference — the session just manages state and execution.

## SDK

```go
s := session.New(
    session.WithAgent(a),
    session.WithWorkDir("/path/to/workdir"),
)

result, err := s.Run(ctx, "Your message")
```

The `Run` method executes the agent loop: it sends the message, handles tool calls, and continues until the model produces a final response or the context is cancelled.

## Server

```
POST   /sessions              # Create session
GET    /sessions              # List sessions
GET    /sessions/{id}         # Get session
PUT    /sessions/{id}         # Update session
DELETE /sessions/{id}         # Delete session
POST   /sessions/{id}/message        # Send message (blocking)
POST   /sessions/{id}/message/stream # Send message (streaming SSE)
```

On every message, the server reconstructs a `session.Session` from the stored history, runs inference, then persists the updated history back to SQLite.

```bash
# Create a session
curl -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}'
```

### Send a message (blocking)

Runs the agent loop and returns the full result when complete.

```bash
curl -X POST http://localhost:2323/sessions/01XYZ.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "01ABC...",
    "message": "What files are in this directory?"
  }'
```

**Response:**
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

### Send a message (streaming)

Streams events as SSE. Use `Accept: text/event-stream` and `-N` to disable curl buffering.

```bash
curl -N -X POST http://localhost:2323/sessions/01XYZ.../message/stream \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent_id": "01ABC...",
    "message": "Explain this codebase."
  }'
```

**Event stream:**
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

Event types: `message_start`, `content_block_start`, `text_delta`, `input_json_delta`, `content_block_stop`, `message_delta`, `message_stop`, `ping`, `error`, `done`
