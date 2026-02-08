---
title: "Sessions"
group: "Primitives"
order: 102
---
# Session

At least for the moment, nothing gets done in Wingman without a session. A session is what you use to combine an agent and a provider. Fundamentally, it is a stateful container that maintaines conversation history and executes agent loops.

## SDK

```go
s := session.New(
    session.WithAgent(a),
    session.WithProvider(p),
    session.WithWorkDir("/path/to/workdir"),
)

result, err := s.Run(ctx, "Your user message to add to the history")
```

The `Run` method executes the agent loop: it sends the prompt, handles tool calls, and continues until the model produces a final response or hits `MaxSteps`.


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

