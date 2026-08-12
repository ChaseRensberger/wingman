---
title: "Streaming Events"
description: "Consume Wingman's server-sent event stream."
---

# Streaming Events

Wingman has two SSE contracts: persistent session events and the one-shot
`POST /run` stream. Clients start persistent work with the message endpoint.
They read session state from the session event stream.

These examples use HTTP Basic authentication. Load managed-service credentials
with `source ~/.config/wingman/service.env`; the username defaults to `wingman`.
See [HTTP API Basics](/build-clients/http-api-basics#authentication).

## Start Work

Start a persistent session run:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "submit-123",
    "agent_id": "agt_...",
    "message": "Summarize this project."
  }'
```

If the client can retry an uncertain response, save `request_id` before you
send the request. An identical retry returns the existing run. It does not
create a second `session.run.queued` event. The accepted response includes
`run_id`, `status`, and `session_version`.

## Subscribe

Subscribe to session events:

```bash
curl -N "http://localhost:2323/sessions/${SESSION_ID}/events?after=0" \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Accept: text/event-stream"
```

Wingman sends SSE frames:

```text
id: <event-id-or-seq>
event: <type>
data: <json>

```

## Cursor

`after` is an exclusive session sequence. If a client last processed sequence
`42`, reconnect with:

```text
GET /sessions/{id}/events?after=42
```

Clients can instead send `Last-Event-ID: 42`. An explicit `after` query takes
precedence if both are present.

The server subscribes to live publication. It captures the current durable
watermark. It replays every stored sequence through that watermark. Replay uses
internal pages. `limit` controls the page size. The default is `100`. The
maximum is `500`. It does not control the total replay length. The server then
emits `session.events.synchronized` before pure live delivery. The server
buffers durable events committed during replay. It delivers them once after the
boundary.

If a client needs a finite page instead of an open stream, use the history
endpoint:

```text
GET /sessions/{id}/events/history?after=<seq>&limit=<n>
```

The history response is `{ "data": [...], "has_more": <boolean> }`. `limit`
has the same default and maximum. Advance `after` to the last returned durable
cursor. Then request another page while `has_more` is true.

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

For live-only activity without `cursor`, the SSE `id` is the event ID. Treat
`type` as the payload discriminator. Ignore event types that the client does
not recognize. Do not interpret an unknown payload as a known event.

## Stream Controls

Control events coordinate replay and recovery. Do not render them as session
activity.

| Event | Meaning |
|---|---|
| `session.events.synchronized` | Every durable event through this cursor was delivered. Subsequent frames are live. |
| `session.events.resync_required` | Delivery overflowed or the cursor did not reconcile. Reload authoritative state. Then reconnect. |

The server disconnects after `session.events.resync_required`. Keep the last
durable cursor. Discard volatile partial rendering. Reload the session and
tracked run. Then reconnect. Never advance a saved cursor backward.

## Durable Events

The server stores and replays durable events. These events reconstruct the
transcript and final run state after a reconnect.

| Event | Meaning |
|---|---|
| `session.run.queued` | A message run was durably queued. |
| `session.run.started` | A session run started. |
| `session.step.started` | A model/tool loop step started. |
| `session.step.completed` | A model/tool loop step completed. |
| `session.text.completed` | A text block reached its final value. |
| `session.reasoning.completed` | A reasoning block reached its final value. |
| `session.tool.called` | The model requested a tool. |
| `session.tool.updated` | A tool reached `proposed`, `authorized`, `started`, `completed`, `failed`, `interrupted`, or `declined`. It includes its latest durable input, text output, structured content, metadata, error, and timing. |
| `session.tool.completed` | A tool finished successfully. |
| `session.tool.failed` | A tool failed. |
| `session.permission.requested` | A tool is waiting for an interactive permission reply. |
| `session.permission.resolved` | A permission request was approved, rejected, timed out, or interrupted. |
| `session.message.created` | A message was appended to history. |
| `session.structured_output.completed` | Output schema parsing succeeded. |
| `session.run.completed` | The run finished successfully. |
| `session.run.failed` | The run failed. |
| `session.run.aborted` | The run was canceled or interrupted. |

Durable events store boundaries, not token streams. A completed text event
stores the final text for that block. It does not store every partial token
delta.

## Live Events

Live events are not replayed. They provide latency-sensitive rendering while
the client is connected.

| Event | Meaning |
|---|---|
| `session.text.delta` | Partial assistant text. |
| `session.reasoning.delta` | Partial reasoning text. |
| `session.tool.input.delta` | Partial tool-call input. |
| `session.tool.progress` | Incremental tool output and metadata reported during execution. |

Live provider events include the correlation identifiers for the later durable
boundary event. Text, reasoning, and tool-input deltas include `run_id`,
`step`, `message_id`, `part_id`, and the message `revision` when the server
saved the delta. Tool progress also includes `call_id`:

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
temporary correlation key. If `session.tool.called` or `session.tool.updated`
supplies `tool_use_id`, migrate the transient state to the durable ID. Provider
call IDs can repeat across runs. Do not use them as session-global identity.

Tool progress includes `tool_use_id`. Append `output_delta` to the visible
output. Shallow-merge `metadata`. Replace both with values from the later
durable `session.tool.updated` event. Treat that event's exact lifecycle status
as authoritative.

Treat `session.message.created` as the authoritative snapshot. If its
`revision` is newer or equal, replace an existing message with the same
`message.id`. Do not append a duplicate after replay. Ignore an older revision.
Apply the same identity rule to parts in the message. A `failed` message can
contain useful partial text, reasoning, or tool input.

## Recovery

A client needs only a session ID and last durable sequence. The open stream
performs complete durable replay. Clients do not need to page history before
they subscribe:

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

Transport loss does not mean that a run failed. The authoritative run resource
defines whether to reconnect or show a terminal result.

## Transport

Persistent session SSE responses set these headers:

```text
Content-Type: text/event-stream
Cache-Control: no-cache, no-transform
X-Accel-Buffering: no
X-Content-Type-Options: nosniff
```

`POST /run` sets `Content-Type: text/event-stream`, `Cache-Control: no-cache`,
and `Connection: keep-alive`.

Idle streams send heartbeat comments:

```text
: heartbeat

```

Persistent run failures and aborts are durable `session.run.failed` and
`session.run.aborted` events with JSON envelopes. They are not transport
errors. The persistent connection stays open after a terminal run event. It can
carry later queued runs for the session.

## One-Shot `/run` Stream

`POST /run` returns a separate, non-persistent SSE stream. It has its own event
vocabulary, including `stream_part`. Its JSON `data:` value is a public
lower-case envelope with `type`, `version`, and event-specific `data`. These
events are not `session.*` events and cannot be replayed. Treat `type` as a
discriminator. Ignore unknown event types.

On success, `/run` sends a terminal `done` event with usage and step
information. Then it closes. If setup or streaming fails, it sends a terminal
`error` event. Then it closes. Its `data:` is a stream envelope. Its inner
`data` uses the same `code`, `message`, and `request_id` fields as an HTTP API
error. The code is `run_failed`. An `error` event from the underlying run uses
the same shape. Treat EOF before either `done` or `error` as an interrupted
run, not as successful completion.

## `stream_part`

`stream_part` is only part of the `/run` contract. It carries provider stream
parts such as text and tool-input deltas:

```text
event: stream_part
```

Persistent session SSE never emits `stream_part`. It translates supported
provider parts into the live `session.text.delta`, `session.reasoning.delta`,
and `session.tool.input.delta` events described above. Use persistent SSE for
recoverable session state. Use `/run` only if the raw one-shot stream is
required.
