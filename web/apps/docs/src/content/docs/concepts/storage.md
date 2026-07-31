---
title: "Storage"
group: "Core"
order: 105
---

# Storage

Storage persists agents, clients, Workspaces, sessions, message history, content parts, and provider auth credentials. Session history is durable by default when the server runs with storage enabled.

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
| `workspaces` | Client-owned saved contexts used to group sessions and optionally seed working directories. |
| `sessions` | Session metadata projection: title, working directory, client ID, optional Workspace ID, timestamps, aggregate version. |
| `session_runs` | Durably admitted session work, request identity, immutable execution snapshot, and status. |
| `session_events` | Public session event history used for SSE replay. |
| `aggregate_events` | Internal append-only session creation, metadata, and run-admission facts used to rebuild critical projections. |
| `messages` | Ordered message rows for each session. |
| `model_calls` | One row per physical upstream model attempt, including run identity, provider/model provenance, lifecycle state, usage, and context-window fullness. |
| `tool_uses` | One row per model-proposed tool invocation, including durable identity, ownership, lifecycle state, input, result, error, and timing. |
| `parts` | Ordered typed content parts for each message. |
| `auth` | Local provider credentials, stored as JSON. |
| `schema_migrations` | Applied migration versions, names, and SQL checksums. |

Sessions do not store `agent_id` or `model_ref`. Agents and models are selected per message. Assistant messages are linked to `model_calls`, which are the durable record of the provider/model route and usage for that turn.

Sessions created with `workspace_id` store the Workspace relationship and, when the Workspace has a path, a working-directory snapshot. Later Workspace path changes do not rewrite existing sessions.

Session creation, rename, move, and run admission are event-sourced. Admission
commits `session.run.admitted`, the run projection, the advanced session
version, and the public queued event in one transaction. The run records the
prompt, effective Agent and model, output schema, client, and placement used by
execution. Messages, model calls, and tool calls remain direct state records.
Hard purge deletes the session projection, aggregate stream, and all
session-owned table rows in one transaction. See [Durable Events and
Projections](/concepts/durable-events).

## Model Calls

`model_calls` stores normalized accounting for each upstream model request:

- Stable call ID, durable run ID, loop step, and physical attempt number.
- Provider, API, model ID, and requested model ref.
- Provider request ID when the upstream response supplies one.
- Status, finish reason, stop reason, and error fields.
- Input, output, reasoning, cached-input, cache-write, total, and context token counts.
- Context window and computed context percentage.

Wingman writes `started` immediately before dispatch and updates that same call
ID when the provider stream completes, fails, or is canceled. A successful model
call is settled before requested tools execute, so a later tool failure does not
misclassify the upstream attempt. Calls are associated with their assistant
message after that message is persisted.

Each durable attempt is unique within its `run_id`, step, and attempt number.
Steps restart at one for each run without overwriting earlier history. Wingman
does not currently retry provider requests, so `attempt` is presently `1`.

The latest model call with usage for a session lets clients show token count and
context-window fullness after a page reload without estimating from transcript
text. Lists are returned in physical start-time order across runs.

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
permission denial, and approval-needed calls become `declined` without running.
Startup changes unfinished rows to `interrupted` before queued work resumes.

The started record is a durability fence, not an exactly-once guarantee. A hard
crash can leave it ambiguous whether an external side effect completed, so
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

Schema migrations live in `store/migrations` and are embedded into the Go
binary. `NewSQLiteStore` validates the applied migration history and runs
pending migrations when the store opens.

Migration files use this naming pattern:

```text
0001_init.sql
```

The runner applies migrations in order. Every migration and its journal record
commit in the same SQLite transaction, so a failed migration is rolled back and
remains pending for the next startup.

The journal must be a contiguous prefix of the migrations embedded in the
binary. Each record must have the expected name and SQL checksum. Wingman
refuses to start when it finds a missing, renamed, unknown, or modified applied
migration rather than guessing how to repair the database.

### Pre-Beta Policy

