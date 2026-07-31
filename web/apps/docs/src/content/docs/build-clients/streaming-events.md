---
title: "Streaming Events"
description: "Consume Wingman's server-sent event stream."
---

# Streaming Events

Wingman has two SSE contracts: persistent session events and the one-shot `POST /run` stream. Clients start persistent work with the message endpoint and watch session state through the session event stream.

Session SSE is a client delivery contract. It is separate from the internal
aggregate event log described in [Durable Events and Projections](/concepts/durable-events),
which records session creation, metadata changes, and run admission to maintain
critical projections.

## Start Work

Start a persistent session run:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "submit-123",
    "agent_id": "agt_...",
    "message": "Summarize this project."
  }'
```

Persist `request_id` before sending if the client may retry an uncertain
response. An identical retry returns the existing run and does not create a
second `session.run.queued` event. The accepted response includes `run_id`,
`status`, and `session_version`.

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

The server first sends at most one bounded page of stored durable events after `42` (default `100`, maximum `500`), keeps the stream open, and then sends events created after subscription. It does not backfill an older backlog beyond that initial page over the live connection.

Use the history endpoint when a client needs a finite page instead of an open stream:

```text
GET /sessions/{id}/events/history?after=<seq>&limit=<n>
```

The history response is `{ "data": [...], "has_more": <boolean> }`. `limit` has the same default and maximum. Advance `after` to the last returned durable cursor and request another page while `has_more` is true; `has_more` means the returned page reached the limit, so a final follow-up may be empty.

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
      "id": "msg_...",
      "revision": 8,
      "state": "completed",
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
| `session.run.queued` | A message run was durably queued. |
| `session.run.started` | A session run started. |
| `session.step.started` | A model/tool loop step started. |
| `session.step.completed` | A model/tool loop step completed. |
| `session.text.completed` | A text block reached its final value. |
| `session.reasoning.completed` | A reasoning block reached its final value. |
| `session.tool.called` | The model requested a tool. |
| `session.tool.updated` | A tool reached `proposed`, `authorized`, `started`, `completed`, `failed`, `interrupted`, or `declined`, including its latest durable input, output, metadata, error, and timing. |
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
| `session.tool.progress` | Incremental tool output and metadata reported during execution. |

Live provider events include the correlation identifiers needed to merge them
with the later durable boundary event. Text, reasoning, and tool-input deltas
include `run_id`, `step`, `message_id`, `part_id`, and the message `revision` at
which the delta was checkpointed. Tool progress also includes `call_id`:

```json
{
  "id": "evt_...",
  "type": "session.text.delta",
  "time": "2026-07-03T12:00:00Z",
  "data": {
    "run_id": "run_...",
    "step": 1,
    "message_id": "msg_...",
    "part_id": "prt_...",
    "revision": 3,
    "delta": "partial text"
  }
}
```

Before proposal, tool-input deltas use `run_id` plus provider `call_id` as a
temporary correlation key. Once `session.tool.called` or
`session.tool.updated` supplies `tool_use_id`, migrate that transient state to
the durable ID. Provider call IDs may repeat across runs and must not be used as
session-global identity.

Tool progress includes `tool_use_id`. Append `output_delta` to the visible
output and shallow-merge `metadata`; replace both with values from the later
durable `session.tool.updated` event. Treat that event's exact lifecycle status
as authoritative.

Treat `session.message.created` as the authoritative snapshot. Replace an
existing message with the same `message.id` when its `revision` is newer or
equal; do not append a duplicate after replay. Ignore an older revision. Apply
the same identity rule to parts within the message. A `failed` message may
contain useful partial text, reasoning, or tool input.

## Recovery

A client only needs a session ID and last durable sequence. If it may have missed more than one page, page through history before opening the live stream:

```text
last_seq = load_checkpoint(session_id)

while true:
  page = GET /sessions/{id}/events/history?after=last_seq
  apply page.data
  last_seq = last durable cursor in page, if any
  if not page.has_more: break

connect /sessions/{id}/events?after=last_seq

for each event:
  apply event
  if event.cursor exists:
    save_checkpoint(event.cursor.seq)

on disconnect:
  reconnect with the saved checkpoint
```

## Transport

Persistent session SSE responses set these headers:

```text
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
X-Accel-Buffering: no
X-Content-Type-Options: nosniff
```

`POST /run` sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`, and `Connection: keep-alive`.

Idle streams send heartbeat comments:

```text
: heartbeat

```

Persistent run failures are durable `session.run.failed` events with JSON envelopes; they are not transport-terminal errors. The persistent connection stays open after `session.run.completed` or `session.run.failed` so it can carry later queued runs for the session.

## One-Shot `/run` Stream

`POST /run` returns a separate, non-persistent SSE stream. It forwards the raw session stream event names, including `stream_part`; its JSON `data:` value is the raw stream envelope with `type`, `version`, and event-specific `data`. These events are not rewritten into `session.*` events and cannot be replayed.

On success, `/run` sends a terminal `done` event containing usage and step information, then closes. If setup or streaming fails, it sends a terminal `error` event and closes; its `data:` is currently error text rather than a JSON event envelope. A raw `error` event can also be forwarded from the underlying run before the terminal outcome.

## `stream_part`

`stream_part` is only part of the `/run` contract. It carries provider stream parts such as text and tool-input deltas:

```text
event: stream_part
```

Persistent session SSE never emits `stream_part`; it translates supported provider parts into the live `session.text.delta`, `session.reasoning.delta`, and `session.tool.input.delta` events described above. Use persistent SSE for recoverable session state and `/run` only when the raw one-shot stream is required.
