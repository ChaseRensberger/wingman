---
title: "Storage"
group: "Core"
order: 105
---

# Storage

Storage persists agents, clients, auth sessions, Workspaces, session history,
tool execution, and provider credentials. Session history is durable by default
when the server uses storage.

## Default Store

The stock server uses SQLite unless started in ephemeral mode.

Default path:

```text
~/.local/share/wingman/wingman.db
```

Override it with `--db`:

```bash
wingman serve --db ./wingman.db
```

Run without persistence:

```bash
wingman serve --ephemeral
```

Ephemeral mode is for one-shot or embedding scenarios where durable agents and sessions are not needed. Persisted HTTP endpoints return not-implemented responses in that mode.

## What Is Stored

The SQLite schema stores:

| Table | Purpose |
|---|---|
| `agents` | Agent definitions: instructions, tool names, model ref, options, output schema. |
| `clients` | API consumer identities, including the built-in `Wingman` default client. |
| `auth_sessions` | Hashed, expiring, and revocable browser or native client sessions. |
| `workspaces` | Client-owned saved contexts used to group sessions and optionally seed working directories. |
| `sessions` | Session metadata: title, working directory, client ID, optional Workspace ID, and timestamps. |
| `session_runs` | Durably admitted session work, request identity, immutable execution snapshot, and status. |
| `session_events` | Public session event history used for SSE replay. |
| `messages` | Ordered message rows for each session. |
| `model_calls` | One row per physical upstream model attempt, including run identity, provider/model provenance, lifecycle state, usage, and context-window fullness. |
| `tool_uses` | One row per model-proposed tool invocation, including durable identity, ownership, lifecycle state, input, model-facing text, structured content, client metadata, error, and timing. |
| `permission_requests` | Pending and terminal interactive decisions linked to session runs and tool uses. |
| `permission_grants` | Exact action/resource approvals remembered for one session. |
| `parts` | Ordered typed content parts for each message. |
| `auth` | Local provider credentials, stored as JSON. |
| `schema_migrations` | Applied migration versions, names, and SQL checksums. |

Sessions do not store `agent_id` or `model_ref`. Agents and models are selected per message. Assistant messages are linked to `model_calls`, which are the durable record of the provider/model route and usage for that turn.

Sessions created with `workspace_id` store the Workspace relationship and, when the Workspace has a path, a working-directory snapshot. Later Workspace path changes do not rewrite existing sessions.

Each admitted run records the prompt, effective Agent and model, output schema,
client, working directory, and status. Deleting a session permanently removes
its associated data.

## Model Calls

`model_calls` stores each upstream model request:

- Stable call ID, durable run ID, loop step, and physical attempt number.
- Provider, API, model ID, and requested model ref.
- Provider request ID when the upstream response supplies one.
- Status, finish reason, stop reason, and error fields.
- Input, output, reasoning, cached-input, cache-write, total, and context token counts.
- Context window and computed context percentage.

Calls include their status, timing, usage, and any provider request ID. Lists
are returned in start-time order across runs.

## Tool Uses

`tool_uses` is the authoritative execution record for model-proposed tools. A
row is linked to its run, physical model call, assistant message, stable part,
step, source ordinal, and provider call ID. Its Wingman-owned `tlu_` ID remains
stable through this lifecycle:

```text
proposed -> authorized -> started -> completed | failed | interrupted
         \-> declined
```

Wingman stores rewritten input at authorization and commits `started` before
calling the tool implementation. Unknown tools, invalid input, skipped hooks,
permission denial, rejected approval, and unavailable non-interactive approval
become `declined` without running. Interactive `ask` requests remain proposed
while they wait and can proceed only after approval. Startup interrupts pending
permission requests and unfinished tool rows before queued work resumes.

Wingman does not automatically replay interrupted tool uses.

## Message Parts

Messages are split into typed parts so the store can preserve provider-neutral model content:

- Text parts.
- Image parts.
- Reasoning parts.
- Tool call parts.
- Tool result parts.
- Plugin-defined opaque parts.

The store treats part payloads as opaque JSON. Interpretation belongs to the model/session layer and any plugin that registered a custom part type.

## Migrations

Pending schema migrations run when the store opens. Wingman does not start if
the applied migration history is invalid.

SQLite is the durable store provided by Wingman. Embedded Go applications can provide a different store implementation.

## Embedding

Embedded Go applications can provide a different store implementation. See the
exported `store.Store` interface for its current contract.
