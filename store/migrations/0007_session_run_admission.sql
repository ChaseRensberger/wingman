ALTER TABLE session_runs RENAME TO session_runs_legacy;

CREATE TABLE session_runs (
    id                  TEXT PRIMARY KEY,
    session_id          TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    request_id          TEXT NOT NULL DEFAULT '',
    request_hash        TEXT NOT NULL DEFAULT '',
    admitted_version    INTEGER NOT NULL DEFAULT 0,
    work_dir            TEXT,
    workspace_id        TEXT,
    client_id           TEXT,
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

INSERT INTO session_runs (
    id, session_id, request_id, request_hash, admitted_version,
    work_dir, workspace_id, client_id, sequence, status, message, agent_json,
    output_schema_json, error_message, created_at, started_at, completed_at, updated_at
)
SELECT
    r.id, r.session_id, '', '', 0,
    s.work_dir, s.workspace_id, s.client_id, r.sequence, r.status, r.message, r.agent_json,
    r.output_schema_json, r.error_message, r.created_at, r.started_at, r.completed_at, r.updated_at
FROM session_runs_legacy r
JOIN sessions s ON s.id = r.session_id;

WITH admitted AS (
    SELECT
        r.*,
        COALESCE((
            SELECT MAX(e.version)
            FROM aggregate_events e
            WHERE e.aggregate_type = 'session' AND e.aggregate_id = r.session_id
        ), 0) + ROW_NUMBER() OVER (PARTITION BY r.session_id ORDER BY r.sequence) AS version
    FROM session_runs r
)
INSERT INTO aggregate_events (
    id, aggregate_type, aggregate_id, version, event_type, schema_version,
    payload_json, created_at, client_id, run_id
)
SELECT
    'evt_' || substr(lower(hex(randomblob(14))), 1, 27),
    'session', session_id, version, 'session.run.admitted', 1,
    json_object(
        'run', json_object(
            'id', id, 'session_id', session_id, 'request_id', request_id,
            'request_hash', request_hash, 'admitted_version', version,
            'work_dir', COALESCE(work_dir, ''), 'workspace_id', COALESCE(workspace_id, ''),
            'client_id', COALESCE(client_id, ''), 'sequence', sequence, 'status', status,
			'message', message, 'agent', json(agent_json), 'output_schema_json', COALESCE(output_schema_json, ''), 'error_message', COALESCE(error_message, ''),
			'created_at', created_at, 'started_at', started_at,
			'completed_at', completed_at, 'updated_at', updated_at
		)
    ),
    created_at, client_id, id
FROM admitted;

UPDATE session_runs
SET admitted_version = COALESCE((
    SELECT e.version
    FROM aggregate_events e
    WHERE e.aggregate_type = 'session'
      AND e.aggregate_id = session_runs.session_id
      AND e.run_id = session_runs.id
      AND e.event_type = 'session.run.admitted'
), 0);

UPDATE sessions
SET aggregate_version = COALESCE((
    SELECT MAX(version)
    FROM aggregate_events e
    WHERE e.aggregate_type = 'session' AND e.aggregate_id = sessions.id
), 0);

DROP TABLE session_runs_legacy;

CREATE INDEX idx_session_runs_session_status_sequence
ON session_runs(session_id, status, sequence);

CREATE UNIQUE INDEX idx_session_runs_session_request_id
ON session_runs(session_id, request_id)
WHERE request_id <> '';
