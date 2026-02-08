---
title: "SDK"
group: "Usage"
order: 10
---
# SDK

If you want more fine grained control over messages, storage, or anything else that the built in server tries to handle for you, the Go SDK provides direct access to Wingman's primitives so that you may do with them what you please.

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Example

```go
package main

import (
    "context"
    "log"

    "wingman/provider/anthropic"
    "wingman/agent"
    "wingman/session"
    "wingman/tool"
)

func main() {
    p := anthropic.New(anthropic.Config{
        model: "claude-sonnet-4-5"
    })

    a := agent.New("MyAgent",
        agent.WithInstructions("You are a helpful assistant."),
        agent.WithMaxTokens(4096),
        agent.WithTools(
            tool.NewBashTool(),
        ),
    )

    s := session.New(
        session.WithAgent(a),
        session.WithProvider(p),
    )

    result, err := s.Run(context.Background(), "What operating system am I using?")
    if err != nil {
        log.Fatal(err)
    }

    log.Println(result.Response)
}
```

## Core Primitives

### Provider

Interface for LLM providers. Currently supports Anthropic.

```go
p := anthropic.New(anthropic.Config{
    APIKey: "sk-...",  // Optional, defaults to ANTHROPIC_API_KEY env var
    Model:  "claude-sonnet-4-20250514",
})
```

### Tools

Built-in tools for common operations:

```go
tool.NewBashTool()     // Execute shell commands
tool.NewReadTool()     // Read file contents
tool.NewWriteTool()    // Write files
tool.NewEditTool()     // Edit files with find/replace
tool.NewGlobTool()     // Find files by pattern
tool.NewGrepTool()     // Search file contents
tool.NewWebFetchTool() // Fetch URLs
```

## Fleet (Concurrent Execution)

Run multiple prompts concurrently across worker actors:

```go
fleet := actor.NewFleet(actor.FleetConfig{
    WorkerCount: 3,
    Agent:       a,
    Provider:    p,
    WorkDir:     "/path/to/workdir",
})
defer fleet.Shutdown()

fleet.SubmitAll([]string{
    "Task 1",
    "Task 2", 
    "Task 3",
})

results := fleet.AwaitAll()
for _, r := range results {
    if r.Error != nil {
        log.Printf("Error: %v", r.Error)
    } else {
        log.Printf("Result: %s", r.Result.Response)
    }
}
```

## Streaming

For streaming responses:

```go
stream, err := s.RunStream(ctx, "Your prompt")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    event := stream.Event()
    // Handle streaming events
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
    Usage     WingmanUsage     // Token usage (InputTokens, OutputTokens)
    Steps     int              // Number of inference steps
}
```
