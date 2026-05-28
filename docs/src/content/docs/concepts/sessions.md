---
title: "Sessions"
group: "Core"
order: 102
---

# Sessions

A session is the runtime record for agent work. It owns message history, drives model turns, dispatches tool calls, emits events, and persists the transcript when storage is enabled.

This distinction is load-bearing:

- An agent is a reusable definition.
- A session is a running conversation or one-shot run.
- A session is not permanently bound to one agent.
- A session is not permanently bound to one model.
- Each message chooses the agent configuration for that turn.

That shape lets one session hand off between agents or models without creating a new conversation record.

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

Send a message:

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt_...",
    "message": "Summarize this project"
  }'
```

`POST /sessions/{id}/message` requires the session to exist. A typo in the ID returns `404`; it does not create a new session.

## Per-Message Agent And Model

Agents and models are selected per message:

```json
{
  "agent_id": "agt_...",
  "model_ref": "anthropic/claude-sonnet-4-6",
  "message": "Use the stronger model for this turn."
}
```

`model_ref` overrides the agent's default model for that request. If neither the request nor the agent provides a model, the run fails before the first provider call.

## Streaming

Use the streaming endpoint when a client needs live events:

```bash
curl -N -X POST "http://localhost:2323/sessions/${SESSION_ID}/message/stream" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{
    "agent_id": "agt_...",
    "message": "Inspect this repository and report back."
  }'
```

The response is server-sent events. Each `data:` payload is a Wingman event envelope containing `type`, `version`, and `data`.

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
      "model_ref": "anthropic/claude-sonnet-4-6"
    },
    "message": "Explain Wingman in one paragraph."
  }'
```

An ephemeral run has a runtime, tools, model calls, and events. It is not written to the store.

When the server is started with `--ephemeral`, persisted endpoints such as `/sessions`, `/agents`, `/clients`, `/workspaces`, and `/provider/auth` return `501 Not Implemented`. Use inline agent specs with `/run` in that mode.

## Working Directory

A session can have a working directory. Directory-scoped tools such as `read`, `glob`, `grep`, `write`, `edit`, `apply_patch`, and `bash` use that directory as their workspace.

Sessions without a working directory are valid if the selected agent only uses tools that do not need one, such as `webfetch`.

A session created with `workspace_id` stores a snapshot of that Workspace's path as `work_dir`. Changing the Workspace later affects future sessions, not existing session history or existing `work_dir` values.

## Message Parts

Session history is stored as messages with typed parts. A part is Wingman's provider-neutral content block:

- Text.
- Image.
- Reasoning.
- Tool call.
- Tool result.
- Structured output.
- Plugin-defined opaque content.

Tool result parts contain model-facing text and may also contain metadata for clients. File-editing tools use this metadata to expose changed files, patches, and addition/deletion counts so UIs can render diffs without parsing prose. Parts let Wingman preserve provider-specific richness without storing provider-native wire formats. UIs can render each block differently, and plugins can introduce custom content.

## Usage And Context

Persisted sessions also store normalized model-call records for assistant turns. A model call captures the provider/model route, finish state, token usage, context token count, context window, and computed context percentage reported by the provider path.

Clients should use the latest model call, not transcript text estimation, when showing session usage or context-window fullness after reload.

## Creating Related Sessions

If a client wants one session to inform another, it creates a new session through the same HTTP API and passes the relevant context in the first message. Wingman does not implicitly copy parent context into new sessions.
