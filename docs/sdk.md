---
title: "SDK"
group: "Usage"
order: 101
---
# SDK

If you feel like the http server is 

The Go SDK provides direct access to Wingman's primitives for maximum control over agent orchestration.

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Quick Start (with the Anthropic api)

```go
package main

import (
    "context"
    "log"

    "wingman/agent"
    "wingman/provider/anthropic"
    "wingman/session"
    "wingman/tool"
)

func main() {
    p := anthropic.New(anthropic.Config{})

    a := agent.New("MyAgent",
        agent.WithInstructions("You are a helpful assistant."),
        agent.WithMaxTokens(4096),
        agent.WithTools(
            tool.NewBashTool(),
            tool.NewReadTool(),
            tool.NewWriteTool(),
        ),
    )

    s := session.New(
        session.WithAgent(a),
        session.WithProvider(p),
        session.WithWorkDir("/path/to/workdir"),
    )

    result, err := s.Run(context.Background(), "Hello, world!")
    if err != nil {
        log.Fatal(err)
    }

    log.Println(result.Response)
}
```

## Core Primitives

### Agent

A stateless template that defines how to process work.

```go
a := agent.New("AgentName",
    agent.WithInstructions("System prompt for the agent"),
    agent.WithMaxTokens(4096),
    agent.WithTemperature(0.7),
    agent.WithMaxSteps(50),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool()),
    agent.WithOutputSchema(map[string]any{"type": "object", ...}),
)
```

### Session

A stateful container that maintains conversation history and executes agent loops.

```go
s := session.New(
    session.WithAgent(a),
    session.WithProvider(p),
    session.WithWorkDir("/path/to/workdir"),
)

result, err := s.Run(ctx, "Your prompt here")

s.History()      // Get conversation history
s.Clear()        // Clear history
s.ID()           // Get session ID
```

The `Run` method executes the agent loop: it sends the prompt, handles tool calls, and continues until the model produces a final response or hits `MaxSteps`.

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
