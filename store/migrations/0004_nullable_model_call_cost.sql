-- Preserve recorded costs while allowing calls without usage or pricing data
-- to represent cost as unavailable instead of zero.
ALTER TABLE model_calls RENAME TO model_calls_old;

CREATE TABLE model_calls (
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
    cost                   REAL,
    structured_output_json TEXT,
    metadata_json          TEXT,
    started_at             TEXT NOT NULL,
    completed_at           TEXT,
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL,
    UNIQUE(session_id, step, attempt)
);

INSERT INTO model_calls (
    id, session_id, assistant_message_id, step, attempt, status,
    agent_id, model_ref, provider, api, model_id,
    finish_reason, stop_reason, error_type, error_message,
    input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens,
    context_tokens, context_window, context_percent, cost,
    structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at
)
SELECT
    id, session_id, assistant_message_id, step, attempt, status,
    agent_id, model_ref, provider, api, model_id,
    finish_reason, stop_reason, error_type, error_message,
    input_tokens, output_tokens, reasoning_tokens, cached_input_tokens, cache_write_tokens, total_tokens,
    context_tokens, context_window, context_percent, cost,
    structured_output_json, metadata_json, started_at, completed_at, created_at, updated_at
FROM model_calls_old;

DROP TABLE model_calls_old;

CREATE INDEX idx_model_calls_session_step ON model_calls(session_id, step DESC, attempt DESC);
CREATE INDEX idx_model_calls_assistant_message ON model_calls(assistant_message_id);
CREATE INDEX idx_model_calls_session_status ON model_calls(session_id, status);
