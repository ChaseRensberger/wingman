-- 0001_init.sql: v1 schema for agent store.
--
-- Design notes:
--   * IDs are KSUID strings prefixed with a typed tag (agt_, ses_, msg_,
--     prt_, cli_) for human readability in logs. The tag is part of the
--     primary key; we never strip it.
--   * Sessions carry metadata (title, work_dir, client_id) but no inline
--     history. Messages are rows in the `messages` table; their content
--     parts are rows in `parts`.
--   * `idx` columns give per-parent ordering; (parent_id, idx) is unique.
--     Use idx instead of relying on insertion order for queries.
--   * ON DELETE CASCADE: dropping a session drops its messages drops its
--     parts. Dropping an agent does NOT drop sessions; sessions reference
--     agents only at runtime via the API. Dropping a client sets its
--     sessions' client_id to NULL.
--   * `parts.payload_json` is opaque to the store — kind/payload
--     interpretation belongs to the agent layer.
--   * The `messages` table holds plain message rows; compaction is a
--     plugin concern that uses the opaque `parts.kind`/`payload_json`
--     extension point, not a core column.

CREATE TABLE IF NOT EXISTS agents (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    instructions  TEXT,
    tools_json    TEXT,
    provider      TEXT,
    model         TEXT,
    options_json  TEXT,
    output_schema_json TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS clients (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL DEFAULT '',
    work_dir   TEXT,
    client_id  TEXT REFERENCES clients(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_client_id ON sessions(client_id);

CREATE TABLE IF NOT EXISTS messages (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    idx           INTEGER NOT NULL,
    role          TEXT NOT NULL,
    metadata_json TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE(session_id, idx)
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, idx);

CREATE TABLE IF NOT EXISTS parts (
    id           TEXT PRIMARY KEY,
    message_id   TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    idx          INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE(message_id, idx)
);

CREATE INDEX IF NOT EXISTS idx_parts_message ON parts(message_id, idx);

CREATE TABLE IF NOT EXISTS auth (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    providers_json TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
