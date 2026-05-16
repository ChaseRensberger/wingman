-- 0002_agent_model_ref.sql: canonicalize agents on model_ref.

CREATE TABLE agents_new (
    id                 TEXT PRIMARY KEY,
    name               TEXT NOT NULL,
    instructions       TEXT,
    tools_json         TEXT,
    model_ref          TEXT,
    options_json       TEXT,
    output_schema_json TEXT,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);

INSERT INTO agents_new (id, name, instructions, tools_json, model_ref, options_json, output_schema_json, created_at, updated_at)
SELECT id, name, instructions, tools_json,
       CASE
         WHEN provider IS NOT NULL AND provider != '' AND model IS NOT NULL AND model != '' THEN provider || '/' || model
         ELSE NULL
       END,
       options_json, output_schema_json, created_at, updated_at
FROM agents;

DROP TABLE agents;
ALTER TABLE agents_new RENAME TO agents;
