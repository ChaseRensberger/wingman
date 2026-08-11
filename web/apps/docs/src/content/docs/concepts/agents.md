---
title: "Agents"
group: "Core"
order: 100
---

# Agents

An agent is a reusable definition for a [session](/concepts/sessions) turn.
It contains the instructions, allowed tools, default model, and optional output
schema for that turn.

This command uses `WINGMAN_DAEMON_PASSWORD` with HTTP Basic authentication. See [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X POST http://localhost:2323/agents \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
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

For agent use, see [sessions](/concepts/sessions).
