---
title: "Storage"
group: "Core"
order: 105
---

# Storage

Storage is a core Wingman primitive. It persists agents, clients, sessions, message history, content parts, and provider auth credentials. Persistence is not implemented as a plugin because session history must be durable by default and storage failures should be surfaced directly by the runtime.

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

Ephemeral mode is for one-shot or embedding scenarios where durable agents and sessions are not needed. Some persisted HTTP endpoints intentionally return not-implemented responses in that mode.

## What Is Stored

The SQLite schema stores:

| Table | Purpose |
|---|---|
| `agents` | Agent definitions: instructions, tool names, model ref, options, output schema. |
| `clients` | Optional API consumer identities. |
| `sessions` | Session metadata: title, working directory, client ID, timestamps. |
| `messages` | Ordered message rows for each session. |
| `parts` | Ordered typed content parts for each message. |
| `auth` | Local provider credentials, stored as JSON. |
| `schema_migrations` | Applied migration versions. |

Sessions do not store `agent_id` or `model_ref`. Agents and models are selected per message, so historical messages are the durable record of what happened.

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

This is intentionally single-process storage. If Wingman grows a remote/multi-tenant deployment mode, Postgres or another external store can sit behind the same store interface.

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
    UpdateSession(session *Session) error
    DeleteSession(id string) error

    UpsertMessage(ctx context.Context, msg StoredMessage) error
    UpsertPart(ctx context.Context, part StoredPart) error
    ListMessages(ctx context.Context, sessionID string) ([]StoredMessage, error)

    CreateClient(name string) (*Client, error)
    GetClient(id string) (*Client, error)
    ListClients() ([]*Client, error)

    GetAuth() (*Auth, error)
    SetAuth(auth *Auth) error

    Close() error
}
```

`store/memory` provides an in-memory implementation used by tests and embedding scenarios.
