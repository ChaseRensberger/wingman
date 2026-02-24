---
title: "SDK"
group: "Usage"
order: 11
---
# SDK

If you want more fine-grained control over messages, storage, or anything else that the built-in server handles for you, the Go SDK provides direct access to Wingman's primitives.

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Example

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
    godotenv.Load(".env.local")

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

## Core Primitives

### Provider

Each provider has a `Config` with a provider-specific field for auth/connection and a generic `Options map[string]any` for inference parameters — using the same key names as the HTTP API.

```go
p, err := anthropic.New(anthropic.Config{
    APIKey: "sk-...", // optional; defaults to ANTHROPIC_API_KEY env var
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})

a := agent.New("MyAgent",
    agent.WithProvider(p),
    agent.WithInstructions("..."),
)
```

```go
p, err := ollama.New(ollama.Config{
    Options: map[string]any{
        "model":    "llama3.2",
        "base_url": "http://localhost:11434",
    },
})
```

### Tools

See [Tools](./tools) for the full list of built-in tools and the `Tool` interface for custom tools.

```go
agent.New("MyAgent",
    agent.WithTools(
        tool.NewBashTool(),
        tool.NewReadTool(),
    ),
)
```

## Fleet (Concurrent Execution)

Run multiple tasks concurrently:

```go
f := fleet.New(fleet.Config{
    Agent: a,
    Tasks: []fleet.Task{
        {Message: "Task 1", WorkDir: "/dir1"},
        {Message: "Task 2", WorkDir: "/dir2"},
        {Message: "Task 3", WorkDir: "/dir3"},
    },
    MaxWorkers: 2,
})

ctx := context.Background()
results, err := f.Run(ctx)
if err != nil {
    log.Fatal(err)
}
for _, r := range results {
    if r.Error != nil {
        log.Printf("Task %d failed: %v", r.TaskIndex, r.Error)
    } else {
        log.Printf("Task %d: %s", r.TaskIndex, r.Result.Response)
    }
}
```

## Streaming

For streaming responses:

```go
stream, err := s.RunStream(ctx, "Your message")
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
```

## Result Structure

```go
type Result struct {
    Response  string           // Final text response
    ToolCalls []ToolCallResult // All tool calls made
    Usage     core.Usage       // Token usage (InputTokens, OutputTokens)
    Steps     int              // Number of inference steps
}
```
