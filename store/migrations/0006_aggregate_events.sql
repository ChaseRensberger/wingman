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
    UNIQUE (aggregate_type, aggregate_id, version)
);

CREATE INDEX idx_aggregate_events_stream
    ON aggregate_events(aggregate_type, aggregate_id, version);

ALTER TABLE sessions ADD COLUMN aggregate_version INTEGER NOT NULL DEFAULT 0;

INSERT INTO aggregate_events (
    id,
    aggregate_type,
    aggregate_id,
    version,
    event_type,
    schema_version,
    payload_json,
    created_at,
    client_id
)
SELECT
    'evt_' || substr(lower(hex(randomblob(14))), 1, 27),
    'session',
    id,
    1,
    'session.created',
    1,
    json_object(
        'id', id,
        'title', title,
        'work_dir', COALESCE(work_dir, ''),
        'workspace_id', COALESCE(workspace_id, ''),
        'client_id', COALESCE(client_id, ''),
        'created_at', created_at,
        'updated_at', updated_at
    ),
    created_at,
    client_id
FROM sessions;

UPDATE sessions SET aggregate_version = 1;
