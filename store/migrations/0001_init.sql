-- 0001_init.sql: current schema for Wingman's local store.
--
-- Design notes:
--   * IDs are KSUID strings prefixed with a typed tag (agt_, ses_, msg_,
--     prt_, cli_, wsp_) for human readability in logs. The tag is part of the
--     primary key; we never strip it.
--   * Sessions carry metadata (title, work_dir, workspace_id, client_id) but no
--     inline history. Messages are rows in the `messages` table; their content
--     parts are rows in `parts`.
--   * Workspaces are optional saved contexts. An empty path means sessions
--     created in that workspace do not start with a working directory.
--   * `idx` columns give per-parent ordering; (parent_id, idx) is unique. Use
--     idx instead of relying on insertion order for queries.
--   * ON DELETE CASCADE: dropping a session drops its messages drops its parts.
--     Dropping an agent does NOT drop sessions; sessions reference agents only
--     at runtime via the API. Dropping a client sets client-owned sessions and
--     workspaces to NULL; dropping a workspace clears sessions.workspace_id.
--   * `parts.payload_json` is opaque to the store — kind/payload interpretation
--     belongs to the agent layer.
--   * The `messages` table holds plain message rows; compaction is a plugin
--     concern that uses the opaque `parts.kind`/`payload_json` extension point,
--     not a core column.

CREATE TABLE IF NOT EXISTS agents (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    instructions       TEXT,
    tools_json         TEXT,
    model_ref          TEXT,
    options_json       TEXT,
    output_schema_json TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS clients (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

INSERT OR IGNORE INTO clients (id, name, created_at)
VALUES ('cli_wingman', 'Wingman', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_name_nocase ON clients(name COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS workspaces (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL DEFAULT '',
    client_id  TEXT REFERENCES clients(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workspaces_client_id ON workspaces(client_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_client_name_nocase
ON workspaces(COALESCE(client_id, ''), name COLLATE NOCASE);

CREATE TABLE IF NOT EXISTS sessions (
    id           TEXT PRIMARY KEY,
    title        TEXT NOT NULL DEFAULT '',
    work_dir     TEXT,
    workspace_id TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
    client_id    TEXT REFERENCES clients(id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_client_id ON sessions(client_id);
CREATE INDEX IF NOT EXISTS idx_sessions_workspace_id ON sessions(workspace_id);

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

CREATE TABLE IF NOT EXISTS model_calls (
    id                     TEXT PRIMARY KEY,
    session_id             TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    assistant_message_id   TEXT REFERENCES messages(id) ON DELETE SET NULL,
    step                   INTEGER NOT NULL,
    attempt                INTEGER NOT NULL DEFAULT 1,
    status                 TEXT NOT NULL,
    agent_id               TEXT,
    model_ref              TEXT,
    provider               TEXT,
    api                    TEXT,
    model_id               TEXT,
    finish_reason          TEXT,
    stop_reason            TEXT,
    error_type             TEXT,
    error_message          TEXT,
    input_tokens           INTEGER NOT NULL DEFAULT 0,
    output_tokens          INTEGER NOT NULL DEFAULT 0,
    reasoning_tokens       INTEGER NOT NULL DEFAULT 0,
    cached_input_tokens    INTEGER NOT NULL DEFAULT 0,
    cache_write_tokens     INTEGER NOT NULL DEFAULT 0,
    total_tokens           INTEGER NOT NULL DEFAULT 0,
    context_tokens         INTEGER NOT NULL DEFAULT 0,
    context_window         INTEGER NOT NULL DEFAULT 0,
    context_percent        REAL,
    cost                   REAL NOT NULL DEFAULT 0,
    structured_output_json TEXT,
    metadata_json          TEXT,
    started_at             TEXT NOT NULL,
    completed_at           TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    UNIQUE(session_id, step, attempt)
);

CREATE INDEX IF NOT EXISTS idx_model_calls_session_step ON model_calls(session_id, step DESC, attempt DESC);
CREATE INDEX IF NOT EXISTS idx_model_calls_assistant_message ON model_calls(assistant_message_id);
CREATE INDEX IF NOT EXISTS idx_model_calls_session_status ON model_calls(session_id, status);

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
