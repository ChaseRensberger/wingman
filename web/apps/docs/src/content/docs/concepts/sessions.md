---
title: "Sessions"
group: "Core"
order: 102
---

# Sessions

A session is the runtime record for agent work. It stores message history. It runs model turns. It dispatches tool calls. It emits events. It persists the transcript when storage is enabled.

A session stores runtime state. An agent provides reusable configuration:

- An agent is a reusable definition.
- A session is a running conversation or one-shot run.
- A session is not permanently bound to one agent.
- A session is not permanently bound to one model.
- Each message chooses the agent configuration for that turn.

One session can move between agents or models without a new conversation record.

Changes to a persisted session and newly admitted work are durable.

Sessions can belong to a [Workspace](/concepts/workspaces). A Workspace is a saved context that groups sessions. It can set the initial working directory.

## Create Then Send

Create a session first, then send messages to it.

The commands find and authenticate with the managed daemon.

Create a session:

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

`working_directory` and `workspace_id` are mutually exclusive. When you use `workspace_id`, Wingman stores it for grouping. If the Workspace has a path, Wingman copies it to the session `work_dir`.

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

Each changed result increments `version`. If another client changes the session first, Wingman returns `409 Conflict`. Reload the session before you retry. Sending the current title or location is a no-op. It does not increment the version.

## Delete

Deletion permanently purges the session. Pass the version that you read as a query parameter:

```bash
wingman api deleteSession \
  --param "id=${SESSION_ID}" \
  --param expected_version=2
```

Wingman permanently removes the session, event history, runs, messages, parts, model calls, tool uses, and permission records. Active event streams close before the endpoint returns success. Wingman cancels active execution before the endpoint returns success. A stale version returns `409 Conflict`. It does not delete the session or cancel its work.

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

The endpoint returns `202 Accepted` when it durably queues the message. The response includes `run_id`, the current run `status`, and `session_version`. A daemon-owned worker runs queued messages serially for each session. Clients observe progress and completion through the event stream. They do not wait for this request.

`request_id` is optional and applies to one session. Retrying the same effective input with the same ID returns the existing run. It does not publish another queued event. Reusing the ID after you change the prompt, effective Agent, model, output schema, client, or session placement returns `409 Conflict`. If you omit it, Wingman always admits a new run.

Admission stores a snapshot of the effective Agent, output schema, working directory, Workspace, and client. Moving the session or editing the Agent later affects future admissions only. Queued work runs from its snapshot.

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

The durable run record is authoritative after a reload or lost event stream:

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

Wingman does not automatically replay work that can have reached a provider or tool. During shutdown or restart, a running run becomes `aborted`. Started model calls become `aborted`. Unfinished tool uses become `interrupted`. An active message becomes `failed` and retains checkpointed content. Only runs that never started remain queued and resume automatically. If a recovery write fails, the server does not serve requests.

## Ephemeral Sessions

Some agent runs do not need durable state. Wingman provides them as ephemeral runs:

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

An ephemeral run has a runtime, tools, model calls, and events. Wingman does not write it to the store.

If the server starts with `--ephemeral`, persisted endpoints such as `/sessions`, `/agents`, `/clients`, `/workspaces`, and `/provider/auth` return `501 Not Implemented`. In this mode, use inline agent specs with `/run`.

## Working Directory

A session can have a working directory. Directory-scoped tools such as `read`, `glob`, `grep`, `write`, `edit`, `apply_patch`, and `bash` use that directory as the workspace.

If the selected agent uses only tools without a working directory requirement, sessions without one are valid. These tools include `webfetch` and `websearch`.

A session created or moved with `workspace_id` stores the Workspace path as a `work_dir` snapshot. Changing the Workspace later affects future sessions and moves. It does not affect an existing session `work_dir` value.

## Message Parts

Session history uses messages with typed parts. A part is a provider-neutral Wingman content block:

- Text.
- Image.
- Reasoning.
- Tool invocation. A tool part belongs to the assistant message that requested
  it. It records pending, running, completed, or error state. When the session continues, the provider receives a derived tool result. Session history does not store separate tool-role messages.
- Structured output.
- Plugin-defined opaque content.

Tool result parts contain model-facing text. They can also contain metadata for clients. File-editing tools use this metadata for changed files, patches, and addition/deletion counts. UIs can show diffs without parsing prose. Parts let Wingman preserve provider-specific content without provider-native wire formats. UIs can show each block differently. Plugins can add custom content.

Persisted messages have a stable `id`, monotonic `revision`, and a `state` of `in_progress`, `completed`, or `failed`. Every built-in part also has a stable `id`. Wingman stores each complete message revision and its parts atomically. A reload cannot observe a message row from one revision with parts from another. Wingman checkpoints text, reasoning, and raw tool input during the provider stream. If streaming fails, the identified partial assistant message remains in history with `state: "failed"`.

Wingman checkpoints pending tool parts before local tool execution starts. This preserves truthful input and state for inspection after interruption. It does not provide exactly-once execution of external side effects.

Each persisted tool part also has a stable `tool_use_id`. The tool-use record commits `started` before execution. It stores the exact durable lifecycle, rewritten input, output, metadata, errors, and timing. Unfinished records become `interrupted` on server startup. Wingman does not replay them automatically. If a client needs execution authority, use `/sessions/{id}/tool-uses`. The tool part remains the transcript presentation.

## Usage and Context

Persisted sessions store one normalized model-call record for each physical upstream attempt. Each record has a stable call ID. It contains its `run_id`, loop step, attempt number, provider/model route, lifecycle state, timing, token usage, context fullness, and provider request ID when available. Wingman stores the started record before dispatch. It settles the record when the provider stream ends.

A model call completes before requested tools run. A later tool failure can fail the run. It does not change the successful upstream attempt to failed. Steps restart at one for each run. Run-scoped identity keeps each turn history.

When clients show session usage or context-window fullness after a reload, use the latest model call. Do not estimate it from transcript text.