Wingman currently ships one canonical `0001_init.sql`. While the project is
pre-beta and has no external data-compatibility commitment, schema changes are
folded into that initial migration. This keeps fresh databases representative of
the intended design instead of preserving transitional tables, `_legacy`
rebuilds, and speculative backfills.

Changing the initial migration intentionally invalidates existing local
databases. Delete the configured database and restart Wingman to create the new
schema. There is no automatic upgrade guarantee during this phase.

The migration runner, checksummed journal, ordering validation, and transactional
application remain in place. Once Wingman has external users or a declared data
compatibility boundary, `0001_init.sql` will be frozen and later schema changes
will use new append-only migration files.

At that point, migrations should preserve committed user data, avoid large
startup backfills, and use resumable application work when a transformation is
too large for one startup transaction.

## SQLite Settings

Wingman configures SQLite for local daemon use:

| Setting | Value | Why |
|---|---|---|
| `journal_mode` | `WAL` | Allows readers while a writer is active. |
| `synchronous` | `NORMAL` | Good developer-tool performance with acceptable durability. |
| `foreign_keys` | `ON` | Enforces cascade behavior in the schema. |
| `busy_timeout` | `5000` | Waits briefly on lock contention. |
| `MaxOpenConns` | `1` | Serializes writes through one connection. |

SQLite is the durable store provided by Wingman. Embedded Go applications can provide a different store implementation.

## Store Interface

Embedding applications can provide their own implementation of `store.Store`:

```go
type Store interface {
    CreateAgent(agent *Agent) error
    GetAgent(id string) (*Agent, error)
    ListAgents() ([]*Agent, error)
    UpdateAgent(agent *Agent) error
    DeleteAgent(id string) error

    CreateSession(session *Session) error
    GetSession(id string) (*Session, error)
    ListSessions() ([]*Session, error)
    ListSessionsByClient(clientID string) ([]*Session, error)
    ListSessionsByWorkspace(workspaceID string) ([]*Session, error)
    RenameSession(ctx context.Context, id, title string, expectedVersion int64) (*Session, error)
    MoveSession(ctx context.Context, id, workDir, workspaceID string, expectedVersion int64) (*Session, error)
    PurgeSession(ctx context.Context, id string, expectedVersion int64) error

    AdmitSessionRun(ctx context.Context, run SessionRun) (SessionRunAdmission, error)
    ClaimNextSessionRun(ctx context.Context, sessionID string) (*SessionRun, error)
    CompleteSessionRun(ctx context.Context, id, status, errorMessage string) error
    ListQueuedSessionRunSessions(ctx context.Context) ([]string, error)
    AbortRunningSessionRuns(ctx context.Context) error

    UpsertMessage(ctx context.Context, msg StoredMessage) error
    UpsertPart(ctx context.Context, part StoredPart) error
    ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error)

    UpsertModelCall(ctx context.Context, call ModelCall) error
    LatestModelCall(ctx context.Context, sessionID string) (*ModelCall, error)
    ListModelCalls(ctx context.Context, sessionID string) ([]ModelCall, error)

    AppendSessionEvent(ctx context.Context, event SessionEvent) (SessionEvent, error)
    ListSessionEvents(ctx context.Context, sessionID string, after int64, limit int) ([]SessionEvent, error)

    CreateClient(name string) (*Client, error)
    EnsureDefaultClient() (*Client, error)
    GetClient(id string) (*Client, error)
    ListClients() ([]*Client, error)

    CreateWorkspace(workspace *Workspace) error
    GetWorkspace(id string) (*Workspace, error)
    ListWorkspaces() ([]*Workspace, error)
    ListWorkspacesByClient(clientID string) ([]*Workspace, error)
    UpdateWorkspace(workspace *Workspace) error
    DeleteWorkspace(id string) error

    GetAuth() (*Auth, error)
    SetAuth(auth *Auth) error

    Close() error
}
```

`store/memory` provides an in-memory implementation used by tests and embedding scenarios.

SQLite and the memory store additionally implement the optional
`store.AggregateEventReader` interface for internal replay and diagnostics. It
is separate from `Store` so adapters that do not expose aggregate history are
not forced to implement a runtime capability Wingman does not yet require.
