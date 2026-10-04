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

If the server restarts, Wingman resumes queued messages and recovers eligible active message runs under the same run IDs.
Recovery reuses committed results and preserves interrupted attempts in history.
An uncertain non-replayable tool outcome stops automatic recovery. See [Run Status And Recovery](/concepts/sessions#run-status-and-recovery) for limits and failure codes.

## Session Events

`GET /sessions/{id}/events` replays durable session events after a cursor. It then continues with live events. Durable events include run status changes, completed message content, tool state, and permission decisions.

Partial text, reasoning, tool input, and progress are not replayed.
After reconnecting, use saved events and session or run records for current state.
Recovery can emit another `session.run.queued` and `session.run.started` for an existing run ID. These events do not represent new submissions.

See [Streaming Events](/build-clients/streaming-events) for the event contract and reconnect procedure.

## Deletion

Deleting a session permanently removes its history, runs, events, messages, model calls, tool uses, and permission records. Wingman does not retain a deleted-session record.
