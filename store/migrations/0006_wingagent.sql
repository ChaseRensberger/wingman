-- Rename the general-purpose starter without deleting user-created agents.
UPDATE agents
SET name = 'WingAgent',
    tools_json = '["read","grep","glob","write","edit","bash","webfetch","websearch","question"]',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE name = 'Friday';
