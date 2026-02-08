---
title: "Agents"
group: "Primitives"
order: 101
draft: false
---
# Agents

The word agent gets thrown around a lot these days and it is possible this gets renamed at some point but at least in Wingman, an **Agent** is stateless template that defines *how* to process some unit of work.

## Server Routes

```
POST   /agents      # Create agent
GET    /agents      # List agents
GET    /agents/{id} # Get agent
PUT    /agents/{id} # Update agent
DELETE /agents/{id} # Delete agent
```

## Usage

### Server

```bash
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodeAssistant",
    "instructions": "You are a helpful coding assistant.",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "max_tokens": 4096,
    "max_steps": 50
  }'
```

### SDK

```go
a := agent.New("AgentName",
    agent.WithInstructions("System prompt"),
    agent.WithMaxTokens(4096),
    agent.WithTemperature(0.7),
    agent.WithMaxSteps(50),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool()),
    agent.WithOutputSchema(map[string]any{"type": "object", ...}),
)
```



