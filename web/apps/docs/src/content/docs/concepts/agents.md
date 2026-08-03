---
title: "Agents"
group: "Core"
order: 100
---

# Agents

An agent is a reusable definition for a [session](/concepts/sessions) turn. It
holds the instructions, allowed tools, model default, and optional output
schema for that turn.

This command uses `WINGMAN_TOKEN` from [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Builder",
        "instructions": "You are a pragmatic software engineer. Make small, correct changes.",
        "tools": ["read", "grep", "glob", "write", "edit", "bash"],
        "model_ref": "anthropic/claude-sonnet-5",
        "options": {
          "max_tokens": 4096
        }
      }'
```

To use an agent, see [sessions](/concepts/sessions).
