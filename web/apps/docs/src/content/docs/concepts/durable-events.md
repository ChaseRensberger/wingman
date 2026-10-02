---
title: "Durable Events"
group: "Core"
order: 103
---

# Durable Events

Saved sessions retain messages, runs, tool calls, permission decisions, and events across restarts.

## Session Versions

Each persisted session has a `version`. Rename, move, and delete requests send the read version as `expected_version`.

If the session changed first, Wingman returns `409 Conflict`. Reload the session before you retry the change.

## Queued Runs

`POST /sessions/{id}/message` saves the run before returning `202 Accepted`.
The run keeps the message and configuration selected when the request was accepted.

Queued runs for one session run in order. Use `GET /sessions/{id}/runs` or `GET /sessions/{id}/runs/{runID}` to read the current status.

If the server restarts, queued runs resume. Wingman records an active run as `aborted`. Wingman does not replay provider calls or tool uses that ran.

## Session Events

`GET /sessions/{id}/events` replays durable session events after a cursor. It then continues with live events. Durable events include run status changes, completed message content, tool state, and permission decisions.

Partial text, reasoning, tool input, and progress are not replayed.
After reconnecting, use saved events and session or run records for current state.

See [Streaming Events](/build-clients/streaming-events) for the event contract and reconnect procedure.

## Deletion

Deleting a session permanently removes its history, runs, events, messages, model calls, tool uses, and permission records. Wingman does not retain a deleted-session record.
