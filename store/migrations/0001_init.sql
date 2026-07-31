-- 0001_init.sql: canonical pre-beta schema for Wingman's local store.

CREATE TABLE agents (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    instructions       TEXT,
    tools_json         TEXT,
    permissions_json   TEXT,
    model_ref          TEXT,
    options_json       TEXT,
    output_schema_json TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

CREATE TABLE clients (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL
);

INSERT INTO clients (id, name, created_at)
VALUES ('cli_wingman', 'Wingman', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));

CREATE UNIQUE INDEX idx_clients_name_nocase ON clients(name COLLATE NOCASE);

CREATE TABLE workspaces (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    path       TEXT NOT NULL DEFAULT '',
    client_id  TEXT REFERENCES clients(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX idx_workspaces_client_id ON workspaces(client_id);
CREATE UNIQUE INDEX idx_workspaces_client_name_nocase
ON workspaces(COALESCE(client_id, ''), name COLLATE NOCASE);

CREATE TABLE sessions (
    id                TEXT PRIMARY KEY,
    title             TEXT NOT NULL DEFAULT '',
    work_dir          TEXT,
    workspace_id      TEXT REFERENCES workspaces(id) ON DELETE SET NULL,
    client_id         TEXT REFERENCES clients(id) ON DELETE SET NULL,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL,
    aggregate_version INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_sessions_client_id ON sessions(client_id);
CREATE INDEX idx_sessions_workspace_id ON sessions(workspace_id);

CREATE TABLE messages (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
	    run_id        TEXT REFERENCES session_runs(id) ON DELETE CASCADE,
    idx           INTEGER NOT NULL,
    role          TEXT NOT NULL,
    revision      INTEGER NOT NULL DEFAULT 1,
    state         TEXT NOT NULL DEFAULT 'completed',
    metadata_json TEXT,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    UNIQUE(session_id, idx)
);

CREATE INDEX idx_messages_session ON messages(session_id, idx);

CREATE TABLE parts (
    id           TEXT PRIMARY KEY,
    message_id   TEXT NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    idx          INTEGER NOT NULL,
    kind         TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE(message_id, idx)
);

CREATE INDEX idx_parts_message ON parts(message_id, idx);

CREATE TABLE session_runs (
    id                  TEXT PRIMARY KEY,
    session_id          TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    request_id          TEXT NOT NULL DEFAULT '',
    request_hash        TEXT NOT NULL DEFAULT '',
    admitted_version    INTEGER NOT NULL,
    work_dir            TEXT,
    workspace_id        TEXT,
    client_id           TEXT,
    sequence            INTEGER NOT NULL,
    status              TEXT NOT NULL,
    message             TEXT NOT NULL,
    agent_json          TEXT NOT NULL,
    output_schema_json  TEXT,
    error_message       TEXT,
	    error_type          TEXT,
    created_at          TEXT NOT NULL,
    started_at          TEXT,
    completed_at        TEXT,
    updated_at          TEXT NOT NULL,
    UNIQUE(session_id, sequence)
);

CREATE INDEX idx_session_runs_session_status_sequence
ON session_runs(session_id, status, sequence);

CREATE UNIQUE INDEX idx_session_runs_one_running_per_session
ON session_runs(session_id) WHERE status = 'running';

CREATE UNIQUE INDEX idx_session_runs_session_request_id
ON session_runs(session_id, request_id)
WHERE request_id <> '';

CREATE TABLE model_calls (
    id                     TEXT PRIMARY KEY,
    session_id             TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    run_id                 TEXT REFERENCES session_runs(id) ON DELETE CASCADE,
    assistant_message_id   TEXT REFERENCES messages(id) ON DELETE SET NULL,
    step                   INTEGER NOT NULL,
    attempt                INTEGER NOT NULL DEFAULT 1,
    status                 TEXT NOT NULL,
    agent_id               TEXT,
    model_ref              TEXT,
    provider               TEXT,
    provider_request_id    TEXT,
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
    cost                   REAL,
    structured_output_json TEXT,
    metadata_json          TEXT,
    started_at             TEXT NOT NULL,
    completed_at           TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    UNIQUE(run_id, step, attempt)
);

CREATE INDEX idx_model_calls_session_started_at ON model_calls(session_id, started_at, id);
CREATE INDEX idx_model_calls_assistant_message ON model_calls(assistant_message_id);
CREATE INDEX idx_model_calls_session_status ON model_calls(session_id, status);

CREATE TABLE tool_uses (
    id                   TEXT PRIMARY KEY,
    session_id           TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    run_id               TEXT REFERENCES session_runs(id) ON DELETE CASCADE,
    model_call_id        TEXT REFERENCES model_calls(id) ON DELETE SET NULL,
    assistant_message_id TEXT REFERENCES messages(id) ON DELETE SET NULL,
    part_id              TEXT REFERENCES parts(id) ON DELETE SET NULL,
    step                 INTEGER NOT NULL,
    ordinal              INTEGER NOT NULL,
    call_id              TEXT NOT NULL DEFAULT '',
    name                 TEXT NOT NULL,
    status               TEXT NOT NULL,
    input_json           TEXT,
    output               TEXT,
	structured_json      TEXT,
    metadata_json        TEXT,
    error_type           TEXT,
    error_message        TEXT,
    proposed_at          TEXT NOT NULL,
    authorized_at        TEXT,
    started_at           TEXT,
    completed_at         TEXT,
    created_at           TEXT NOT NULL,
    updated_at           TEXT NOT NULL
);

CREATE INDEX idx_tool_uses_session_proposed_at ON tool_uses(session_id, proposed_at, step, ordinal, id);
CREATE UNIQUE INDEX idx_tool_uses_run_step_ordinal ON tool_uses(run_id, step, ordinal)
WHERE run_id IS NOT NULL AND run_id <> '';
CREATE INDEX idx_tool_uses_status ON tool_uses(status);
CREATE INDEX idx_tool_uses_assistant_message ON tool_uses(assistant_message_id);
CREATE INDEX idx_tool_uses_part ON tool_uses(part_id);

CREATE TABLE permission_requests (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    run_id        TEXT REFERENCES session_runs(id) ON DELETE CASCADE,
    tool_use_id   TEXT REFERENCES tool_uses(id) ON DELETE CASCADE,
    call_id       TEXT NOT NULL DEFAULT '',
    action        TEXT NOT NULL,
    resources_json TEXT NOT NULL CHECK (json_valid(resources_json) AND json_type(resources_json) = 'array' AND json_array_length(resources_json) > 0),
    status        TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'timed_out', 'interrupted')),
    response      TEXT NOT NULL DEFAULT '' CHECK (response IN ('', 'once', 'always', 'reject')),
    error_type    TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    resolved_at   TEXT,
    updated_at    TEXT NOT NULL,
    CHECK (
        (status = 'pending' AND response = '') OR
        (status = 'approved' AND response IN ('once', 'always')) OR
        (status = 'rejected' AND response = 'reject') OR
        (status IN ('timed_out', 'interrupted') AND response = '')
    )
);

CREATE INDEX idx_permission_requests_session_created ON permission_requests(session_id, created_at, id);
CREATE INDEX idx_permission_requests_status ON permission_requests(status);

CREATE TABLE permission_grants (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    action     TEXT NOT NULL,
    resource   TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(session_id, action, resource)
);

CREATE INDEX idx_permission_grants_session ON permission_grants(session_id, action, resource);

CREATE TABLE session_events (
    id         TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    type       TEXT NOT NULL,
    data_json  TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(session_id, seq)
);

CREATE INDEX idx_session_events_session_seq ON session_events(session_id, seq);

CREATE TABLE aggregate_events (
    global_sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id              TEXT NOT NULL UNIQUE,
    aggregate_type  TEXT NOT NULL,
    aggregate_id    TEXT NOT NULL,
    version         INTEGER NOT NULL CHECK (version > 0),
    event_type      TEXT NOT NULL,
    schema_version  INTEGER NOT NULL CHECK (schema_version > 0),
    payload_json    TEXT NOT NULL CHECK (json_valid(payload_json)),
    created_at      TEXT NOT NULL,
    causation_id    TEXT,
    correlation_id  TEXT,
    client_id       TEXT,
    run_id          TEXT,
    UNIQUE(aggregate_type, aggregate_id, version)
);

CREATE INDEX idx_aggregate_events_stream
ON aggregate_events(aggregate_type, aggregate_id, version);

CREATE TABLE auth (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    providers_json TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
