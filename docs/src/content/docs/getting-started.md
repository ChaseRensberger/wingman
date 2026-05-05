---
title: "Quickstart"
group: "Getting started"
draft: true
order: 5
---

# Quickstart

This page walks through a minimal Go SDK session and the equivalent HTTP-server flow.

## Prerequisites

- Go 1.25+
- An Anthropic API key in `ANTHROPIC_API_KEY` (or in a `.env.local`)

## Install

```bash
go get github.com/chaserensberger/wingman
```

## Run a session in-process

The SDK has no `agent` package. You construct a `*session.Session` directly and pass it a model, system prompt, tools, and any opt-in plugins.

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/joho/godotenv"

    "github.com/chaserensberger/wingman/plugins/compaction"
    "github.com/chaserensberger/wingman/agent/session"
    "github.com/chaserensberger/wingman/tool"
    "github.com/chaserensberger/wingman/models/providers/anthropic"
)

func main() {
    godotenv.Load(".env.local")

    workDir, err := os.Getwd()
    if err != nil {
        log.Fatal(err)
    }

    p, err := anthropic.New(anthropic.Config{})
    if err != nil {
        log.Fatalf("failed to create Anthropic provider: %v", err)
    }

    s := session.New(
        session.WithWorkDir(workDir),
        session.WithModel(p),
        session.WithSystem("You are a helpful coding assistant."),
        session.WithTools(
            tool.NewBashTool(),
            tool.NewReadTool(),
        ),
        // Plugins are opt-in. Compaction keeps long sessions under
        // the model's context window.
        session.WithPlugin(compaction.New()),
    )

    ctx := context.Background()
    result, err := s.Run(ctx, "What files are in this directory?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Response)
    fmt.Printf("Tokens — input: %d, output: %d\n",
        result.Usage.InputTokens, result.Usage.OutputTokens)
}
```

`Run` returns a `*session.Result` with the assistant's final text, every tool call made during the run, cumulative token usage, the number of turns, and a `StopReason`.

Keep the same `*session.Session` alive across calls if you want multi-turn context.

## Stream the same session

`RunStream` returns a single-consumer iterator over typed events.

```go
stream, err := s.RunStream(ctx, "Tell me about the largest file here.")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    event := stream.Event()
    fmt.Printf("[%s] %v\n", event.Type, event.Data)
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
result := stream.Result()
_ = result
```

See [Streaming](./agent/streaming) for the event taxonomy.

## Run the server

```bash
go run ./cmd/wingman serve
```

Defaults: `127.0.0.1:2323`, SQLite at `~/.local/share/wingman/wingman.db`. Configure provider auth, create an agent, create a session, then send messages:

```bash
# 1. Configure provider auth
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d '{"providers": {"anthropic": {"type": "api_key", "key": "sk-ant-..."}}}'

# 2. Create an agent
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Assistant",
    "instructions": "Be helpful and concise.",
    "tools": ["bash", "read", "write"],
    "provider": "anthropic",
    "model": "claude-haiku-4-5",
    "options": {"max_tokens": 4096}
  }'

# 3. Create a session
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d '{"title": "demo", "work_dir": "/tmp"}'

# 4. Send a message
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "agt_...", "message": "What OS am I on?"}'
```

See [Server](./server) for full request/response details.

## Next

- [SDK](./sdk) — full SDK surface
- [Sessions](./agent/sessions) — what `Run` actually does
- [Plugins](./agent/plugins) — opt-in extensions like compaction
- [API](./api) — endpoint reference
