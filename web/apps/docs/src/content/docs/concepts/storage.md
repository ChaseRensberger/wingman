---
title: "Storage"
group: "Core"
order: 105
---

# Storage

Storage persists agents, clients, Workspaces, session history, tool execution, and provider credentials. Session history is durable by default when the server uses storage.

## Default Store

The stock server uses SQLite unless it starts in ephemeral mode.

Default path:

```text
~/.local/share/wingman/wingman.db
```

Use `--db` to override this path:

```bash
wingman serve --db ./wingman.db
```

Use this command to run without persistence:

```bash
wingman serve --ephemeral
```

Use ephemeral mode for one-shot or embedded scenarios without durable agents and sessions. Persisted HTTP endpoints return not-implemented responses in this mode.

## What Is Stored

The SQLite schema stores:

| Table                 | Purpose                                                                                                                                                                               |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `agents`              | Agent definitions: instructions, tool names, model ref, options, output schema.                                                                                                       |
| `clients`             | API consumer identities, including the built-in `Wingman` default client.                                                                                                             |
| `workspaces`          | Client-owned saved contexts used to group sessions and optionally seed working directories.                                                                                           |
| `sessions`            | Session metadata: title, working directory, client ID, optional Workspace ID, and timestamps.                                                                                         |
| `session_runs`        | Durably admitted session work, request identity, immutable execution snapshot, and status.                                                                                            |
| `session_events`      | Public session event history used for SSE replay.                                                                                                                                     |
| `messages`            | Ordered message rows for each session.                                                                                                                                                |
| `model_calls`         | One row per physical upstream model attempt, including run identity, provider/model provenance, lifecycle state, usage, and context-window fullness.                                  |
| `tool_uses`           | One row per model-proposed tool invocation, including durable identity, ownership, lifecycle state, input, model-facing text, structured content, client metadata, error, and timing. |
| `permission_requests` | Pending and terminal interactive decisions linked to session runs and tool uses.                                                                                                      |
| `permission_grants`   | Exact action/resource approvals remembered for one session.                                                                                                                           |
| `parts`               | Ordered typed content parts for each message.                                                                                                                                         |
| `auth`                | Local provider credentials, stored as JSON.                                                                                                                                           |
| `schema_migrations`   | Applied migration versions, names, and SQL checksums.                                                                                                                                 |

Sessions do not store `agent_id` or `model_ref`. Wingman selects agents and models for each message. Assistant messages link to `model_calls`. These rows are the durable record for provider/model routes and usage.

Sessions created with `workspace_id` store the Workspace relationship. If the Workspace has a path, the session also stores a working-directory snapshot. Later Workspace path changes do not rewrite existing sessions.

Each admitted run records the prompt, effective Agent and model, output schema, client, working directory, and status. Deleting a session permanently removes its associated data.

## Model Calls

`model_calls` stores each upstream model request:

- Stable call ID, durable run ID, loop step, and physical attempt number.
- Provider, API, model ID, and requested model ref.
- Provider request ID when the upstream response supplies one.
- Status, finish reason, stop reason, and error fields.
- Input, output, reasoning, cached-input, cache-write, total, and context token counts.
- Context window and computed context percentage.

Calls include their status, timing, usage, and any provider request ID. Lists
use start-time order across runs.

## Tool Uses

`tool_uses` is the authoritative execution record for model-proposed tools. A row links to its run, model call, assistant message, stable part, step, source ordinal, and provider call ID. Its Wingman-owned `tlu_` ID stays stable through this lifecycle:

```text
proposed -> authorized -> started -> completed | failed | interrupted
         \-> declined
```

Wingman stores rewritten input at authorization. It commits `started` before it calls the tool implementation. Unknown tools, invalid input, skipped hooks, permission denial, rejected approval, and unavailable non-interactive approval become `declined` without execution. Interactive `ask` requests stay proposed while they wait. They proceed only after approval. At startup, Wingman interrupts pending permission requests and unfinished tool rows before queued work resumes.

Wingman does not automatically replay interrupted tool uses.

## Message Parts

Messages use typed parts. This lets the store preserve provider-neutral model content:

- Text parts.
- Image parts.
- Reasoning parts.
- Tool call parts.
- Tool result parts.
- Plugin-defined opaque parts.

The store treats part payloads as opaque JSON. The model/session layer interprets the payloads. A plugin that registers a custom part type also interprets its payloads.

## Migrations

Pending schema migrations run when the store opens. If the applied migration history is invalid, Wingman does not start.

SQLite is the durable store that Wingman provides. Embedded Go applications can
provide a different store implementation.

## Embedding

Embedded Go applications can provide a different store implementation. See the
exported `store.Store` interface for the current contract.
