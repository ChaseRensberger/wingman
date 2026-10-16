---
title: "Agents"
---

# Agents

An agent defines instructions, allowed tools, and a default model. It can also define permissions and a schema, which describes the required output shape.
An agent does not store a conversation. Sessions store conversations and can use a different agent for each message.

Create an agent on the local managed service:

```bash
wingman api createAgent -d '{
        "name": "Builder",
        "instructions": "You are a pragmatic software engineer. Make small, correct changes.",
        "tools": ["read", "grep", "glob", "write", "edit", "bash"],
        "model_ref": "anthropic/claude-sonnet-5"
      }'
```

The example uses file tools. Create a session with a working directory before you send a message.
See [Sessions](/concepts/sessions) for the commands.

The API saves the `options` object, but the server does not apply its `max_tokens` or `temperature` values to model requests.
Use a supported [model variant](/configure/models#model-variants) to select its named model configuration.

For global and project `AGENTS.md` instructions, see
[Project Instructions](/configure/project-instructions).
