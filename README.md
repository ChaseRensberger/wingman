# Wingman

A Go framework for building LLM-powered agents with tool execution and actor-based concurrency.

## Features

- **Stateless Agents** - Define agents as reusable configurations
- **Stateful Sessions** - Maintain conversation history across multiple interactions
- **Tool Execution** - Built-in tools for bash, file operations, and code search
- **Actor System** - Run multiple agents in parallel with fault isolation
- **Direct Provider Integration** - No external SDK dependencies

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Quick Start

### One-Shot Agent (Stateless)

For tasks that don't need history:

```go
package main

import (
    "context"
    "fmt"
    "wingman/agent"
    "wingman/provider/anthropic"
    "wingman/tool"
)

func main() {
    p := anthropic.New(anthropic.Config{})
    
    a := agent.New("Assistant",
        agent.WithInstructions("You are a helpful coding assistant."),
        agent.WithTools(
            tool.NewBashTool("."),
            tool.NewReadTool("."),
            tool.NewWriteTool("."),
        ),
    )

    result, err := a.Run(context.Background(), p, "Create a hello.py file that prints 'Hello, World!'")
    if err != nil {
        panic(err)
    }
    
    fmt.Println(result.Response)
}
```

### Session (Stateful)

```go
package main

import (
    "context"
    "fmt"
    "wingman/agent"
    "wingman/provider/anthropic"
    "wingman/session"
)

func main() {
    p := anthropic.New(anthropic.Config{})
    a := agent.New("Assistant", agent.WithInstructions("You are helpful."))

    s := session.New(
        session.WithAgent(a),
        session.WithProvider(p),
    )

    // First message
    result, _ := s.Run(context.Background(), "My name is Alice")
    fmt.Println(result.Response)

    // Follow-up (has context from previous message)
    result, _ = s.Run(context.Background(), "What's my name?")
    fmt.Println(result.Response) // "Your name is Alice"
}
```

### Parallel Workers (Actor Pool)

For batch processing with multiple concurrent agents:

```go
package main

import (
    "fmt"
    "wingman/actor"
    "wingman/agent"
    "wingman/provider/anthropic"
)

func main() {
    p := anthropic.New(anthropic.Config{})
    a := agent.New("Calculator",
        agent.WithInstructions("Solve math problems. Reply with only the number."),
    )

    pool := actor.NewPool(actor.PoolConfig{
        WorkerCount: 5,
        Agent:       a,
        Provider:    p,
    })
    defer pool.Shutdown()

    problems := []string{
        "What is 15 * 7?",
        "What is 123 + 456?",
        "What is 1000 / 8?",
    }

    pool.SubmitAll(problems)
    results := pool.AwaitAll()

    for _, r := range results {
        fmt.Printf("%s: %s\n", r.WorkerName, r.Result.Response)
    }
}
```

## Configuration

### Environment Variables

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

### Provider Configuration

```go
p := anthropic.New(anthropic.Config{
    APIKey:      "sk-ant-...",        // or use ANTHROPIC_API_KEY env var
    Model:       "claude-sonnet-4-20250514",
    MaxTokens:   8192,
    Temperature: ptr(0.7),
})
```

## Built-in Tools

| Tool | Description |
|------|-------------|
| `BashTool` | Execute shell commands |
| `ReadTool` | Read file contents |
| `WriteTool` | Create or overwrite files |
| `EditTool` | Edit files with string replacement |
| `GlobTool` | Find files by pattern |
| `GrepTool` | Search file contents with regex |

```go
tools := []tool.Tool{
    tool.NewBashTool(workDir),
    tool.NewReadTool(workDir),
    tool.NewWriteTool(workDir),
    tool.NewEditTool(workDir),
    tool.NewGlobTool(workDir),
    tool.NewGrepTool(workDir),
}

a := agent.New("Coder", agent.WithTools(tools...))
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        User Code                         │
├─────────────────────────────────────────────────────────┤
│   Agent (stateless)  │  Session (stateful)  │   Pool    │
├─────────────────────────────────────────────────────────┤
│                    Provider Interface                    │
├─────────────────────────────────────────────────────────┤
│   Anthropic   │   OpenAI (soon)   │   Custom Provider   │
└─────────────────────────────────────────────────────────┘
```

### Key Concepts

- **Agent**: A stateless definition containing instructions, tools, and configuration. Can be reused across multiple sessions.

- **Session**: A stateful container that holds an agent reference, provider, and conversation history. Maintains context across multiple `Run()` calls.

- **Provider**: Interface for LLM communication. Currently supports Anthropic Claude.

- **Actor System**: Enables running multiple agents concurrently with isolated mailboxes and fault tolerance.

## Examples

Run the examples:

```bash
# Basic one-shot agent
go run examples/agent/basic/main.go

# Stateful session
go run examples/session/main.go

# Parallel worker pool
go run examples/pool/main.go
```
