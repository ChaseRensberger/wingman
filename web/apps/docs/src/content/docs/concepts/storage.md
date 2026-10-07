---
title: "Storage"
group: "Core"
order: 105
---

# Storage

Wingman saves agents, sessions, runs, and provider credentials in SQLite by default.

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

Ephemeral mode does not save runs. Endpoints for stored resources return `501 Not Implemented`.

## What Is Stored

The database contains:

| Table                 | Purpose                                                                                       |
| --------------------- | --------------------------------------------------------------------------------------------- |
| `agents`              | Agent definitions: instructions, tool names, model ref, options, output schema.               |
| `clients`             | API consumer identities, including the built-in `WingClient` default client.                  |
| `workspaces`          | Session groups with optional working directories.                                             |
| `sessions`            | Session metadata: title, working directory, client ID, optional Workspace ID, and timestamps. |
| `session_runs`        | Queued work, saved input, request ID, and status.                                             |
| `session_events`      | Public session event history used for SSE replay.                                             |
| `messages`            | Ordered message rows for each session.                                                        |
| `model_calls`         | Provider attempts, status, timing, usage, and errors.                                         |
| `tool_uses`           | Tool calls, input, results, status, and timing.                                               |
| `permission_requests` | Tool approval requests and decisions.                                                         |
| `permission_grants`   | Exact action/resource approvals remembered for one session.                                   |
| `parts`               | Ordered typed content parts for each message.                                                 |
| `auth`                | Local provider credentials, stored as JSON.                                                   |
| `schema_migrations`   | Applied migration versions, names, and SQL checksums.                                         |

Each queued run saves its input and configuration. Later agent edits or session moves do not change queued work.
Deleting a session permanently removes its associated records.

## Model Calls

`model_calls` stores each upstream model request:

- Call ID, run ID, step, and attempt number.
- Provider, API, model ID, and requested model ref.
- Provider request ID when the upstream response supplies one.
- Status, finish reason, stop reason, and error fields.
- Input, output, reasoning, cached-input, cache-write, total, and context token counts.
- Context window and computed context percentage.

Calls also include timing. Lists use start-time order across runs.

## Tool Uses

`tool_uses` records execution status. Each call has a stable `tlu_` ID and follows this lifecycle:

```text
proposed -> authorized -> started -> completed | failed | interrupted
         \-> declined
```

Wingman saves authorized input and records `started` before execution.
On restart, unfinished calls become `interrupted`. Recovery reuses saved results and resumes calls that did not start.
Started calls can run again only when their saved and current tool definitions permit replay.
See [Tools](/concepts/tools#durable-execution-lifecycle) for validation and approval behavior.

## Message Parts

Messages contain text, images, reasoning, tool calls, and plugin content as JSON parts.
See [Sessions](/concepts/sessions#message-parts) for message state and client display.

With the compaction plugin installed, execution loads the latest completed compaction marker and the messages after it.
Earlier messages remain in storage and in the session history API. Compaction does not delete them.
Model-call metadata loads only for the active range. Run recovery reads its own saved records separately.

## Migrations

Pending schema migrations run when the store opens. If the applied migration history is invalid, Wingman does not start.

The run-recovery update requires a new database. Databases from before this update fail migration checksum validation and cannot open with this version.
There is no automatic upgrade for those databases.
To preserve existing data, stop Wingman and keep a backup of the database and any associated `-wal` and `-shm` files.
Start this version with `--db` set to a new path. The new database contains no previous sessions, agents, or provider credentials.

## Embedding

Embedded Go applications can provide another implementation of `store.Store`.
Custom stores must support `QueryMessages`, `QueryModelCalls`, and `ListRunToolUses` without loading excluded message content.
Message queries retain absolute history indexes. A boundary query starts at the latest completed message that contains the requested part type.
