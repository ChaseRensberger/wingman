CREATE TABLE IF NOT EXISTS session_runs (
    id                  TEXT PRIMARY KEY,
    session_id          TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    sequence            INTEGER NOT NULL,
    status              TEXT NOT NULL,
    message             TEXT NOT NULL,
    agent_json          TEXT NOT NULL,
    output_schema_json  TEXT,
    error_message       TEXT,
    created_at          TEXT NOT NULL,
    started_at          TEXT,
    completed_at        TEXT,
    updated_at          TEXT NOT NULL,
    UNIQUE(session_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_session_runs_session_status_sequence
ON session_runs(session_id, status, sequence);
