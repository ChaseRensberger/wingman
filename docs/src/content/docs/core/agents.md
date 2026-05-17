---
title: "Agents"
group: "Core"
order: 100
---

# Agents

An agent is a reusable definition for how a tasks should be completed (in the contect of a [session](/core/sessions)). It describes the runtime configuration for a turn.

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Builder",
        "instructions": "You are a pragmatic software engineer. Make small, correct changes.",
        "tools": ["read", "grep", "glob", "write", "edit", "bash"],
        "model_ref": "anthropic/claude-sonnet-4-6",
        "options": {
          "max_tokens": 4096
        }
      }'
```

To use an agent, see [sessions](/core/sessions).
