---
title: "Streaming Events"
description: "Consume Wingman's server-sent event stream."
---

# Streaming Events

Wingman uses server-sent events (SSE) to stream updates over HTTP.
Session events support replay. The one-shot `POST /run` stream does not.

These examples use the direct HTTP setup in
[Authentication](/concepts/authentication#direct-http-requests).

## Start Work

Start a persistent session run:

```bash
curl -sS -X POST "$WINGMAN_URL/sessions/${SESSION_ID}/message" \
  -u "$WINGMAN_AUTH" \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "submit-123",
    "agent_id": "agt_...",
    "message": "Summarize this project."
  }'
```

Save `request_id` before sending. If the response is lost, retry the same input with that ID.
Wingman returns the existing run without queuing another. The response includes `run_id`, `status`, and `session_version`.

## Subscribe

Subscribe to session events:

```bash
curl -N "$WINGMAN_URL/sessions/${SESSION_ID}/events?after=0" \
  -u "$WINGMAN_AUTH" \
  -H "Accept: text/event-stream"
```

Wingman sends SSE frames:

```text
id: <event-id-or-seq>
event: <type>
data: <json>

```

## Cursor

The cursor is the last saved event sequence. `after` requests events after that sequence.
If the client last processed sequence `42`, reconnect with:

```text
GET /sessions/{id}/events?after=42
```

Clients can instead send `Last-Event-ID: 42`. An explicit `after` query takes
precedence when both are present.

Wingman replays saved events, emits `session.events.synchronized`, then delivers live events.
Events saved during replay arrive after that boundary.
`limit` controls replay page size, not total replay length. The default is `100` and the maximum is `500`.

For a finite page instead of an open stream, use the history endpoint:

```text
GET /sessions/{id}/events/history?after=<seq>&limit=<n>
```

The history response is `{ "data": [...], "has_more": <boolean> }`. `limit`
has the same default and maximum. Advance `after` to the last durable cursor.
Then request another page while `has_more` is true.

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

| Field    | Meaning                                                                            |
| -------- | ---------------------------------------------------------------------------------- |
| `id`     | Unique event ID.                                                                   |
| `type`   | Event type. Also used as the SSE event name.                                       |
| `time`   | Event timestamp.                                                                   |
| `cursor` | Resume position. Present for durable events and nonzero stream control boundaries. |
| `data`   | Event-specific payload.                                                            |

Live events without `cursor` use the event ID as their SSE `id`.
Use `type` to choose how to read the data. Ignore unknown event types.

## Stream Controls

Control events coordinate replay and recovery. Do not render them as session activity.

| Event                            | Meaning                                                                                     |
| -------------------------------- | ------------------------------------------------------------------------------------------- |
| `session.events.synchronized`    | Every durable event through this cursor was delivered. Subsequent frames are live.          |
| `session.events.resync_required` | Delivery overflowed or the cursor could not be matched. Reload the session and run, then reconnect. |

After `session.events.resync_required`, the server disconnects.
Keep the last saved cursor, discard live partial output, and reload the session and run before reconnecting.
Never move a saved cursor backward.

## Durable Events

The server stores and replays durable events. These events reconstruct the
transcript and final run state after a reconnect.

| Event                                 | Meaning                                                                                                  |
| ------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| `session.run.queued`                  | A message run was durably queued.                                                                        |
| `session.run.started`                 | A session run started.                                                                                   |
| `session.step.started`                | A model/tool loop step started.                                                                          |
| `session.step.completed`              | A model/tool loop step completed.                                                                        |
| `session.text.completed`              | A text block reached its final value.                                                                    |
| `session.reasoning.completed`         | A reasoning block reached its final value.                                                               |
| `session.tool.called`                 | The model requested a tool.                                                                              |
| `session.tool.updated`                | A tool reached `proposed`, `authorized`, `started`, `completed`, `failed`, `interrupted`, or `declined`. |
| `session.tool.completed`              | A tool finished successfully.                                                                            |
| `session.tool.failed`                 | A tool failed.                                                                                           |
| `session.permission.requested`        | A tool is waiting for an interactive permission reply.                                                   |
| `session.permission.resolved`         | A permission request was approved, rejected, timed out, or interrupted.                                  |
| `session.message.created`             | A message was appended to history.                                                                       |
| `session.structured_output.completed` | Output schema parsing succeeded.                                                                         |
| `session.run.completed`               | The run finished successfully.                                                                           |
| `session.run.failed`                  | The run failed.                                                                                          |
| `session.run.aborted`                 | The run was canceled or interrupted.                                                                     |

Saved events contain completed content, not each partial token update.

`session.tool.updated` includes the latest durable input, text output,
structured content, metadata, error, and timing.

## Live Events

Live events provide partial output while connected. They are not replayed.

| Event                      | Meaning                                                         |
| -------------------------- | --------------------------------------------------------------- |
| `session.text.delta`       | Partial assistant text.                                         |
| `session.reasoning.delta`  | Partial reasoning text.                                         |
| `session.tool.input.delta` | Partial tool-call input.                                        |
| `session.tool.progress`    | Incremental tool output and metadata reported during execution. |

Text, reasoning, and tool-input updates include `run_id`, `step`, `message_id`, `part_id`, and a `revision` when saved.
Tool progress also includes `call_id`:

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

Track partial tool input with `run_id` and `call_id` until an event supplies `tool_use_id`. Then use `tool_use_id`.
Provider call IDs can repeat across runs.

Tool progress includes `tool_use_id`. Append `output_delta` and merge top-level `metadata` fields.
Replace both with the later saved `session.tool.updated` values. Use that event's status for execution state.

For `session.message.created`, replace the matching message if its `revision` is equal or newer.
Ignore older revisions. Match parts by ID too, so replay does not create duplicates.
A `failed` message can still contain partial text, reasoning, or tool input.

## Recovery

Reconnect with the session ID and last saved sequence. The stream replays history without a separate history request:

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

A disconnect does not mean that the run failed. Read the run status before deciding whether to reconnect.

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
`session.run.aborted` events with JSON envelopes. They are not transport errors.
The persistent connection stays open after a terminal run event. It can carry
later queued runs for the session.

## One-Shot `/run` Stream

`POST /run` events contain `type`, `version`, and event-specific `data`.
They use different types from session events, including `stream_part`, and cannot be replayed. Ignore unknown types.

On success, `/run` sends a terminal `done` event with usage and step
information. Then it closes. If setup or streaming fails, it sends a terminal
`error` event. Then it closes. Its `data:` is a stream envelope. Its inner
`data` uses the HTTP API error fields `code`, `message`, and `request_id`. The
code is `run_failed`. An underlying run `error` event uses the same shape. Treat
EOF before `done` or `error` as an interrupted run, not successful completion.

## `stream_part`

`stream_part` is only in the `/run` contract. It carries provider stream parts,
such as text and tool-input deltas:

```text
event: stream_part
```

Session streams use `session.text.delta`, `session.reasoning.delta`, and `session.tool.input.delta` instead of `stream_part`.
