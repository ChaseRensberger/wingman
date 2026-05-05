---
title: "Streaming"
group: "WingHarness"
draft: false
order: 104
---

# Streaming

Wingman has two layers of streaming. This page covers the upper layer — loop lifecycle events, SDK consumption, and the SSE wire format. For the provider stream parts that make up a single assistant turn, see [Streaming](../wingmodels/streaming).

## Loop lifecycle events

The loop emits typed Go values on a `Sink`. Every event satisfies the closed `loop.Event` union.

| Type | Fires when |
|---|---|
| `IterationStartEvent` | Top of a turn, after `BeforeIteration`, before the LLM call |
| `IterationEndEvent` | After a turn (assistant + tool results) is appended |
| `MessageEvent` | After a complete message is appended to history |
| `ToolExecutionStartEvent` | Immediately before `Tool.Execute` |
| `ToolExecutionEndEvent` | After execution returns; in completion order |
| `StreamPartEvent` | Wraps a raw provider `StreamPart` |
| `ContextTransformedEvent` | `BeforeStep` or `TransformContext` changed the slice length |
| `ErrorEvent` | The loop is about to terminate with an error |

`MessageEvent` includes plugin-injected messages — when a plugin emits one through `info.Sink` from a `BeforeStep` hook, observers see it on the same channel as loop-produced messages.

## SDK consumption

`session.RunStream` returns a `*SessionStream`. It is single-consumer: the loop runs on a background goroutine and forwards every event to a buffered channel.

```go
stream, err := s.RunStream(ctx, "Write a Go HTTP server")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    ev := stream.Event()
    switch ev.Type {
    case "stream_part":
        if part, ok := ev.Data.(loop.StreamPartEvent); ok {
            // part.Part is a wingmodels.StreamPart
            _ = part
        }
    case "tool_start":
        // ev.Data is loop.ToolExecutionStartEvent
    case "message":
        // ev.Data is loop.MessageEvent
    }
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
result := stream.Result()
_ = result
```

If the consumer stops calling `Next`, the forwarding sink blocks and the loop stalls. Cancel the context to abort.

## SSE envelope

The HTTP server forwards `SessionStream` events as SSE. Each event is:

```text
event: <type>
data: <json>

```

`<json>` is the full envelope:

```json
{ "type": "tool_start", "version": 1, "data": { ... } }
```

`version` is the envelope schema version (currently `EnvelopeVersion = 1`). Bump only on breaking changes to the envelope itself; the `data` payload may evolve additively.

### Defined envelope types

| Envelope `type` | Inner `data` |
|---|---|
| `iteration_start` | `loop.IterationStartEvent` |
| `iteration_end` | `loop.IterationEndEvent` |
| `message` | `loop.MessageEvent` |
| `tool_start` | `loop.ToolExecutionStartEvent` |
| `tool_end` | `loop.ToolExecutionEndEvent` |
| `stream_part` | `loop.StreamPartEvent` (carries a `wingmodels.StreamPart`) |
| `compaction` | `loop.ContextTransformedEvent` (head Part type `compaction_marker`) |
| `context_transformed` | `loop.ContextTransformedEvent` (other transforms) |
| `error` | `{"error": "..."}` |

After the loop returns, the server writes one terminal envelope:

```text
event: done
data: {"type":"done","version":1,"data":{"usage":{...},"steps":N}}
```

The `compaction` envelope is a special case: the stream classifier inspects the head message's first part discriminator and surfaces `compaction_marker` as its own SSE event so UIs get a dedicated affordance. Other transforms ride the generic `context_transformed` event. The loop and the core remain ignorant of plugin Go types — only the discriminator string is consulted.

## Aborts

Cancelling the request context (or POSTing to `/sessions/{id}/abort` on the server) cancels the loop. The provider stream emits a final `finish` with `FinishReasonAborted`; the loop returns with `StopReasonAborted`; `Result.Response` reflects whatever text accumulated before the cancel; partial messages are still in history.

## Server timeouts

The standard 60-second request timeout is bypassed for `/sessions/{id}/message/stream`. The server tracks active streams in a `WaitGroup` and waits for them during graceful shutdown (subject to the shutdown context's deadline).