---
title: "Streaming Events"
description: "Consume Wingman's server-sent event stream."
---

# Streaming Events

Wingman has two SSE contracts: persistent session events and the one-shot `POST /run` stream. Clients start persistent work with the message endpoint and watch session state through the session event stream.

These examples use `WINGMAN_DAEMON_PASSWORD` with HTTP Basic authentication. See [HTTP API Basics](/build-clients/http-api-basics#authentication).

## Start Work

Start a persistent session run:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
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
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
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

Clients may instead send `Last-Event-ID: 42`. An explicit `after` query takes
precedence when both are present.

The server subscribes to live publication, captures the current durable
watermark, and replays every stored sequence through that watermark. Replay is
paged internally; `limit` controls page size (default `100`, maximum `500`), not
total replay length. The server then emits `session.events.synchronized` before
pure live delivery. Durable events committed during replay are buffered and
delivered once after the boundary.

Use the history endpoint when a client needs a finite page instead of an open stream:

```text
GET /sessions/{id}/events/history?after=<seq>&limit=<n>
```

The history response is `{ "data": [...], "has_more": <boolean> }`. `limit`
has the same default and maximum. Advance `after` to the last returned durable
cursor and request another page while `has_more` is true.

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
| `cursor` | Resume position. Present for durable events and nonzero stream control boundaries. |
| `data` | Event-specific payload. |

For live-only activity without `cursor`, the SSE `id` is the event ID.
Treat `type` as the payload discriminator. Ignore event types that the client
does not recognize; do not interpret an unknown payload as a known event.

## Stream Controls

Control events coordinate replay and recovery; do not render them as session
activity.

| Event | Meaning |
|---|---|
| `session.events.synchronized` | Every durable event through this cursor has been delivered; subsequent frames are live. |
| `session.events.resync_required` | Delivery overflowed or the cursor could not be reconciled. Reload authoritative state and reconnect. |

The server disconnects after `session.events.resync_required`. Keep the last
durable cursor, discard volatile partial rendering, reload the session and
tracked run, and reconnect. Never advance a saved cursor backward.

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
| `session.tool.updated` | A tool reached `proposed`, `authorized`, `started`, `completed`, `failed`, `interrupted`, or `declined`, including its latest durable input, text output, structured content, metadata, error, and timing. |
| `session.tool.completed` | A tool finished successfully. |
| `session.tool.failed` | A tool failed. |
| `session.permission.requested` | A tool is waiting for an interactive permission reply. |
| `session.permission.resolved` | A permission request was approved, rejected, timed out, or interrupted. |
| `session.message.created` | A message was appended to history. |
| `session.structured_output.completed` | Output schema parsing succeeded. |
| `session.run.completed` | The run finished successfully. |
| `session.run.failed` | The run failed. |
| `session.run.aborted` | The run was canceled or interrupted. |

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

A client only needs a session ID and last durable sequence. The open stream
performs complete durable replay, so clients do not need to page history before
subscribing:

```text
last_seq = load_checkpoint(session_id)
connect /sessions/{id}/events?after=last_seq

for each event:
  if event.cursor exists:
    last_seq = max(last_seq, event.cursor.seq)
    save_checkpoint(last_seq)
  if event.type == "session.events.synchronized":
    continue
  if event.type == "session.events.resync_required":
    clear volatile state
    reload session and run
    reconnect with last_seq
  apply event

on disconnect:
  reload session and run
  if the run is still queued or running:
    reconnect with bounded backoff from last_seq
```

Transport loss is not evidence that a run failed. The authoritative run resource
decides whether to reconnect or present a terminal result.

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

Persistent run failures and aborts are durable `session.run.failed` and
`session.run.aborted` events with JSON envelopes; they are not transport errors.
The persistent connection stays open after a terminal run event so it can carry
later queued runs for the session.

## One-Shot `/run` Stream

`POST /run` returns a separate, non-persistent SSE stream with its own event
vocabulary, including `stream_part`. Its JSON `data:` value is a public
lower-case envelope with `type`, `version`, and event-specific `data`. These
events are not `session.*` events and cannot be replayed. Treat `type` as a
discriminator and ignore unknown event types.

On success, `/run` sends a terminal `done` event containing usage and step
information, then closes. If setup or streaming fails, it sends a terminal
`error` event and closes. Its `data:` is a stream envelope whose inner `data`
uses the same `code`, `message`, and `request_id` fields as an HTTP API error.
The code is `run_failed`. An `error` event forwarded from the underlying run
uses the same shape. Treat EOF before either `done` or `error` as an interrupted
run, not as successful completion.

## `stream_part`

`stream_part` is only part of the `/run` contract. It carries provider stream parts such as text and tool-input deltas:

```text
event: stream_part
```

Persistent session SSE never emits `stream_part`; it translates supported provider parts into the live `session.text.delta`, `session.reasoning.delta`, and `session.tool.input.delta` events described above. Use persistent SSE for recoverable session state and `/run` only when the raw one-shot stream is required.
