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

## Coding context

The `wingcode` plugin adds coding guidance and Git repository information. It refreshes the repository root before each model call.
It works with ordinary repositories and linked Git worktrees. It does not grant tools or permissions.

The standard server enables `wingcode` for every agent, including existing agents.
An agent's name or tool selection does not change which runtime plugins apply.
Open Tools or the Console Inspector to see the server's registered plugins and their enabled or disabled state.

## Shared context

Server sessions supply the date, execution directory, platform, temporary directory, and current provider and model IDs.
They also advertise available skills and configured reference directories.
MCP server instructions appear when the agent selects at least one tool from that server that permissions do not fully deny.
Tools that permissions fully deny do not appear in the model's tool list.
Resource-specific permissions still apply when tools execute.
The Inspector records this context with each model input.
