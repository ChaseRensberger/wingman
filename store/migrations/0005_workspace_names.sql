-- 0005_workspace_names.sql: workspace names are unique per client.

CREATE UNIQUE INDEX IF NOT EXISTS idx_workspaces_client_name_nocase
ON workspaces(COALESCE(client_id, ''), name COLLATE NOCASE);
