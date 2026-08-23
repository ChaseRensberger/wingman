---
title: "Agents"
group: "Core"
order: 100
---

# Agents

An agent is a reusable definition for a [session](/concepts/sessions) turn. It contains instructions, allowed tools, a default model, and an optional output schema.

This command finds and authenticates with the managed daemon.

```bash
wingman api createAgent -d '{
        "name": "Builder",
        "instructions": "You are a pragmatic software engineer. Make small, correct changes.",
        "tools": ["read", "grep", "glob", "write", "edit", "bash"],
        "model_ref": "anthropic/claude-sonnet-5",
        "options": {
          "max_tokens": 4096
        }
      }'
```

For agent use, see [Sessions](/concepts/sessions).

For global and project `AGENTS.md` instructions, see
[Project Instructions](/configure/project-instructions).
