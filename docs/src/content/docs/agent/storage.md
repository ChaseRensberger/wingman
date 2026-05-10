---
title: "Storage"
group: "Concepts"
draft: true
order: 108
---

# Storage

The HTTP server persists state in SQLite via the `store.Store` interface. Sessions wire persistence directly via `session.WithStore`. The SDK does not require storage; you can run sessions entirely in memory and decide your own persistence strategy.

## The `Store` interface

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

Notes:

- **`UpdateSession` is metadata-only.** It persists `title` and `updated_at`. It does NOT touch message history.
- **`UpsertMessage`** / **`UpsertPart`** insert or update a single message or part row keyed by ID. This is the routine path for incremental persistence — the session calls these automatically when `WithStore` is configured.
- **`ListMessages`** returns all messages for a session ordered by `Idx`, with parts populated and ordered by `Sequence`.

## `WithStore`

`session.WithStore(store)` wires both sides of persistence directly into the session:

- **Hydration:** on the first `Run` when in-memory history is empty, the session calls `store.ListMessages(ctx, sessionID)` and rebuilds its history from the returned rows.
- **Upserts:** every new message (user, assistant, and tool results) is persisted via `UpsertMessage` and `UpsertPart` as it is produced. Errors propagate and fail the run.

```go
import (
    "github.com/chaserensberger/wingman/agent/session"
    wstorage "github.com/chaserensberger/wingman/store"
)

store, err := wstorage.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Ensure the session row exists (CreateSession to mint a new one,
// GetSession to verify an existing id).
sess, err := store.GetSession(sessionID)
if err != nil {
    log.Fatal(err)
}

s := session.New(
    session.WithID(sess.ID),
    session.WithModel(model),
    session.WithStore(store),
)
```

## `WithMessageSink` — lower-level primitive

`session.WithMessageSink(fn)` installs a callback that fires for every `MessageEvent`. It remains supported for ad-hoc observation:

```go
s := session.New(
    session.WithModel(model),
    session.WithMessageSink(func(m models.Message) {
        log.Printf("turn: %s (%d parts)", m.Role, len(m.Content))
    }),
)
```

For persistence, prefer `WithStore`: it bundles load and save together, so you can't accidentally wire one without the other. `WithMessageSink` is appropriate when you only need observation (logging, metrics, UI fanout) and not load-side rehydration.

## Stored types

```go
type Agent struct {
    ID           string
    Name         string
    Instructions string
    Tools        []string         // built-in tool names
    Provider     string           // e.g. "anthropic"
    Model        string           // e.g. "claude-haiku-4-5"
    Options      map[string]any
    OutputSchema map[string]any
    CreatedAt    string
    UpdatedAt    string
}

type Session struct {
    ID        string
    Title     string
    WorkDir   string
    History   []models.Message
    CreatedAt string
    UpdatedAt string
}

type AuthCredential struct {
    Type string // "api_key"
    Key  string
}

type Auth struct {
    Providers map[string]AuthCredential
    UpdatedAt string
}
```

## ID prefixes

Every persisted entity carries a stable prefix so IDs are self-describing in logs and URLs.

| Prefix | Entity |
|---|---|
| `agt_` | Agent |
| `ses_` | Session |
| `msg_` | Message |
| `prt_` | Part |
| `tlu_` | Tool use |

IDs are KSUIDs (27 base62 chars after the prefix). KSUID over ULID: smaller wire size, time-resolution sortable without monotonic-entropy state, valid through year 2150.

`store.NewID(prefix)` mints a new ID. `store.ParseID(id)` validates a known prefix and splits `(prefix, body)` — useful at API boundaries to catch a session id where an agent id was expected.

## Schema

The migration in `store/migrations/0001_init.sql` defines:

- `agents` — agent records, with `output_schema_json` as a separate column
- `sessions` — session metadata
- `messages` — one row per message, ordered by `(session_id, idx)`
- `parts` — one row per part, ordered by `(message_id, idx)`, with the discriminator `type` and the JSON body
- `auth` — provider credentials keyed by provider id

Parts are stored individually (not as JSON arrays inside messages) so partial reads and selective updates are straightforward, and so the same per-part `type` discriminator works for storage and the SSE wire.

## Opening a store

```go
import "github.com/chaserensberger/wingman/store"

store, err := store.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

`store.DefaultDBPath()` returns the server's default location (`~/.local/share/wingman/wingman.db`).

## Plugin parts in storage

Custom Part types ([Parts](../models/parts)) round-trip losslessly. Even when the originating plugin is uninstalled, the parts come back as `OpaquePart` values preserving the original bytes — UIs may render a placeholder or skip them, and re-marshaling reproduces the original payload.

## Stored types

```go
type Agent struct {
    ID           string
    Name         string
    Instructions string
    Tools        []string         // built-in tool names
    Provider     string           // e.g. "anthropic"
    Model        string           // e.g. "claude-haiku-4-5"
    Options      map[string]any
    OutputSchema map[string]any
    CreatedAt    string
    UpdatedAt    string
}

type Session struct {
    ID        string
    Title     string
    WorkDir   string
    History   []models.Message
    CreatedAt string
    UpdatedAt string
}

type AuthCredential struct {
    Type string // "api_key"
    Key  string
}

type Auth struct {
    Providers map[string]AuthCredential
    UpdatedAt string
}
```

## ID prefixes

Every persisted entity carries a stable prefix so IDs are self-describing in logs and URLs.

| Prefix | Entity |
|---|---|
| `agt_` | Agent |
| `ses_` | Session |
| `msg_` | Message |
| `prt_` | Part |
| `tlu_` | Tool use |

IDs are KSUIDs (27 base62 chars after the prefix). KSUID over ULID: smaller wire size, time-resolution sortable without monotonic-entropy state, valid through year 2150.

`store.NewID(prefix)` mints a new ID. `store.ParseID(id)` validates a known prefix and splits `(prefix, body)` — useful at API boundaries to catch a session id where an agent id was expected.

## Schema

The migration in `store/migrations/0001_init.sql` defines:

- `agents` — agent records, with `output_schema_json` as a separate column
- `sessions` — session metadata
- `messages` — one row per message, ordered by `(session_id, idx)`
- `parts` — one row per part, ordered by `(message_id, idx)`, with the discriminator `type` and the JSON body
- `auth` — provider credentials keyed by provider id

Parts are stored individually (not as JSON arrays inside messages) so partial reads and selective updates are straightforward, and so the same per-part `type` discriminator works for storage and the SSE wire.

## Opening a store

```go
import "github.com/chaserensberger/wingman/store"

store, err := store.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

`store.DefaultDBPath()` returns the server's default location (`~/.local/share/wingman/wingman.db`).

## Plugin parts in storage

Custom Part types ([Parts](../models/parts)) round-trip losslessly. Even when the originating plugin is uninstalled, the parts come back as `OpaquePart` values preserving the original bytes — UIs may render a placeholder or skip them, and re-marshaling reproduces the original payload.
