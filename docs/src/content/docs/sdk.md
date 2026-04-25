---
title: "SDK"
group: "Usage"
draft: false
order: 10
---

# SDK

Use the Go SDK when you want direct access to Wingman's runtime primitives inside your own application.

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Minimal example

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/joho/godotenv"

    "github.com/chaserensberger/wingman/agent"
    "github.com/chaserensberger/wingman/provider/anthropic"
    "github.com/chaserensberger/wingman/session"
    "github.com/chaserensberger/wingman/tool"
)

func main() {
    _ = godotenv.Load(".env.local")

    p, err := anthropic.New(anthropic.Config{
        Options: map[string]any{
            "model":      "claude-sonnet-4-5",
            "max_tokens": 4096,
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    a := agent.New("Coder",
        agent.WithInstructions("You are a senior Go developer."),
        agent.WithProvider(p),
        agent.WithTools(tool.NewBashTool(), tool.NewWriteTool()),
    )

    s := session.New(session.WithAgent(a))
    result, err := s.Run(context.Background(), "Write hello.go and run it")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Response)
}
```

## Build from primitives

The typical SDK flow is:

1. construct a provider
2. create an agent
3. attach tools and optional output schema
4. create a session or fleet
5. run work with your own lifecycle and persistence decisions

The focused snippets below omit package imports unless they are relevant to the example.

## Providers

Provider packages expose typed constructors and accept a free-form `Options map[string]any` for inference settings.

```go
p, err := anthropic.New(anthropic.Config{
    APIKey: "sk-...",
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
        "temperature": 0.2,
    },
})
```

If you prefer the registry path used by the server, blank-import a provider and construct it by ID:

```go
import _ "github.com/chaserensberger/wingman/provider/anthropic"
import "github.com/chaserensberger/wingman/provider"

p, err := provider.New("anthropic", map[string]any{
    "model":      "claude-opus-4-6",
    "max_tokens": 4096,
    "api_key":    "sk-...",
})
```

## Sessions

Sessions are ephemeral in the SDK. Keep a `*session.Session` alive if you want multi-turn context.

```go
s := session.New(session.WithAgent(a))

result1, _ := s.Run(ctx, "What is 2 + 2?")
result2, _ := s.Run(ctx, "Multiply that by 10")

_ = result1
_ = result2
```

You can also set a working directory for tool execution:

```go
s := session.New(
    session.WithAgent(a),
    session.WithWorkDir("/path/to/project"),
)
```

## Streaming

Use `RunStream` when you want incremental events rather than waiting for the final response.

```go
stream, err := s.RunStream(ctx, "Tell me a story")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    event := stream.Event()
    if event.Type == core.EventTextDelta {
        fmt.Print(event.Text)
    }
}

if err := stream.Err(); err != nil {
    log.Fatal(err)
}

result := stream.Result()
_ = result
```

## Fleets

Use fleets when you want one agent template to handle many tasks concurrently.

```go
f := fleet.New(fleet.Config{
    Agent: a,
    Tasks: []fleet.Task{
        {Message: "Analyze auth module", WorkDir: "/src/auth", Data: "auth"},
        {Message: "Analyze API module", WorkDir: "/src/api", Data: "api"},
    },
    MaxWorkers: 2,
})

results, err := f.Run(ctx)
if err != nil {
    log.Fatal(err)
}

for _, r := range results {
    if r.Error != nil {
        log.Printf("task %d failed: %v", r.TaskIndex, r.Error)
        continue
    }
    log.Printf("task %d: %s", r.TaskIndex, r.Result.Response)
}
```

## Tools

Built-in tools are available in the SDK as constructors in `tool/`, and custom tools are supported by implementing `core.Tool`.

See [Tools](./tools) for the built-in list and the custom-tool interface.
