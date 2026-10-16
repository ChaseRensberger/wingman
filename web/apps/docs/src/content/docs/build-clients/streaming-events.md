---
title: "Streaming Events"
description: "Read live updates and recover after a disconnect."
---

# Streaming Events

Server-sent events (SSE) send updates over an open HTTP connection.
Saved session streams support replay. The one-shot `/run` stream does not.
The [Go](/build-clients/go-sdk) and [TypeScript](/build-clients/typescript-sdk) SDKs read both formats.

## Subscribe to a session

Complete the [HTTP authentication setup](/concepts/authentication#direct-http-requests).
Set `SESSION_ID` to an existing session ID, then connect:

```bash
curl -N "$WINGMAN_URL/sessions/${SESSION_ID}/events?after=0" \
  -u "$WINGMAN_AUTH" \
  -H "Accept: text/event-stream"
```

Each SSE frame has `id`, `event`, and `data` fields. `data` contains the Wingman event:

```json
{
  "id": "evt_example",
  "schema_version": 1,
  "type": "session.message.created",
  "cursor": { "session_id": "ses_example", "seq": 43 },
  "data": {
    "run_id": "run_example",
    "message": {
      "id": "msg_example",
      "revision": 8,
      "state": "completed",
      "role": "assistant",
      "content": []
    }
  }
}
```

Use `type` to read the payload. Ignore unknown event types.
`schema_version` is currently `1`. The API schema describes each payload.

## Replay and cursors

A cursor marks a position in the saved event sequence.
After processing sequence `42`, reconnect with `?after=42` or `Last-Event-ID: 42`.
The `after` query takes precedence when both are present.

Wingman replays saved events, emits `session.events.synchronized`, then sends live updates.
The connection stays open after a run ends and can carry later runs.
Idle connections send heartbeat comments every 15 seconds.

`limit` controls replay page size, not total replay length. Its default is `100` and maximum is `500`.
For a finite page, use `/sessions/{id}/events/history?after=<seq>&limit=<n>`.
Read `data`, advance to its last cursor, and continue while `has_more` is true.

## Saved and live updates

Saved events have a `cursor`. They include run status, message snapshots, tool state, and permission decisions.
The SSE `id` is the sequence number for these events.

Live text, reasoning, tool-input deltas, and tool progress do not have a cursor and are not replayed.
Their SSE `id` is the event ID. Do not use that ID as a saved resume position.
`session.tool.updated` can also arrive as a live `pending` update before a tool call is complete.

| Event                                                                | Client action                                                                            |
| -------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| `session.message.created`                                            | Replace the message when its revision is equal or newer. Match messages and parts by ID. |
| `session.text.delta`, `session.reasoning.delta`                      | Append live text to the identified message part.                                         |
| `session.tool.input.delta`                                           | Track input by run and call ID until `tool_use_id` is available.                         |
| `session.tool.progress`                                              | Append `output_delta` and merge top-level `metadata` fields.                             |
| Saved `session.tool.updated`                                         | Replace live tool output and metadata with the saved values.                             |
| `session.permission.requested`, `session.permission.resolved`        | Update approval state from the complete request.                                         |
| `session.run.completed`, `session.run.failed`, `session.run.aborted` | Update run status. These are not connection errors.                                      |

A failed message can contain partial content. Provider `call_id` values can repeat, so use Wingman `tool_use_id` when available.
See [Tools](/concepts/tools#durable-execution-lifecycle) for execution states.

## Reconnect safely

Apply an event before saving its cursor. Save state and cursor together when your storage supports it.
Otherwise, a crash can skip an event that your application did not apply.

On disconnect, reload the session and run, then reconnect from the last processed cursor.
Receive missing events even if the run completed while disconnected.
Use a limited wait between connection attempts.

`session.events.resync_required` means delivery overflowed or the cursor cannot match saved history. The server then disconnects.
Discard live partial output, reload saved state, and reconnect with your last processed cursor.
Do not advance the cursor from this control event.

If your cursor exceeds the server history, reconnecting with it fails again.
After a database restore or server change, rebuild local state from `after=0` and save a new cursor for that server.

## One-shot `/run` stream

`POST /run` uses a different envelope: `type`, `version`, and `data`.
Its `stream_part` events contain provider output parts. It has no replay or heartbeat comments.

Invalid requests and setup failures before streaming return HTTP errors with JSON bodies.
After streaming starts, a failure sends a final `error` event and closes the stream.
Its inner `data` contains `code: "run_failed"`, `message`, and `request_id`.

Success sends a final `done` event with usage and step information.
Treat a stream that ends before `done` or `error` as interrupted.
Closing this request cancels the one-shot run. Closing a saved session stream does not cancel its runs.
