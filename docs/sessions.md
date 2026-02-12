---
title: "Sessions"
group: "Primitives"
order: 102
---
# Session

A session is a stateful container that maintains conversation history and executes agent loops. The agent (and its provider) define what model to use and how to run inference — the session just manages state and execution.

## SDK

```go
s := session.New(
    session.WithAgent(a),
    session.WithWorkDir("/path/to/workdir"),
)

result, err := s.Run(ctx, "Your message")
```

The `Run` method executes the agent loop: it sends the message, handles tool calls, and continues until the model produces a final response or hits the step limit.

## Server

```
POST   /sessions              # Create session
GET    /sessions              # List sessions
GET    /sessions/{id}         # Get session
PUT    /sessions/{id}         # Update session
DELETE /sessions/{id}         # Delete session
POST   /sessions/{id}/message # Send message (blocking)
POST   /sessions/{id}/message/stream # Send message (streaming)
```

```bash
# Create a session
curl -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}'

# Send a message
curl -X POST http://localhost:2323/sessions/{id}/message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "01ABC...",
    "message": "What files are in this directory?"
  }'
```
