---
title: "Storage"
group: "Concepts"
draft: false
order: 108
---

# Storage

The HTTP server persists state in SQLite via the `storage.Store` interface. Persistence is exposed to sessions through the **storage plugin**, which packages both load (rehydrate prior history) and save (append new messages) behind a single `session.WithPlugin` call. The SDK does not require storage; you can run sessions entirely in memory and decide your own persistence strategy.

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

- **`UpdateSession` is metadata-only.** It persists `title`, `work_dir`, and `updated_at`. It does NOT touch message history.
- **`AppendMessage`** appends a single message (and its parts) at the next index. This is the routine path for incremental persistence — wire it via the storage plugin (recommended) or `WithMessageSink` (low-level).
- **`ReplaceMessages`** atomically clears history and writes `msgs` in order. Reserved for power users (rehydration tools, history editors); routine traffic uses `AppendMessage`.

## The storage plugin

`storageplugin.NewPlugin(store, sessionID)` returns a [Plugin](./plugins) that wires both sides of persistence to `store` and `sessionID`:

- A **`BeforeRun`** hook calls `store.GetSession(sessionID)` and returns `sess.History` as the loop's initial messages — so a session resumed across processes (or just across HTTP requests) starts with the same context the prior run ended with.
- A **sink** filters for `loop.MessageEvent` and calls `store.AppendMessage` for each completed message — so every turn (and any plugin-injected message such as a compaction marker) lands in storage as it happens.

```go
import (
    "github.com/chaserensberger/wingman/plugins/storage"
    "github.com/chaserensberger/wingman/wingagent/session"
    wstorage "github.com/chaserensberger/wingman/storage"
)

store, err := wstorage.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()

// Ensure the session row exists (CreateSession to mint a new one,
// GetSession to verify an existing id). The plugin loads from this id.
sess, err := store.GetSession(sessionID)
if err != nil {
    log.Fatal(err)
}

s := session.New(
    session.WithModel(model),
    session.WithPlugin(storageplugin.NewPlugin(store, sess.ID)),
)
```

The plugin's `Name()` is `"storage"`. Installing two storage plugins in the same session fails the run — the [plugin registry](./plugins#the-plugin-interface) enforces name uniqueness so you can't accidentally have two BeforeRun hooks fighting over initial history.

Sink-side errors (an `AppendMessage` failure) are logged and swallowed: a single SQLite hiccup shouldn't kill an in-flight run. The transcript on disk may end up with gaps in the rare error case, but the in-memory transcript returned in `Result.Messages` is always correct. `BeforeRun` errors do fail the run, since proceeding without a known starting state would silently desynchronize the in-memory and on-disk transcripts.

## `WithMessageSink` — lower-level primitive

`session.WithMessageSink(fn)` installs a callback that fires for every `MessageEvent`. It's the building block the storage plugin uses internally, and it remains supported for ad-hoc observation:

```go
s := session.New(
    session.WithModel(model),
    session.WithMessageSink(func(m wingmodels.Message) {
        log.Printf("turn: %s (%d parts)", m.Role, len(m.Content))
    }),
)
```

For persistence, prefer the storage plugin: it bundles load and save together, so you can't accidentally wire one without the other. `WithMessageSink` is appropriate when you only need observation (logging, metrics, UI fanout) and not load-side rehydration.

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

The migration in `storage/migrations/0001_init.sql` defines:

- `agents` — agent records, with `output_schema_json` as a separate column
- `sessions` — session metadata
- `messages` — one row per message, ordered by `(session_id, idx)`
- `parts` — one row per part, ordered by `(message_id, idx)`, with the discriminator `type` and the JSON body
- `auth` — provider credentials keyed by provider id

Parts are stored individually (not as JSON arrays inside messages) so partial reads and selective updates are straightforward, and so the same per-part `type` discriminator works for storage and the SSE wire.

## Opening a store

```go
import "github.com/chaserensberger/wingman/storage"

store, err := storage.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

`storage.DefaultDBPath()` returns the server's default location (`~/.local/share/wingman/wingman.db`).

## Plugin parts in storage

Custom Part types ([Parts](../wingmodels/parts)) round-trip losslessly. Even when the originating plugin is uninstalled, the parts come back as `OpaquePart` values preserving the original bytes — UIs may render a placeholder or skip them, and re-marshaling reproduces the original payload.

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

The migration in `storage/migrations/0001_init.sql` defines:

- `agents` — agent records, with `output_schema_json` as a separate column
- `sessions` — session metadata
- `messages` — one row per message, ordered by `(session_id, idx)`
- `parts` — one row per part, ordered by `(message_id, idx)`, with the discriminator `type` and the JSON body
- `auth` — provider credentials keyed by provider id

Parts are stored individually (not as JSON arrays inside messages) so partial reads and selective updates are straightforward, and so the same per-part `type` discriminator works for storage and the SSE wire.

## Opening a store

```go
import "github.com/chaserensberger/wingman/storage"

store, err := storage.NewSQLiteStore("/path/to/wingman.db")
if err != nil {
    log.Fatal(err)
}
defer store.Close()
```

`storage.DefaultDBPath()` returns the server's default location (`~/.local/share/wingman/wingman.db`).

## Plugin parts in storage

Custom Part types ([Parts](../wingmodels/parts)) round-trip losslessly. Even when the originating plugin is uninstalled, the parts come back as `OpaquePart` values preserving the original bytes — UIs may render a placeholder or skip them, and re-marshaling reproduces the original payload.
