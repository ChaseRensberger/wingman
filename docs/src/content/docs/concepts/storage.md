---
title: "Storage"
group: "Core"
order: 105
---

# Storage

Storage is a core Wingman primitive. It persists agents, clients, Workspaces, sessions, message history, content parts, and provider auth credentials. Persistence is not implemented as a plugin because session history must be durable by default and storage failures should be surfaced directly by the runtime.

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
| `sessions` | Session metadata: title, working directory, client ID, optional Workspace ID, timestamps. |
| `messages` | Ordered message rows for each session. |
| `model_calls` | One row per upstream model-call attempt, including provider/model provenance, finish state, usage, and context-window fullness. |
| `parts` | Ordered typed content parts for each message. |
| `auth` | Local provider credentials, stored as JSON. |
| `schema_migrations` | Applied migration versions. |

Sessions do not store `agent_id` or `model_ref`. Agents and models are selected per message. Assistant messages are linked to `model_calls`, which are the durable record of the provider/model route and usage for that turn.

Sessions created with `workspace_id` store the Workspace relationship and, when the Workspace has a path, a working-directory snapshot. Later Workspace path changes do not rewrite existing sessions.

## Model Calls

`model_calls` stores normalized accounting for each upstream model request:

- Provider, API, model ID, and requested model ref.
- Status, finish reason, stop reason, and error fields.
- Input, output, reasoning, cached-input, cache-write, total, and context token counts.
- Context window and computed context percentage.

The latest model call for a session lets clients show token count and context-window fullness after a page reload without estimating from transcript text.

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

Schema migrations live in `store/migrations` and are embedded into the Go binary. `NewSQLiteStore` runs pending migrations when the store opens.

Migration files use this naming pattern:

```text
0001_init.sql
0002_agent_model_ref.sql
```

The runner applies migrations in order and refuses gaps, which prevents accidentally deleting a migration that existing databases may depend on.

## SQLite Settings

Wingman configures SQLite for local daemon use:

| Setting | Value | Why |
|---|---|---|
| `journal_mode` | `WAL` | Allows readers while a writer is active. |
| `synchronous` | `NORMAL` | Good developer-tool performance with acceptable durability. |
| `foreign_keys` | `ON` | Enforces cascade behavior in the schema. |
| `busy_timeout` | `5000` | Waits briefly on lock contention. |
| `MaxOpenConns` | `1` | Serializes writes through one connection. |

SQLite is the durable store provided by Wingman. The storage boundary is adapter-shaped for embedded Go applications.

## Store Interface

Embedding applications can provide their own implementation of `store.Store`. This is a Go adapter boundary, not a plugin hook:

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
    UpdateSession(session *Session) error
    DeleteSession(id string) error

    UpsertMessage(ctx context.Context, msg StoredMessage) error
    UpsertPart(ctx context.Context, part StoredPart) error
    ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error)

    UpsertModelCall(ctx context.Context, call ModelCall) error
    LatestModelCall(ctx context.Context, sessionID string) (*ModelCall, error)
    ListModelCalls(ctx context.Context, sessionID string) ([]ModelCall, error)

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
