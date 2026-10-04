---
title: "Sessions"
group: "Core"
order: 102
---

# Sessions

A session stores a conversation and runs its messages. Each message can use a different agent or model.
Saved sessions survive server restarts. Wingman resumes unfinished work when it can do so safely.

A [Workspace](/concepts/workspaces) groups sessions and can supply their initial working directory.

## Create Then Send

These examples use `wingman api`, which connects to the local managed service.
Create a session before you send a message:

```bash
SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Explore repo\",\"working_directory\":\"$(pwd)\"}" | jq -r .id)
```

Create a session in a Workspace:

```bash
WORKSPACE_ID=$(wingman api listWorkspaces | jq -r '.[0].id')

SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Send either `working_directory` or `workspace_id`, not both. Wingman copies a Workspace path to the session's `work_dir`.

## Rename And Move

Persisted session responses include a `version`. Use this value to rename a session:

```bash
wingman api renameSession --param "id=${SESSION_ID}" \
  -d '{"title":"Investigate retries","expected_version":1}'
```

To move a session, send exactly one of `working_directory` or `workspace_id`:

```bash
wingman api moveSession --param "id=${SESSION_ID}" \
  -d '{"working_directory":"/home/me/other-project","expected_version":2}'
```

Changes increment `version`. An unchanged title or location leaves it unchanged.
If another client changes the session first, Wingman returns `409 Conflict`. Reload the session before you retry.

## Delete

Deleting a session permanently removes its history and associated records. Pass its current version:

```bash
wingman api deleteSession \
  --param "id=${SESSION_ID}" \
  --param expected_version=2
```

Wingman cancels active work and closes event streams before deletion returns success.
A stale version returns `409 Conflict` without deleting the session or canceling work.

## Admit Work

Send a message with an optional retry ID:

```bash
wingman api messageSession --param "id=${SESSION_ID}" \
  -d '{
    "request_id": "submit-123",
    "agent_id": "agt_...",
    "message": "Summarize this project"
  }'
```

`POST /sessions/{id}/message` requires an existing session. A typo in the ID returns `404`. It does not create a new session.

The endpoint queues the message and returns `202 Accepted` with `run_id`, `status`, and `session_version`.
Messages run in order within each session. Use the event stream to follow progress.

Retry the same input with the same `request_id` to return the existing run.
Changed input with that ID returns `409 Conflict`. Without an ID, each request creates a new run.
See the [message reference](/reference/referenceapi#message-request) for the fields that must match.

Queued work uses the agent, model, location, [project instructions](/configure/project-instructions), and [skills](/configure/skills) saved when the request was accepted.
Later edits and session moves affect only new requests.

## Per-Message Agent and Model

Each message selects its agent and model:

```json
{
  "agent_id": "agt_...",
  "model_ref": "anthropic/claude-sonnet-5",
  "message": "Use the stronger model for this turn."
}
```

`model_ref` overrides the agent default model for that request. If neither the request nor the agent provides a model, the run fails before its first provider call.

## Streaming

If a client needs live events, use the event stream:

```bash
wingman api streamSessionEvents \
  --param "id=${SESSION_ID}" \
  --param after=0
```

The response is server-sent events. Each `data:` payload is a Wingman event envelope with `id`, `type`, and `data`. Durable events also include `cursor`.

Each accepted message emits `session.run.queued`. It then emits `session.run.started` when execution starts. Terminal events are `session.run.completed`, `session.run.failed`, and `session.run.aborted`. Each terminal event carries the accepted `run_id`. `POST /sessions/{id}/abort` cancels only the active run. It keeps later queued messages.

## Run Status And Recovery

After a reload or disconnect, read the saved run status:

```bash
wingman api listSessionRuns --param "id=${SESSION_ID}"
wingman api getSessionRun --param "id=${SESSION_ID}" --param runID=run_...
```

A run moves from `queued` to `running`. It then moves to `completed`, `failed`,
or `aborted`. A queued run can also be aborted before it starts.

Abort a specific queued or running run with:

```bash
wingman api abortSessionRun --param "id=${SESSION_ID}" --param runID=run_...
```

After a restart, Wingman resumes queued messages and recovers interrupted message runs under their original run IDs.
Recovery returns a run to `queued`, then `running`. It does not submit the input again.
Wingman reuses saved model responses and tool results. It retries eligible interrupted model requests, which can incur another provider charge.
Partial responses remain in history as `failed`. Wingman excludes these incomplete responses from the recovered model request.
If a partial response records a tool that the provider executed, its uncertain outcome blocks recovery with `recovery_blocked`.

A replay-safe tool permits repeated execution with the same input. Wingman retries an interrupted tool only when its saved and current definitions permit replay.
Calls that did not start can proceed through their remaining authorization and execution steps. Saved authorizations remain valid for the same call and input.
If a non-replayable tool started without a saved outcome, the run fails with `recovery_blocked`.
Inspect the tool record and its external effects before you submit another request.

Wingman permits three automatic recovery attempts per run. Further interruptions abort the run with `recovery_exhausted`.
Explicit cancellation remains terminal across restarts. Interrupted plugin actions remain `aborted` because they do not have a recovery contract.
Ephemeral runs do not recover. See [Tools](/concepts/tools#durable-execution-lifecycle) for replay rules.

## Ephemeral Sessions

An ephemeral run executes without saving a conversation:

```bash
wingman api runAgent -d '{
    "agent": {
      "name": "One-shot Assistant",
      "instructions": "Be concise.",
      "tools": ["webfetch"],
      "model_ref": "anthropic/claude-sonnet-5"
    },
    "message": "Explain Wingman in one paragraph."
  }'
```

If the server starts with `--ephemeral`, persisted endpoints such as `/sessions`, `/agents`, `/clients`, `/workspaces`, and `/provider/auth` return `501 Not Implemented`. In this mode, use inline agent specs with `/run`.

## Working Directory

A working directory is required for file and shell tools. Web tools can run without one.
See [Tools](/concepts/tools) for requirements and access limits.

Changing a Workspace path does not change existing sessions. Create or move a session to use the new path.

## Message Parts

A message contains content blocks called parts:

- Text.
- Image.
- Reasoning.
- Tool calls and results, stored on the assistant message that requested them.
- Structured output.
- Plugin-defined opaque content.

Messages have a stable `id`, increasing `revision`, and a `state` of `in_progress`, `completed`, or `failed`.
Partial content remains available after a stream failure.
See [Streaming Events](/build-clients/streaming-events) for updating messages in a client.

Tool metadata includes file changes and patches that clients can display without parsing text.
For execution status, read `/sessions/{id}/tool-uses` rather than the displayed tool part.

## Usage and Context

Each provider attempt has a model-call record with status, timing, and token usage.
Use the latest record for usage and context-window fullness. Do not estimate these values from transcript text.
See [Observability](/use-wingman/observability) for inspecting calls and failures.
