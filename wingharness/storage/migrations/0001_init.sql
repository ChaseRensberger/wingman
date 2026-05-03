-- 0001_init.sql: initial schema for wingharness storage.
--
-- Design notes:
--   * IDs are KSUID strings prefixed with a typed tag (agt_, ses_, msg_,
--     prt_, tlu_) for human readability in logs. The tag is part of the
--     primary key; we never strip it.
--   * Sessions no longer carry an inline history blob. Messages are rows
--     in the `messages` table; their content parts are rows in `parts`.
--     This lets us paginate, append-only stream, and (in a later tier)
--     swap a single part with a compaction marker without rewriting the
--     whole conversation.
--   * `idx` columns give per-parent ordering; (parent_id, idx) is unique.
--     Use idx instead of relying on insertion order for queries.
--   * ON DELETE CASCADE: dropping a session drops its messages drops its
--     parts. Dropping an agent does NOT drop sessions; sessions reference
--     agents only at runtime via the API.
--   * payload_json on parts stores the full MarshalPart output (including
--     the "type" discriminator). The `kind` column is a denormalized copy
--     for cheap indexing/inspection.

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

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    work_dir   TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    idx        INTEGER NOT NULL,
    role       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(session_id, idx)
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, idx);

CREATE TABLE IF NOT EXISTS parts (
    id           TEXT PRIMARY KEY,
    message_id   TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    idx          INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    UNIQUE(message_id, idx)
);

CREATE INDEX IF NOT EXISTS idx_parts_message ON parts(message_id, idx);

CREATE TABLE IF NOT EXISTS auth (
    id            INTEGER PRIMARY KEY CHECK (id = 1),
    providers_json TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
