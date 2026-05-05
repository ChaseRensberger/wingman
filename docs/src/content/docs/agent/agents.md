---
title: "Agents (server)"
group: "Concepts"
draft: false
order: 109
---

# Agents (server)

An *agent* in Wingman v0.1 is a server-side concept: a stored configuration record that the HTTP server uses to build a session at request time. The SDK has no `agent` package — when you embed Wingman in Go, you construct sessions directly with `session.New(...)` and the options you want.

## What an agent is

An agent record is a reusable bundle of:

- a name
- system instructions
- a provider id and model id
- inference options
- a list of built-in tool names
- an optional JSON Schema for structured output

Agents do not own conversation state. A session references an agent by id, and the server reconstructs a fresh `*session.Session` per message using the agent's fields plus the session's stored history.

## Routes

```text
POST   /agents
GET    /agents
GET    /agents/{id}
PUT    /agents/{id}
DELETE /agents/{id}
```

## Create an agent

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodeAssistant",
    "instructions": "You are a practical coding assistant.",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": "anthropic",
    "model": "claude-haiku-4-5",
    "options": {
      "max_tokens": 4096,
      "temperature": 0.2
    }
  }'
```

## Fields

| Field | Type | Notes |
|---|---|---|
| `name` | string | Required display name |
| `provider` | string | Provider id such as `anthropic` |
| `model` | string | Model id such as `claude-haiku-4-5` |
| `options` | object | Provider-specific inference settings (see [Providers](../models/providers)) |
| `instructions` | string | System prompt sent on every inference call |
| `tools` | string[] | Built-in tool names ([Tools](./tools)) |
| `output_schema` | object | Optional JSON Schema for structured output |

Tool names are resolved against the server's built-in tool registry. Custom Go tools are an SDK concern; the server only knows the built-ins.

## Update behavior

`PUT /agents/{id}` accepts the same fields as create. The server updates each provided field; omitted fields are left unchanged.

```bash
curl -sS -X PUT http://localhost:2323/agents/agt_... \
  -H "Content-Type: application/json" \
  -d '{
    "instructions": "You are a fast, practical coding assistant.",
    "tools": ["bash", "read", "edit", "glob", "grep"]
  }'
```

## Use in a session

A session by itself only carries `title`, `work_dir`, and history. The agent id is supplied per message:

```bash
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "agt_...",
    "message": "Explain this codebase."
  }'
```

The server loads the agent record, builds the provider via the registry (injecting credentials from the auth store), constructs a `*session.Session` with the agent's instructions and tool set, replays stored history into it, wires `WithMessageSink` to incremental storage, and runs the loop. See [Server](../server) for the full request flow.

## SDK equivalent

There is no `agent.New` in the SDK. The same agent expressed in Go:

```go
import (
    "github.com/chaserensberger/wingman/agent/session"
    "github.com/chaserensberger/wingman/tool"
    "github.com/chaserensberger/wingman/models/providers/anthropic"
)

p, _ := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-haiku-4-5",
        "max_tokens": 4096,
        "temperature": 0.2,
    },
})

s := session.New(
    session.WithModel(p),
    session.WithSystem("You are a practical coding assistant."),
    session.WithTools(
        tool.NewBashTool(),
        tool.NewReadTool(),
        tool.NewWriteTool(),
        tool.NewEditTool(),
        tool.NewGlobTool(),
        tool.NewGrepTool(),
    ),
    session.WithWorkDir(workDir),
)
```

If you want to reuse the configuration across sessions in your own program, hold the model and tool slice as values and pass them to each `session.New` call.
