---
title: "Sessions"
---

# Sessions

A session stores a conversation and runs work for its messages. Each submitted message can use a different agent or model.
One turn is the response to a message, including model and tool steps. A run is the saved execution record for that turn.
Saved sessions survive server restarts. Wingman resumes unfinished work when it can do so safely.

A [Workspace](/concepts/workspaces) groups sessions and can supply their initial working directory.

## Create Then Send

These examples use `wingman api`, which connects to the local managed service.
Create a session before you send a message:

```bash
SESSION_ID=$(wingman api createSession \
  -d "$(jq -n --arg path "$PWD" '{title: "Explore repo", working_directory: $path}')" | jq -r .id)
```

Create a session in a Workspace:

```bash
WORKSPACE_ID=$(wingman api listWorkspaces | jq -er '.[0].id // error("Create a workspace first")')

SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Send either `working_directory` or `workspace_id`, not both. Wingman copies a Workspace path to the session's `work_dir`.

## Rename And Move

Saved session responses include a `version`. Read this value immediately before each change:

```bash
VERSION=$(wingman api getSession --param "id=${SESSION_ID}" | jq -r .version)
```

Use the value to rename the session:

```bash
wingman api renameSession --param "id=${SESSION_ID}" \
  -d "$(jq -n --argjson version "$VERSION" '{title: "Investigate retries", expected_version: $version}')"
```

To move a session, read its version again. Send exactly one of `working_directory` or `workspace_id`:

```bash
VERSION=$(wingman api getSession --param "id=${SESSION_ID}" | jq -r .version)
wingman api moveSession --param "id=${SESSION_ID}" \
  -d "$(jq -n --argjson version "$VERSION" '{working_directory: "/home/me/other-project", expected_version: $version}')"
```

Changes increment `version`. An unchanged title or location leaves it unchanged.
If another client changes the session first, Wingman returns `409 Conflict`. Reload the session before you retry.

## Delete

Deleting a session permanently removes its history and associated records. Read its version again before you delete it:

```bash
VERSION=$(wingman api getSession --param "id=${SESSION_ID}" | jq -r .version)
wingman api deleteSession \
  --param "id=${SESSION_ID}" \
  --param "expected_version=${VERSION}"
