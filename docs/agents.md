---
title: "Agents"
group: "Primitives"
order: 11
draft: false
---
# Agents

The word agent gets thrown around a lot these days and it is possible this gets renamed at some point but at least in Wingman, an **Agent** is stateless template that defines *how* to process some unit of work.

## Example

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
