---
title: "Streaming Events"
description: "Consume Wingman's server-sent event stream."
---

# Streaming Events

Wingman uses server-sent events for session updates. Clients start work with the message endpoint and watch session state through the event stream.

## Start Work

Start a persistent session run:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt_...",
    "message": "Summarize this project."
  }'
```

## Subscribe

Subscribe to session events:

```bash
curl -N "http://localhost:2323/sessions/${SESSION_ID}/events?after=0" \
  -H "Accept: text/event-stream"
```

Wingman sends SSE frames:

```text
id: <event-id-or-seq>
event: <type>
data: <json>

```

## Cursor

`after` is an exclusive session sequence. If a client last processed sequence `42`, it reconnects with:

```text
GET /sessions/{id}/events?after=42
```

The server sends stored durable events after `42`, keeps the stream open, and then sends new events as they happen.

Use the history endpoint when a client needs a finite page instead of an open stream:

```text
GET /sessions/{id}/events/history?after=<seq>&limit=<n>
```

## Event Envelope

Each event is a JSON object in the SSE `data:` body:

```json
{
  "id": "evt_...",
  "type": "session.message.created",
  "time": "2026-07-03T12:00:00Z",
  "cursor": {
    "session_id": "ses_...",
    "seq": 43
  },
  "data": {
    "run_id": "run_...",
    "message": {
      "role": "assistant",
      "content": []
    }
  }
}
```

Field meanings:

| Field | Meaning |
|---|---|
| `id` | Unique event ID. |
| `type` | Event type. Also used as the SSE event name. |
| `time` | Event timestamp. |
| `cursor` | Resume position. Present only for durable session events. |
| `data` | Event-specific payload. |

For live-only events without `cursor`, the SSE `id` is the event ID.

## Durable Events

Durable events are stored and replayed. They reconstruct the transcript and final run state after a reconnect.

| Event | Meaning |
|---|---|
| `session.run.started` | A session run started. |
| `session.step.started` | A model/tool loop step started. |
| `session.step.completed` | A model/tool loop step completed. |
| `session.text.completed` | A text block reached its final value. |
| `session.reasoning.completed` | A reasoning block reached its final value. |
| `session.tool.called` | The model requested a tool. |
| `session.tool.completed` | A tool finished successfully. |
| `session.tool.failed` | A tool failed. |
| `session.message.created` | A message was appended to history. |
| `session.structured_output.completed` | Output schema parsing succeeded. |
| `session.run.completed` | The run finished successfully. |
| `session.run.failed` | The run failed. |

Durable events store boundaries, not token streams. A completed text event stores the final text for that block; it does not store every partial token delta.

## Live Events

Live events are not replayed. They exist for latency-sensitive rendering while the client is connected.

| Event | Meaning |
|---|---|
| `session.text.delta` | Partial assistant text. |
| `session.reasoning.delta` | Partial reasoning text. |
| `session.tool.input.delta` | Partial tool-call input. |

Live events include the run and step needed to merge them with the later durable boundary event:

```json
{
  "id": "evt_...",
  "type": "session.text.delta",
  "time": "2026-07-03T12:00:00Z",
  "data": {
    "run_id": "run_...",
    "step": 1,
    "delta": "partial text"
  }
}
```

## Recovery

A client only needs a session ID and last durable sequence:

```text
last_seq = load_checkpoint(session_id)

connect /sessions/{id}/events?after=last_seq

for each event:
  apply event
  if event.cursor exists:
    save_checkpoint(event.cursor.seq)

on disconnect:
  reconnect with the saved checkpoint
```

## Transport

SSE responses set these headers:

```text
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
X-Accel-Buffering: no
X-Content-Type-Options: nosniff
```

Idle streams send heartbeat comments:

```text
: heartbeat

```

Errors are events with JSON payloads:

```text
event: session.run.failed
data: {"id":"evt_...","type":"session.run.failed","data":{"error":"model stream failed"}}

```

## `stream_part`

`stream_part` is not part of this contract.

The old stream exposed provider stream parts through one public event type:

```text
event: stream_part
```

Clients then inspected nested provider data such as `part.type: "text_delta"`.

The SSE contract exposes session events directly. Provider-specific metadata belongs inside event payloads when a client needs it.
