---
title: "Durable Events and Projections"
group: "Core"
order: 103
---

# Durable Events and Projections

Wingman's durable store records session creation in an append-only aggregate
event log and maintains the session metadata projection in the same transaction.

## Events and Projections

A durable event records a fact that already happened. For example,
`session.created` records the initial state of one session.

A projection is a query-friendly table derived from those facts. The `sessions`
table is the session metadata projection used by the HTTP API.

For session creation, Wingman commits both in one SQLite transaction:

```text
append session.created
update sessions projection
commit
```

If either write fails, both are rolled back. A session creation cannot leave an
event without a projected session or a projected session without its creation
event.

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
zero and commits version one. A duplicate creation sees the existing version
and fails instead of overwriting state.

The event also has a global insertion sequence for storage diagnostics and a
schema version for decoding its payload. Aggregate version and payload schema
version solve different problems.

## Replay

The session projector can rebuild the initial `sessions` row from the ordered
creation event. Projectors reject:

- Missing or out-of-order event versions.
- Duplicate creation events.
- Unknown event types.
- Unsupported payload schema versions.
- Payload IDs that do not match the aggregate.

This makes projection behavior deterministic and testable independently of the
HTTP server.

## Session Creation History

The aggregate event log records session creation. Session metadata updates,
deletion, queued runs, messages, model calls, and tool calls are persisted in
their respective state tables and are not represented in aggregate history.

## Existing Databases

Migration `0006_aggregate_events.sql` creates the event log, adds the session
projection version, and backfills one `session.created` event for every existing
session. Existing session IDs, metadata, timestamps, Workspace relationships,
and client attribution are preserved.

## Aggregate Events vs. Streaming Events

Aggregate events are internal persistence facts. The session SSE API is a
public client delivery contract.

They have different jobs:

| Mechanism | Purpose |
|---|---|
| Aggregate event log | Rebuild the initial session projection from `session.created`. |
| Session event history | Let clients replay public session activity. |
| Live SSE events | Render low-latency deltas that may not be durable. |

Clients should continue using the documented [Streaming Events](/build-clients/streaming-events) API. They do not read the internal aggregate log directly.

## Store Implementations

SQLite and the in-memory store apply the same session projector. Both also
implement the optional `store.AggregateEventReader` diagnostic and replay
interface. SQLite provides the durable transaction; the memory store applies
the event and projection under one lock.
