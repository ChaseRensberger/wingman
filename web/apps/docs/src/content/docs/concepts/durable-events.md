---
title: "Durable Events and Projections"
group: "Core"
order: 103
---

# Durable Events and Projections

Wingman's durable store records session creation, renames, moves, and run
admissions in an append-only aggregate event log and maintains their critical
projections in the same transaction.

## Events and Projections

A durable event records a fact that already happened. `session.created` records
the initial state, `session.renamed` records a title change, `session.moved`
records a working-directory or Workspace change, and `session.run.admitted`
records the immutable input and execution snapshot for queued work.

A projection is a query-friendly table derived from those facts. The `sessions`
table is the session metadata projection used by the HTTP API, and
`session_runs` holds admitted work and its execution status.

For each metadata transition, Wingman commits both in one SQLite transaction:

```text
append session.created, session.renamed, or session.moved
update sessions projection
commit
```

If either write fails, both are rolled back. A metadata transition cannot leave
an event and projection at different versions.

Admission atomically appends `session.run.admitted`, inserts the run projection,
advances the session version, and inserts the public `session.run.queued` event.
Execution begins only after that transaction commits.

## Aggregate Streams

Events are ordered within an aggregate. A session aggregate is identified by
its session ID:

```text
aggregate type: session
aggregate ID:   ses_...
version:        1
event type:     session.created
```

Versions are contiguous within one aggregate. Session creation expects version
zero and commits version one. Rename and move require the caller's expected
version and increment it when they change state. Each new run admission also
advances the version. A duplicate creation or stale metadata command fails
instead of overwriting state.

The event also has a global insertion sequence for storage diagnostics and a
schema version for decoding its payload. Aggregate version and payload schema
version solve different problems.

## Replay

The session projector rebuilds the current `sessions` row and version from its
ordered creation, rename, move, and run-admission events. The run projector
decodes the immutable run snapshot from each `session.run.admitted` event.
Projectors reject:

- Missing or out-of-order event versions.
- Duplicate creation events.
- Unknown event types.
- Unsupported payload schema versions.
- Payload IDs that do not match the aggregate.

This makes projection behavior deterministic and testable independently of the
HTTP server.

## Session History

The aggregate event log records session creation, title changes, location
changes, and run admissions. An admission captures its request identity, prompt,
effective Agent and model, output schema, client, and placement snapshot.
Messages, model calls, and tool uses are persisted in their respective state
tables and are not yet represented in aggregate history.

## Hard Purge

Deletion is not an aggregate event. `DELETE /sessions/{id}` requires the
session's current version and atomically removes both the projection and the
entire aggregate stream. The same transaction removes all session-owned runs,
public events, messages, parts, model calls, and tool uses through foreign-key
cascades.

Wingman retains no deletion event or tombstone. After the transaction commits,
the server closes live event streams, cancels active execution, and waits for
the session worker to settle before returning success.

## Aggregate Events vs. Streaming Events

Aggregate events are internal persistence facts. The session SSE API is a
public client delivery contract.

They have different jobs:

| Mechanism | Purpose |
|---|---|
| Aggregate event log | Rebuild session metadata and admitted runs from session aggregate facts. |
| Session event history | Let clients replay public session activity. |
| Live SSE events | Render low-latency deltas that may not be durable. |

Clients should continue using the documented [Streaming Events](/build-clients/streaming-events) API. They do not read the internal aggregate log directly.

## Store Implementations

SQLite and the in-memory store apply the same session projector. Both also
implement the optional `store.AggregateEventReader` diagnostic and replay
interface. SQLite provides the durable transaction; the memory store applies
the event and projection under one lock.
