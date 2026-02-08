---
title: "Providers"
group: "Primitives"
order: 100
---

# Providers

Providers are just an interface so that it's easy to translate between a model provider's specific typing and the typing Wingman uses. If you read the *Introduction*, this project was largely inspired by OpenCode's server. Instead of using Vercel's AI SDK, I've opted to define provider translation within Wingman. The con of this pattern (assuming it doesn't change) is that Wingman will likely never have the comprehensive support of the models you'll find on [models.dev](https://models.dev), the pro is that Wingman's core dependencies are pretty limited.

## SDK

```go
import (
    "wingman/provider/anthropic"
)

p := anthropic.New()
```

Under the hood this creates an internal anthropic client that gets used to make calls during a `Session.Run()`.

## Server

```
GET    /provider                    # List all providers
GET    /provider/{name}             # Get provider info
GET    /provider/{name}/models      # List available models
GET    /provider/{name}/models/{id} # Get model details
GET    /provider/auth               # Check auth status
PUT    /provider/auth               # Set provider credentials
DELETE /provider/auth/{provider}    # Remove provider credentials
```

```bash
curl -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"work_dir": "/path/to/project"}'
```

```bash
curl -X POST http://localhost:2323/sessions/{id}/message \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "01ABC...",
    "prompt": "What files are in this directory?"
  }'
```

