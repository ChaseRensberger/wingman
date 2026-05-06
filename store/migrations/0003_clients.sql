-- 0003_clients.sql: add clients table for namespacing sessions.
--
-- Clients are first-party or third-party consumers of the API. Sessions
-- can be optionally scoped to a client so that listings are namespaced.
-- Open registration: no secrets required.

CREATE TABLE IF NOT EXISTS clients (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL
);

ALTER TABLE sessions ADD COLUMN client_id TEXT REFERENCES clients(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_sessions_client_id ON sessions(client_id);
