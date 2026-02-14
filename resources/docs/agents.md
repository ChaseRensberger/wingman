---
title: "Agents"
group: "Primitives"
order: 101
draft: false
---

# Agents

An **Agent** is a stateless template that defines *how* to process a unit of work. It holds a name, instructions, [tools](/docs/tools), an optional output schema, and a [provider](/docs/providers).

## SDK Usage

```go
a := agent.New("AgentName",
    agent.WithInstructions("System prompt"),
    agent.WithProvider(p),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool()),
    agent.WithOutputSchema(map[string]any{"type": "object", ...}),
)
```

All options are optional except the name. For provider setup, see [Providers — SDK Usage](/docs/providers).

### Available Options

| Option | Description |
|--------|-------------|
| `WithID(id)` | Set a specific ID (auto-generated if omitted) |
| `WithInstructions(s)` | System prompt for the agent |
| `WithProvider(p)` | The [provider](/docs/providers) to use for inference |
| `WithTools(t...)` | [Tools](/docs/tools) the agent can invoke |
| `WithOutputSchema(s)` | JSON schema for structured output |

## Server Usage

```
POST   /agents      # Create agent
GET    /agents      # List agents
GET    /agents/{id} # Get agent
PUT    /agents/{id} # Update agent
DELETE /agents/{id} # Delete agent
```

```bash
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodeAssistant",
    "instructions": "You are a helpful coding assistant.",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": {
      "id": "anthropic",
      "model": "claude-sonnet-4-5",
      "max_tokens": 4096
    }
  }'
```

The `provider.id` field determines which provider to use. The server looks up credentials from the auth store (see [Providers — Auth Management](/docs/providers)) and constructs the provider at inference time.
