---
title: "Sessions"
group: "Core"
order: 102
---

# Sessions

A session is the runtime record for agent work. It owns message history, drives model turns, dispatches tool calls, emits events, and persists the transcript when storage is enabled.

A session stores runtime state, while an agent provides reusable configuration:

- An agent is a reusable definition.
- A session is a running conversation or one-shot run.
- A session is not permanently bound to one agent.
- A session is not permanently bound to one model.
- Each message chooses the agent configuration for that turn.

One session can hand off between agents or models without creating a new conversation record.

Creating, renaming, moving, or admitting work to a persisted session atomically
appends a durable aggregate event and updates its critical projections. See
[Durable Events and Projections](/concepts/durable-events) for the persistence
model and its current scope.

Sessions can belong to a [Workspace](/concepts/workspaces). A Workspace is a saved context that groups sessions and can optionally seed their working directory.

## Create Then Send

Wingman's session API follows the same split as OpenCode: create a session first, then send messages to that session.

Create a session:

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"working_directory\":\"$(pwd)\"}" | jq -r .id)
```

Create a session in a Workspace:

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces | jq -r '.[0].id')

SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

`working_directory` and `workspace_id` are mutually exclusive. When `workspace_id` is used, Wingman keeps `workspace_id` for grouping and copies the Workspace path into the session's `work_dir` when it has one.

## Rename And Move

Persisted session responses include a `version`. Use it to rename a session:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/rename" \
  -H "Content-Type: application/json" \
  -d '{"title":"Investigate retries","expected_version":1}'
```

Move a session by sending exactly one of `working_directory` or `workspace_id`:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/move" \
  -H "Content-Type: application/json" \
  -d '{"working_directory":"/home/me/other-project","expected_version":2}'
```

Each changed result increments `version`. If another client changed the session
first, Wingman returns `409 Conflict`; reload before deciding whether to retry.
Sending the current title or location is a no-op and does not increment the
version.

## Delete

Deletion is a permanent hard purge. Pass the version you read as a query
parameter:

```bash
curl -sS -X DELETE \
  "http://localhost:2323/sessions/${SESSION_ID}?expected_version=2"
```

Wingman atomically removes the session, aggregate history, public event
history, queued and completed runs, messages, parts, and model-call records. It
retains no deletion event or tombstone. Active event streams close, active
execution is canceled, and the worker settles before the endpoint returns
success. A stale version returns `409 Conflict` without deleting the session or
canceling its work.

## Admit Work

Send a message with an optional retry identity:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -H "Content-Type: application/json" \
  -d '{
    "request_id": "submit-123",
    "agent_id": "agt_...",
    "message": "Summarize this project"
  }'
```

`POST /sessions/{id}/message` requires the session to exist. A typo in the ID returns `404`; it does not create a new session.

The endpoint returns `202 Accepted` with `run_id`, the current run `status`, and
`session_version` as soon as the message is durably queued. A daemon-owned
worker executes queued messages serially for each session, so clients observe
progress and completion through the event stream instead of waiting for this
request.

`request_id` is optional and scoped to one session. Retrying the same effective
input with the same ID returns the existing run without publishing another
queued event. Reusing the ID after changing the prompt, effective Agent or
model, output schema, client, or session placement returns `409 Conflict`.
Omitting it always admits a new run.

Admission snapshots the effective Agent, output schema, working directory,
Workspace, and client. Moving the session or editing the Agent afterward affects
future admissions only; already queued work executes from its snapshot.

## Per-Message Agent and Model

Agents and models are selected per message:

```json
{
  "agent_id": "agt_...",
  "model_ref": "anthropic/claude-sonnet-5",
  "message": "Use the stronger model for this turn."
}
```

`model_ref` overrides the agent's default model for that request. If neither the request nor the agent provides a model, the run fails before the first provider call.

## Streaming

Use the event stream when a client needs live events:

```bash
curl -N "http://localhost:2323/sessions/${SESSION_ID}/events?after=0" \
  -H "Accept: text/event-stream"
```

The response is server-sent events. Each `data:` payload is a Wingman event envelope containing `id`, `type`, and `data`. Durable events also include `cursor`.

The public SSE history is separate from the internal aggregate event log used
to rebuild database projections.

Each accepted message emits `session.run.queued`, then `session.run.started` when execution begins. Terminal events are `session.run.completed` and `session.run.failed`; all carry the accepted `run_id`. `POST /sessions/{id}/abort` cancels only the active run and leaves later queued messages intact.

## Ephemeral Sessions

Some agent runs should not leave durable state. Wingman exposes that as an ephemeral run:

```bash
curl -N -X POST http://localhost:2323/run \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent": {
      "name": "One-shot Assistant",
      "instructions": "Be concise.",
      "tools": ["webfetch"],
      "model_ref": "anthropic/claude-sonnet-5"
    },
    "message": "Explain Wingman in one paragraph."
  }'
```

An ephemeral run has a runtime, tools, model calls, and events. It is not written to the store.

When the server is started with `--ephemeral`, persisted endpoints such as `/sessions`, `/agents`, `/clients`, `/workspaces`, and `/provider/auth` return `501 Not Implemented`. Use inline agent specs with `/run` in that mode.

## Working Directory

A session can have a working directory. Directory-scoped tools such as `read`, `glob`, `grep`, `write`, `edit`, `apply_patch`, and `bash` use that directory as their workspace.

Sessions without a working directory are valid if the selected agent only uses tools that do not need one, such as `webfetch` or `websearch`.

A session created or moved with `workspace_id` stores a snapshot of that Workspace's path as `work_dir`. Changing the Workspace later affects future sessions and moves, not an existing session's `work_dir` value.

## Message Parts

Session history is stored as messages with typed parts. A part is Wingman's provider-neutral content block:

- Text.
- Image.
- Reasoning.
- Tool invocation. A tool part belongs to the assistant message that requested
  it and records pending, running, completed, or error state. The provider
  receives a derived tool result when the session continues; separate
  tool-role messages are not stored in session history.
- Structured output.
- Plugin-defined opaque content.

Tool result parts contain model-facing text and may also contain metadata for clients. File-editing tools use this metadata to expose changed files, patches, and addition/deletion counts so UIs can render diffs without parsing prose. Parts let Wingman preserve provider-specific richness without storing provider-native wire formats. UIs can render each block differently, and plugins can introduce custom content.

Persisted messages have a stable `id`, monotonic `revision`, and a `state` of
`in_progress`, `completed`, or `failed`. Every built-in part also has a stable
`id`. Wingman stores each complete message revision and its parts atomically, so
a reload cannot observe a message row from one revision with parts from
another. Text, reasoning, and raw tool input are checkpointed while the
provider stream is active. If streaming fails, the identified partial assistant
message remains in history with `state: "failed"`.

Pending tool parts are checkpointed before Wingman starts local tool execution.
This preserves truthful input and state for inspection after interruption; it
does not provide exactly-once execution of external side effects.

## Usage and Context

Persisted sessions store one normalized model-call record per physical upstream
attempt. Each record has a stable call ID and carries its `run_id`, loop step,
attempt number, provider/model route, lifecycle state, timing, token usage,
context fullness, and provider request ID when available. Wingman persists the
started record before dispatch and settles it when the provider stream ends.

A model call completes before requested tools execute. A later tool failure can
fail the run without changing the successful upstream attempt to failed. Steps
restart at one for each run; run-scoped identity keeps every turn's history.

Clients should use the latest model call, not transcript text estimation, when showing session usage or context-window fullness after reload.
