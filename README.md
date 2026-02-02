# Wingman

A Go framework for building LLM-powered agents with tool execution and actor-based concurrency.

## Features

- **Stateless Agents** - Define agents as reusable configurations
- **Stateful Sessions** - Maintain conversation history and working directory context
- **Tool Execution** - Built-in tools for bash, file operations, code search, and web fetching
- **Actor System** - Run multiple agents in parallel with fault isolation
- **Direct Provider Integration** - No external SDK dependencies

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Quick Start

### Basic Session

Sessions are the primary way to run agents. They maintain conversation history and provide a working directory context for filesystem tools.

```go
package main

import (
    "context"
    "fmt"
    "wingman/pkg/agent"
    "wingman/pkg/provider/anthropic"
    "wingman/pkg/session"
    "wingman/pkg/tool"
)

func main() {
    p := anthropic.New(anthropic.Config{})
    
    a := agent.New("Coder",
        agent.WithInstructions("You are a helpful coding assistant."),
        agent.WithTools(
            tool.NewBashTool(),
            tool.NewReadTool(),
            tool.NewWriteTool(),
        ),
    )

    s := session.New(
        session.WithWorkDir("/path/to/project"),
        session.WithAgent(a),
        session.WithProvider(p),
    )

    result, err := s.Run(context.Background(), "Create a hello.py file")
    if err != nil {
        panic(err)
    }
    
    fmt.Println(result.Response)
}
```

### Multi-turn Conversation

Sessions maintain history across multiple interactions:

```go
s := session.New(
    session.WithAgent(a),
    session.WithProvider(p),
)

result, _ := s.Run(ctx, "My name is Alice")
fmt.Println(result.Response)

result, _ = s.Run(ctx, "What's my name?")
fmt.Println(result.Response)
```

### Agent Swapping

Sessions can switch agents while preserving conversation history:

```go
codingAgent := agent.New("Coder",
    agent.WithInstructions("You write code."),
    agent.WithTools(tool.NewWriteTool(), tool.NewBashTool()),
)

reviewAgent := agent.New("Reviewer",
    agent.WithInstructions("You review code."),
    agent.WithTools(tool.NewReadTool()),
)

s := session.New(
    session.WithWorkDir("/project"),
    session.WithAgent(codingAgent),
    session.WithProvider(p),
)

s.Run(ctx, "Write a fibonacci function")

s.SetAgent(reviewAgent)
s.Run(ctx, "Review the code you just wrote")
```

### Web Research Agent

Agents without filesystem tools don't require a working directory:

```go
a := agent.New("Researcher",
    agent.WithInstructions("You research topics using the web."),
    agent.WithTools(tool.NewWebFetchTool()),
)

s := session.New(
    session.WithAgent(a),
    session.WithProvider(p),
)

result, _ := s.Run(ctx, "Fetch https://news.ycombinator.com and summarize the top stories")
```

### Parallel Workers (Actor Pool)

For batch processing with multiple concurrent agents:

```go
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
```

## Configuration

### Environment Variables

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

### Provider Configuration

```go
p := anthropic.New(anthropic.Config{
    APIKey:      "sk-ant-...",
    Model:       "claude-sonnet-4-20250514",
    MaxTokens:   8192,
    Temperature: ptr(0.7),
})
```

## Built-in Tools

| Tool | Description | Requires WorkDir |
|------|-------------|------------------|
| `BashTool` | Execute shell commands | Yes |
| `ReadTool` | Read file contents | Yes |
| `WriteTool` | Create or overwrite files | Yes |
| `EditTool` | Edit files with string replacement | Yes |
| `GlobTool` | Find files by pattern | Yes |
| `GrepTool` | Search file contents with regex | Yes |
| `WebFetchTool` | Fetch and parse web content | No |

```go
a := agent.New("Coder",
    agent.WithTools(
        tool.NewBashTool(),
        tool.NewReadTool(),
        tool.NewWriteTool(),
        tool.NewEditTool(),
        tool.NewGlobTool(),
        tool.NewGrepTool(),
        tool.NewWebFetchTool(),
    ),
)
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                        User Code                         │
├─────────────────────────────────────────────────────────┤
│          Session (stateful execution context)            │
│   ┌─────────────┐  ┌──────────┐  ┌─────────────────┐   │
│   │   WorkDir   │  │  Agent   │  │  Conversation   │   │
│   │  (security  │  │  (config │  │    History      │   │
│   │  boundary)  │  │   only)  │  │                 │   │
│   └─────────────┘  └──────────┘  └─────────────────┘   │
├─────────────────────────────────────────────────────────┤
│                    Provider Interface                    │
├─────────────────────────────────────────────────────────┤
│   Anthropic   │   OpenAI (soon)   │   Custom Provider   │
└─────────────────────────────────────────────────────────┘
```

### Key Concepts

- **Agent**: A stateless definition containing instructions, tools, and configuration. Can be reused across multiple sessions. Agents don't execute anything directly.

- **Session**: The execution context that combines an agent, provider, working directory, and conversation history. All agent execution happens through sessions.

- **WorkDir**: The working directory for filesystem tools. Acts as a security boundary - tools cannot access files outside this directory.

- **Provider**: Interface for LLM communication. Currently supports Anthropic Claude.

- **Actor System**: Enables running multiple sessions concurrently with isolated mailboxes and fault tolerance.

## Examples

```bash
go run examples/agent/basic/main.go

go run examples/session/main.go

go run examples/webfetch/main.go

go run examples/pool/main.go
```

## License

MIT
