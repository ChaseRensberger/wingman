---
title: "Agents"
group: "Concepts"
draft: false
order: 101
---

# Agents

An agent is a reusable configuration bundle that defines how Wingman should perform a unit of work. Agents are intentionally stateless: they do not own conversation history and they do not execute on their own.

An agent typically includes:

- a name
- system instructions
- a provider and model
- a set of tools
- an optional output schema

Sessions and fleets execute agents; the agent itself is the template.

## SDK

```go
p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})

a := agent.New("CodeAssistant",
    agent.WithInstructions("You are a practical coding assistant."),
    agent.WithProvider(p),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool(), tool.NewEditTool()),
    agent.WithOutputSchema(map[string]any{"type": "object"}),
)
```

## Server

The server persists agent definitions in SQLite and reconstructs live agent instances when they are used.

```text
POST   /agents
GET    /agents
GET    /agents/{id}
PUT    /agents/{id}
DELETE /agents/{id}
```

### Create an agent

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodeAssistant",
    "instructions": "You are a practical coding assistant.",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": "anthropic",
    "model": "claude-sonnet-4-5",
    "options": {
      "max_tokens": 4096,
      "temperature": 0.2
    }
  }'
```

## Agent fields

| Field | Type | Notes |
|---|---|---|
| `name` | string | Required display name |
| `provider` | string | Provider ID such as `anthropic` |
| `model` | string | Model ID such as `claude-sonnet-4-5` |
| `options` | object | Provider-specific inference settings |
| `instructions` | string | System prompt sent on each inference call |
| `tools` | string[] | Built-in server tool names |
| `output_schema` | object | Optional JSON Schema for structured output |

## Update behavior

`PUT /agents/{id}` accepts the same fields as create. Omitted fields are left unchanged.

```bash
curl -sS -X PUT http://localhost:2323/agents/01ABC... \
  -H "Content-Type: application/json" \
  -d '{
    "instructions": "You are a fast, practical coding assistant.",
    "tools": ["bash", "read", "edit", "glob", "grep"]
  }'
```