```

Wingman cancels active work and closes event streams before deletion returns success.
A stale version returns `409 Conflict` without deleting the session or canceling work.

## Submit work

Replace `agt_...` with an existing agent ID. Send a message with an optional retry ID:

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

Queued work uses the agent, model, location, and [skills](/configure/skills) saved when the request was accepted.
Later agent edits, skill edits, and session moves affect only new requests.
Wingman rereads [project instructions](/configure/project-instructions) before each model call, so file changes can affect queued work and later calls in an active run.

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

The connection stays open after a run ends. Press Ctrl+C to stop reading without canceling work.
See [Streaming Events](/build-clients/streaming-events) for event formats and reconnect steps.

## Questions

A question is a request for information or a decision.
A session can ask its owning client through the `question` tool.
Applications can also create questions through `POST /sessions/{id}/questions` without a tool call.
These application-created questions do not suspend a run.

The tool accepts one or more prompts with suggested choices.
Each prompt accepts custom text. A prompt with `multiple: true` accepts more than one answer.
The Console displays pending questions above the message composer.

While a tool question waits, the run enters `awaiting_input` and releases its execution capacity.
Later messages in that session remain queued. Other sessions can continue.
The client can disconnect, and the server can restart, without losing the question or its answer.

Use these endpoints to manage questions:

| Method | Path                                          | Purpose                         |
| ------ | --------------------------------------------- | ------------------------------- |
| `GET`  | `/sessions/{id}/questions`                    | List saved questions.           |
| `POST` | `/sessions/{id}/questions`                    | Create an application question. |
| `GET`  | `/sessions/{id}/questions/{questionID}`       | Read one question.              |
| `POST` | `/sessions/{id}/questions/{questionID}/reply` | Answer or dismiss a question.   |

A reply with `status: "answered"` supplies `answers`, an array of string arrays in prompt order.
A reply with `status: "dismissed"` supplies no answers. The model receives the dismissal and can continue.
Canceling the run cancels its pending questions. A late answer cannot restart a canceled run.
An identical reply is safe to retry. A conflicting reply returns `409 Conflict`.

Question events use `session.question.pending`, `session.question.answered`, `session.question.dismissed`, and `session.question.cancelled`.
Each event contains the full question record. The `session.run.input` event reports the wait or resumption status.
See the [TypeScript example](/build-clients/typescript-sdk#answer-questions) for client calls.

## Run Status And Recovery

After a reload or disconnect, read the saved run status:

```bash
wingman api listSessionRuns --param "id=${SESSION_ID}"
wingman api getSessionRun --param "id=${SESSION_ID}" --param runID=run_...
```

A run moves from `queued` to `running`. It then moves to `completed`, `failed`,
or `aborted`. A tool question pauses the run in `awaiting_input`.
Remote validation pauses the run in `awaiting_completion`. A queued or paused run can also be aborted.

Abort a specific run with:

```bash
wingman api abortSessionRun --param "id=${SESSION_ID}" --param runID=run_...
```

After a restart, Wingman resumes queued messages and recovers interrupted message runs under their original run IDs.
Recovery returns a run to `queued`, then `running`. It does not submit the input again.
Wingman reuses saved model responses and tool results. It retries eligible interrupted model requests, which can incur another provider charge.
Partial responses remain in history as `failed`. Wingman excludes these incomplete responses from the recovered model request.
If a partial response records a provider-executed tool with an unknown result, recovery stops with `recovery_blocked`.

A replay-safe tool permits repeated execution with the same input.
Wingman retries an interrupted tool only when its saved and current definitions permit replay.
Calls that did not start can proceed through their remaining authorization and execution steps. Saved authorizations remain valid for the same call and input.
If a non-replayable tool started without a saved outcome, the run fails with `recovery_blocked`.
Inspect the tool record and its external effects before you submit another request.

Wingman permits three automatic recovery attempts per run. Further interruptions abort the run with `recovery_exhausted`.
Explicit cancellation remains terminal across restarts. Interrupted plugin actions remain `aborted` because they do not have a recovery contract.
Ephemeral runs do not recover. See [Tools](/concepts/tools#durable-execution-lifecycle) for replay rules.

## Completion validation

Your client can inspect each proposed answer before the run completes. Include this field in a message request to enable remote validation:

```json
{
  "agent_id": "agt_...",
  "message": "Implement the change and run its tests.",
  "completion": { "remote": true, "max_continuations": 3 }
}
```

After plugin validators accept an answer, the run enters `awaiting_completion`.
Read the latest entry in the run's `completions` array or listen for `session.run.completion` events.
The `candidate_id` identifies the saved assistant message. Use the same client identity for admission and replies.

Send a decision to `POST /sessions/{id}/runs/{runID}/completion`:

```json
{
  "candidate_id": "msg_...",
  "decision": { "action": "continue", "feedback": "Add a regression test for empty input." }
}
```

To accept the answer, send `"decision": { "action": "accept" }`.
Wingman saves continuation feedback as a user-role message with `metadata.source` set to `completion_validator`.
It continues the same run. Later submitted messages remain queued until that run ends.

A repeated identical reply does not create another iteration. A conflicting reply or an unknown candidate returns `409 Conflict`.
A disconnect or restart preserves the pending request. Waiting does not occupy an execution slot, and you can still abort the run.

The default limit is three continuations across plugin and remote decisions. Zero selects that default.
If validation requests more continuations, the run fails with `completion_limit`. Committed decisions do not run again after restart.
Remote validation requires a saved session. It is unavailable for `POST /run`.

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

If the server starts with `--ephemeral`, stored resources are unavailable.
Endpoints such as `/sessions`, `/agents`, `/clients`, `/workspaces`, and `/provider/auth` return `501 Not Implemented`.
In this mode, include the agent definition in each `/run` request.

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
