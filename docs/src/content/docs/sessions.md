---
title: "Sessions"
group: "Concepts"
draft: false
order: 102
---

# Sessions

A session is the stateful execution context for an agent. It owns conversation history, optional working directory, and the tool-calling loop.

## What a session does

When you call `Run` or `RunStream`, the session:

1. appends the user message to history
2. builds an inference request from history, instructions, tools, and output schema
3. calls the provider
4. appends the assistant response
5. executes any requested tools and appends tool results
6. repeats until the model returns a final answer

## SDK

```go
s := session.New(
    session.WithAgent(a),
    session.WithWorkDir("/path/to/project"),
)

result, err := s.Run(ctx, "Explain this codebase")
```

Keep the same `*session.Session` alive if you want multi-turn context.

## Blocking result

Blocking execution returns a `Result` with:

- final response text
- all tool calls made across the run
- aggregate token usage
- number of inference steps

## Streaming

`RunStream` emits incremental events while the agent loop is running.

```go
stream, err := s.RunStream(ctx, "Write a Go HTTP server")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    event := stream.Event()
    if event.Type == core.EventTextDelta {
        fmt.Print(event.Text)
    }
}
```

## Server behavior

The server persists session history in SQLite, but execution remains ephemeral. On each message request, the server rebuilds a fresh `session.Session`, replays stored history into it, runs the loop, and persists the updated history back to storage.

```text
POST   /sessions
GET    /sessions
GET    /sessions/{id}
PUT    /sessions/{id}
DELETE /sessions/{id}
POST   /sessions/{id}/message
POST   /sessions/{id}/message/stream
```

### Create a session

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}'
```

### Send a message

```bash
curl -sS -X POST http://localhost:2323/sessions/01XYZ.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "01ABC...",
    "message": "What files are in this directory?"
  }'
```

### Stream a message

```bash
curl -N -X POST http://localhost:2323/sessions/01XYZ.../message/stream \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent_id": "01ABC...",
    "message": "Explain this codebase."
  }'
```

Streaming responses are sent as SSE `StreamEvent` payloads followed by a final `done` event.
