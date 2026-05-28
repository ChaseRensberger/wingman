-- 0004_workspaces.sql: breaking rename from Bases to Workspaces.

ALTER TABLE bases RENAME TO workspaces;
ALTER TABLE sessions RENAME COLUMN base_id TO workspace_id;

DROP INDEX IF EXISTS idx_bases_client_id;
DROP INDEX IF EXISTS idx_sessions_base_id;

CREATE INDEX IF NOT EXISTS idx_workspaces_client_id ON workspaces(client_id);
CREATE INDEX IF NOT EXISTS idx_sessions_workspace_id ON sessions(workspace_id);
