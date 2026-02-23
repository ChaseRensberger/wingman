---
title: "Agents"
group: "Primitives"
order: 101
draft: false
---
# Agents

The word agent gets thrown around a lot these days and it is possible this gets renamed at some point but at least in Wingman, an **Agent** is stateless template that defines *how* to process some unit of work.

## SDK

```go
p := anthropic.New(anthropic.Config{
    Model:     "claude-sonnet-4-5",
    MaxTokens: 4096,
})

a := agent.New("AgentName",
    agent.WithInstructions("System prompt"),
    agent.WithProvider(p),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool()),
    agent.WithOutputSchema(map[string]any{"type": "object", ...}),
)
```

## Server

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
    "model": "anthropic/claude-sonnet-4-5",
    "options": {
      "max_tokens": 4096
    }
  }'
```
