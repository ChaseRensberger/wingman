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

### Create

Use `model` in `"provider/model"` format. Use `options` for inference configuration — it is a free-form map of keys supported by the provider.

**Common options (all providers):**

| Key | Type | Description |
|-----|------|-------------|
| `max_tokens` | number | Maximum tokens to generate |
| `temperature` | number | Sampling temperature |

**Provider-specific options:**

| Key | Provider | Description |
|-----|----------|-------------|
| `base_url` | ollama | Custom Ollama server URL (default: `http://localhost:11434`) |
| `api_key` | anthropic | Override the API key set in auth (optional) |

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

```bash
# Ollama with a custom server URL
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "LocalAgent",
    "instructions": "Be helpful",
    "model": "ollama/llama3.2",
    "options": {
      "base_url": "http://my-ollama-host:11434"
    }
  }'
```

Specify built-in tools by name. See [Tools](./tools) for the full list.

### Update

`PUT /agents/{id}` accepts the same fields as create; omitted fields are left unchanged.

```bash
curl -X PUT http://localhost:2323/agents/01ABC... \
  -H "Content-Type: application/json" \
  -d '{
    "instructions": "You are a fast, practical coding assistant.",
    "tools": ["bash", "read", "edit", "glob", "grep"]
  }'
```
```
