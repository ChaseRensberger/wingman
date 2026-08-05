---
title: "Durable Events"
group: "Core"
order: 103
---

# Durable Events

Persisted sessions keep durable records for session changes, queued runs,
messages, tool use, permission requests, and session events. Clients can reload
that state after a disconnect or restart.

## Session Versions

Each persisted session has a `version`. Rename, move, and delete requests send
the version they read as `expected_version`.

If the session changed first, Wingman returns `409 Conflict`. Reload the session
before deciding whether to retry the change.

## Queued Runs

`POST /sessions/{id}/message` creates a durable run before returning `202
Accepted`. The run stores the message, selected Agent and model, client, and
working directory for that execution.

Queued runs for one session execute in order. Use `GET /sessions/{id}/runs` or
`GET /sessions/{id}/runs/{runID}` to read the current status.

If the server restarts, queued runs resume. A run that was already executing is
recorded as `aborted`. Provider calls and tool uses that may have already run are
not replayed.

## Session Events

`GET /sessions/{id}/events` replays durable session events after a cursor, then
continues with live events. Durable events include run status changes, completed
message content, tool state, and permission decisions.

Live text, reasoning, tool-input, and tool-progress updates are not replayed.
After reconnecting, use the replayed events and session or run resources as the
authoritative state.

See [Streaming Events](/build-clients/streaming-events) for the event contract
and reconnect procedure.

## Deletion

Deleting a session permanently removes its history, runs, events, messages,
model calls, tool uses, and permission records. Wingman does not retain a
deleted-session record.
