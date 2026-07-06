-- 0002_session_events.sql: durable session event log for SSE replay.

CREATE TABLE IF NOT EXISTS session_events (
    id           TEXT PRIMARY KEY,
    session_id   TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL,
    type         TEXT NOT NULL,
    version      INTEGER NOT NULL DEFAULT 1,
    data_json    TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    UNIQUE(session_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_session_events_session_seq ON session_events(session_id, seq);
