---
title: "Sessions"
group: "Concepts"
draft: false
order: 102
---

# Sessions

A session is the stateful execution context for a Wingman agent. It owns conversation history, the active model, system prompt, tool registry, working directory, installed plugins, and an optional message sink.

`*session.Session` is a thin wrapper over `wingagent/loop`. It holds state; the loop does the work.

## What `Run` actually does

When you call `Run` or `RunStream`, the session:

1. Locks state and snapshots the current model, system, tools, work dir, plugins, raw hooks, and message sink.
2. Appends the user message to history.
3. Builds a fresh plugin `Registry` and folds it into composed `loop.Hooks`, a merged tool slice, and a fan-out `Sink`.
4. Composes any user-supplied raw `BeforeStep` / `TransformContext` hooks after the plugin chain (last-wins for the user's seam).
5. Drives `loop.Run`. The loop streams from the model, executes tool calls, and emits typed events.
6. Adopts the loop's terminal message slice wholesale — so plugin mutations (e.g. compaction markers) end up in history.
7. Returns a `*Result` with the final text, all tool calls in completion order, cumulative usage, step count, and `StopReason`.

`Run` always returns a non-nil `*Result` even when the error is non-nil, so callers can persist partial state.

## Construction

```go
import (
    "github.com/chaserensberger/wingman/wingagent/session"
    "github.com/chaserensberger/wingman/tool"
    "github.com/chaserensberger/wingman/wingmodels/providers/anthropic"
)

p, _ := anthropic.New(anthropic.Config{})

s := session.New(
    session.WithModel(p),
    session.WithSystem("You are a helpful coding assistant."),
    session.WithTools(tool.NewBashTool(), tool.NewReadTool()),
    session.WithWorkDir("/path/to/project"),
)
```

A bare `New()` session has an empty history, no model, no tools, no plugins, and no hooks. `Run` returns `ErrNoModel` until a model is set.

## Multi-turn

Keep the same `*session.Session` alive across calls.

```go
r1, _ := s.Run(ctx, "What is 2 + 2?")
r2, _ := s.Run(ctx, "Multiply that by 10")
```

History grows across turns. Use `History()` to snapshot it (returned as a copy), `AddMessage` / `SetHistory` to rehydrate from storage, and `Clear` to reset.

## Result

```go
type Result struct {
    Response   string
    ToolCalls  []ToolCallResult
    Usage      wingmodels.Usage
    Steps      int
    StopReason loop.StopReason
}
```

`StopReason` is one of `end_turn`, `max_steps`, `aborted`, or `error`. `ToolCalls` is in execution-completion order across every turn of this run.

## Streaming

`RunStream` runs the loop on a background goroutine and exposes a single-consumer iterator. See [Streaming](../wingagent/streaming).

```go
stream, err := s.RunStream(ctx, "Write a Go HTTP server")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    ev := stream.Event()
    fmt.Printf("%s\t%v\n", ev.Type, ev.Data)
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
result := stream.Result()
_ = result
```

If the consumer stops calling `Next`, the loop blocks on its event channel. Cancel the context to abort.

## Message sink

`session.WithMessageSink(fn)` registers a synchronous callback fired for every complete message added to history during a run — including plugin-injected messages such as compaction markers. Sinks must not block. The HTTP server uses this to persist messages incrementally (`store.AppendMessage`) so partial transcripts survive crashes.

## Server behavior

On the server, sessions are rebuilt per request: history is loaded from SQLite, replayed into a fresh `*session.Session`, the message sink is wired to incremental persistence, and the loop runs. After it returns, the session adopts the loop's terminal history; storage already has each message appended.

```text
POST   /sessions
GET    /sessions
GET    /sessions/{id}
PUT    /sessions/{id}
DELETE /sessions/{id}
POST   /sessions/{id}/message
POST   /sessions/{id}/message/stream
POST   /sessions/{id}/abort
```

`PUT /sessions/{id}` is metadata-only (title, work dir, updated timestamp); it does not touch history.

### Send a message

```bash
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt_...",
    "message": "What files are in this directory?"
  }'
```

### Stream a message

```bash
curl -N -X POST http://localhost:2323/sessions/ses_.../message/stream \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent_id": "agt_...",
    "message": "Explain this codebase."
  }'
```

### Abort

```bash
curl -sS -X POST http://localhost:2323/sessions/ses_.../abort
```

Aborts every in-flight Run for the session. Idempotent.
