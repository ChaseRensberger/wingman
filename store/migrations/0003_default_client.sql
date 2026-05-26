-- 0003_default_client.sql: add the built-in default client and unique names.

INSERT OR IGNORE INTO clients (id, name, created_at)
VALUES ('cli_wingman', 'Wingman', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'));

UPDATE clients
SET name = name || ' (' || id || ')'
WHERE lower(name) = 'wingman' AND id != 'cli_wingman';

UPDATE clients SET name = 'Wingman' WHERE id = 'cli_wingman';

WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY lower(name)
               ORDER BY CASE WHEN id = 'cli_wingman' THEN 0 ELSE 1 END, created_at, id
           ) AS rn
    FROM clients
)
UPDATE clients
SET name = name || ' (' || id || ')'
WHERE id IN (SELECT id FROM ranked WHERE rn > 1);

UPDATE sessions SET client_id = 'cli_wingman' WHERE client_id IS NULL;
UPDATE bases SET client_id = 'cli_wingman' WHERE client_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_name_nocase ON clients(name COLLATE NOCASE);
