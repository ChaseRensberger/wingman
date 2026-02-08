---
title: "Server"
group: "Usage"
order: 5
---
# Server

The HTTP server is the primary way to use Wingman. It's batteries-included with

## Starting the Server

```bash
wingman serve
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 2323 | Port to listen on |
| `--host` | 127.0.0.1 | Host to bind to |
| `--db` | ~/.local/share/wingman/wingman.db | Database path |

## Authentication

Before making inference requests, configure your provider API keys:

```bash
curl -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'
```

## API Reference

### Providers

```
GET    /provider                    # List all providers
GET    /provider/{name}             # Get provider info
GET    /provider/{name}/models      # List available models
GET    /provider/{name}/models/{id} # Get model details
GET    /provider/auth               # Check auth status
PUT    /provider/auth               # Set provider credentials
DELETE /provider/auth/{provider}    # Remove provider credentials
```

### Agents

Agents are stateless templates that define how to process work.

```
POST   /agents      # Create agent
GET    /agents      # List agents
GET    /agents/{id} # Get agent
PUT    /agents/{id} # Update agent
DELETE /agents/{id} # Delete agent
```

#### Create Agent

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

**Available tools:** `bash`, `read`, `write`, `edit`, `glob`, `grep`, `webfetch`

### Sessions

Sessions maintain conversation history across multiple messages.

```
POST   /sessions              # Create session
GET    /sessions              # List sessions
GET    /sessions/{id}         # Get session
PUT    /sessions/{id}         # Update session
DELETE /sessions/{id}         # Delete session
POST   /sessions/{id}/message # Send message (blocking)
POST   /sessions/{id}/message/stream # Send message (streaming)
```

#### Create Session

```bash
curl -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}'
```

#### Send Message

```bash
curl -X POST http://localhost:2323/sessions/{id}/message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "01ABC...",
    "prompt": "What files are in this directory?"
  }'
```

**Response:**

```json
{
  "response": "The directory contains...",
  "tool_calls": [...],
  "usage": {"input_tokens": 150, "output_tokens": 50},
  "steps": 2
}
```

#### Streaming

For streaming responses, use the `/message/stream` endpoint. Events are sent as SSE:

```bash
curl -X POST http://localhost:2323/sessions/{id}/message/stream \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "01ABC...", "prompt": "Hello"}'
```

Events: `text`, `tool_use`, `tool_result`, `done`, `error`

## Example Workflow

```bash
# 1. Configure auth
curl -X PUT http://localhost:2323/provider/auth \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'

# 2. Create an agent
curl -X POST http://localhost:2323/agents \
  -d '{"name": "Assistant", "instructions": "Be helpful", "tools": ["bash"]}'
# Returns: {"id": "01ABC...", ...}

# 3. Create a session
curl -X POST http://localhost:2323/sessions \
  -d '{"work_dir": "/tmp"}'
# Returns: {"id": "01XYZ...", ...}

# 4. Send messages
curl -X POST http://localhost:2323/sessions/01XYZ.../message \
  -d '{"agent_id": "01ABC...", "prompt": "What OS am I on?"}'
```
