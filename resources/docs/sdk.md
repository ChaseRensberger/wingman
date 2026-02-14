---
title: "SDK"
group: "Usage"
order: 11
---

# SDK

The Go SDK provides direct access to Wingman's primitives for fine-grained control over messages, storage, and execution — without the persistence and HTTP layers that the [server](/docs/server) provides.

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Quick Start

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
        Model: "claude-sonnet-4-5",
    })

    a := agent.New("MyAgent",
        agent.WithInstructions("You are a helpful assistant."),
        agent.WithProvider(p),
        agent.WithTools(tool.NewBashTool()),
    )

    s := session.New(session.WithAgent(a))

    result, err := s.Run(context.Background(), "What operating system am I using?")
    if err != nil {
        log.Fatal(err)
    }

    log.Println(result.Response)
}
```

## Primitives

The SDK is built around three core primitives. Each has its own reference page:

- **[Providers](/docs/providers)** — Interface for LLM providers. Owns inference configuration (model, max tokens, temperature) and is attached to an agent.
- **[Agents](/docs/agents)** — Stateless templates that define how to handle a unit of work (name, instructions, tools, provider).
- **[Sessions](/docs/sessions)** — Stateful containers that maintain conversation history and execute the agent loop (`Run` / `RunStream`).
- **[Tools](/docs/tools)** — Built-in and custom capabilities that agents can invoke during execution.

## Fleet (Concurrent Execution)

Run multiple messages concurrently across worker actors using the actor model:

```go
fleet := actor.NewFleet(actor.FleetConfig{
    WorkerCount: 3,
    Agent:       a,
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

For individual submissions with attached metadata, use `fleet.Submit(message, data)`.

See [Architecture](/docs/architecture) for more on the actor model design.
