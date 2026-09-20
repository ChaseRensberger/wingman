CREATE TABLE triggers (
    id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL REFERENCES clients(id),
    config_json TEXT NOT NULL,
    version INTEGER NOT NULL,
    enabled INTEGER NOT NULL,
    next_fire_at INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX triggers_due ON triggers(next_fire_at) WHERE enabled = 1;
CREATE INDEX triggers_client ON triggers(client_id);

CREATE TABLE trigger_occurrences (
    id TEXT PRIMARY KEY,
    trigger_id TEXT NOT NULL REFERENCES triggers(id) ON DELETE CASCADE,
    trigger_name TEXT NOT NULL,
    source TEXT NOT NULL,
    request_id TEXT NOT NULL,
    scheduled_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    status TEXT NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    session_id TEXT NOT NULL DEFAULT '',
    run_id TEXT NOT NULL DEFAULT '',
    UNIQUE(trigger_id, source, request_id)
);
CREATE INDEX trigger_occurrences_history ON trigger_occurrences(trigger_id, created_at DESC);
CREATE INDEX trigger_occurrences_run ON trigger_occurrences(run_id);
