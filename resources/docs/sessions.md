---
title: "Sessions"
group: "Primitives"
order: 102
---

# Sessions

A **Session** is a stateful container that maintains conversation history and executes the agent loop. The [agent](/docs/agents) (and its [provider](/docs/providers)) define what model to use — the session manages state and execution.

## SDK Usage

```go
s := session.New(
    session.WithAgent(a),
    session.WithWorkDir("/path/to/workdir"),
)

result, err := s.Run(ctx, "Your message")
```

`Run` executes the agent loop: send the message, handle tool calls, accumulate history, and repeat until the model produces a final response or hits the step limit.

### Streaming

```go
stream, err := s.RunStream(ctx, "Your message")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    event := stream.Event()
}

if err := stream.Err(); err != nil {
    log.Fatal(err)
}

result := stream.Result()
```

### Result Structure

```go
type Result struct {
    Response  string           // Final text response
    ToolCalls []ToolCallResult // All tool calls made during execution
    Usage     WingmanUsage     // Token usage (InputTokens, OutputTokens)
    Steps     int              // Number of inference steps
}
```

### Available Options

| Option | Description |
|--------|-------------|
| `WithAgent(a)` | The [agent](/docs/agents) to execute |
| `WithWorkDir(dir)` | Working directory for tool execution |

## Server Usage

```
POST   /sessions              # Create session
GET    /sessions              # List sessions
GET    /sessions/{id}         # Get session
PUT    /sessions/{id}         # Update session
DELETE /sessions/{id}         # Delete session
POST   /sessions/{id}/message # Send message (blocking)
POST   /sessions/{id}/message/stream # Send message (SSE streaming)
```

```bash
curl -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}'
```

```bash
curl -X POST http://localhost:2323/sessions/{id}/message \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "01ABC...", "message": "What files are in this directory?"}'
```

The streaming endpoint sends Server-Sent Events with the following event types: `text`, `tool_use`, `tool_result`, `done`, `error`.
