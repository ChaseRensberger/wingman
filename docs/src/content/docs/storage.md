---
title: "Storage"
group: "Concepts"
draft: false
order: 108
---

# Storage

The HTTP server persists state in SQLite via the `wingagent/storage.Store` interface. The SDK does not require storage; you can run sessions entirely in memory and decide your own persistence strategy.

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
    UpdateSession(session *Session) error
    AppendMessage(sessionID string, msg wingmodels.Message) error
    ReplaceMessages(sessionID string, msgs []wingmodels.Message) error
    DeleteSession(id string) error

    GetAuth() (*Auth, error)
    SetAuth(auth *Auth) error

    Close() error
}
```

Notes:

- **`UpdateSession` is metadata-only.** It persists `work_dir` and `updated_at`. It does NOT touch message history.
- **`AppendMessage`** appends a single message (and its parts) at the next index. This is the routine path for incremental persistence — wire it to a session via `WithMessageSink`.
- **`ReplaceMessages`** atomically clears history and writes `msgs` in order. Reserved for power users (rehydration tools, history editors); routine traffic uses `AppendMessage`.

## Incremental persistence

The server wires every session it builds with a message sink that calls `AppendMessage`:

```go
s := session.New(
    session.WithModel(model),
    // ... agent config ...
    session.WithMessageSink(func(m wingmodels.Message) {
        if err := store.AppendMessage(sessionID, m); err != nil {
            log.Printf("append: %v", err)
        }
    }),
)
```

The sink fires for every complete message added to history during the run, including plugin-injected messages such as compaction markers. It is synchronous; the callback must not block.

Because messages are persisted as they happen, partial transcripts survive crashes and aborts.

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
    WorkDir   string
    History   []wingmodels.Message
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

`storage.NewID(prefix)` mints a new ID. `storage.ParseID(id)` validates a known prefix and splits `(prefix, body)` — useful at API boundaries to catch a session id where an agent id was expected.

## Schema

The migration in `wingagent/storage/migrations/0001_init.sql` defines:

- `agents` — agent records, with `output_schema_json` as a separate column
- `sessions` — session metadata
- `messages` — one row per message, ordered by `(session_id, idx)`
- `parts` — one row per part, ordered by `(message_id, idx)`, with the discriminator `type` and the JSON body
- `auth` — provider credentials keyed by provider id

Parts are stored individually (not as JSON arrays inside messages) so partial reads and selective updates are straightforward, and so the same per-part `type` discriminator works for storage and the SSE wire.

## Opening a store

```go
import "github.com/chaserensberger/wingman/wingagent/storage"

store, err := storage.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

`storage.DefaultDBPath()` returns the server's default location (`~/.local/share/wingman/wingman.db`).

## Plugin parts in storage

Custom Part types ([Parts](./parts)) round-trip losslessly. Even when the originating plugin is uninstalled, the parts come back as `OpaquePart` values preserving the original bytes — UIs may render a placeholder or skip them, and re-marshaling reproduces the original payload.
