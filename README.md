# Wingman

A Go framework for building LLM-powered agents with tool execution, actor-based concurrency, and a batteries include http server (if you don't want to use the go sdk).

## Features

- **Stateless Agents** - Define agents as reusable configurations
- **Stateful Sessions** - Maintain conversation history and working directory context
- **Tool Execution** - Built-in tools for bash, file operations, code search, and web fetching
- **Actor System** - Run multiple agents in parallel with fault isolation
- **Fleets** - Worker pools for parallel batch processing
- **Formations** - Multi-agent topologies for complex workflows
- **HTTP API Server** - REST API with JSON storage for managing agents, sessions, fleets, and formations
- **SSE Streaming** - Real-time streaming responses via Server-Sent Events
- **Direct Provider Integration** - No external SDK dependencies

## Installation

```bash
curl -fsSL https://wingman.actor/install | bash
```

## HTTP Server

### Starting the Server

```bash
go build -o wingman ./cmd/wingman
./wingman serve --port 8080
```

### Server Options

```
--port, -p     Port to listen on (default: 8080)
--host, -h     Host to bind to (default: 127.0.0.1)
--data-dir, -d Data directory for JSON storage (default: ~/.local/share/wingman)
```

### API Endpoints

#### Health
```
GET /health              Health check
```

#### Authentication
```
PUT /auth                Set provider credentials
GET /auth                Get providers (keys redacted)
```

#### Agents
```
POST   /agents           Create agent
GET    /agents           List agents
GET    /agents/:id       Get agent
PUT    /agents/:id       Update agent
DELETE /agents/:id       Delete agent
```

#### Sessions
```
POST   /sessions              Create session
GET    /sessions              List sessions
GET    /sessions/:id          Get session
PUT    /sessions/:id          Update session
DELETE /sessions/:id          Delete session
POST   /sessions/:id/run      Run inference (blocking)
POST   /sessions/:id/stream   Run inference (SSE streaming)
```

#### Fleets
```
POST   /fleets               Create fleet
GET    /fleets               List fleets
GET    /fleets/:id           Get fleet
PUT    /fleets/:id           Update fleet
DELETE /fleets/:id           Delete fleet
POST   /fleets/:id/start     Start fleet workers
POST   /fleets/:id/stop      Stop fleet workers
POST   /fleets/:id/submit    Submit prompts to fleet
```

#### Formations
```
POST   /formations           Create formation
GET    /formations           List formations
GET    /formations/:id       Get formation
PUT    /formations/:id       Update formation
DELETE /formations/:id       Delete formation
POST   /formations/:id/start Start formation (not yet implemented)
POST   /formations/:id/stop  Stop formation (not yet implemented)
POST   /formations/:id/run   Run formation (not yet implemented)
```

### API Examples

#### Set Authentication
```bash
curl -X PUT http://localhost:8080/auth \
  -H "Content-Type: application/json" \
  -d '{
    "anthropic": {
      "type": "api_key",
      "key": "sk-ant-..."
    }
  }'
```

#### Create an Agent
```bash
curl -X POST http://localhost:8080/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Coder",
    "instructions": "You are a helpful coding assistant.",
    "provider": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"]
  }'
```

#### Create a Session
```bash
curl -X POST http://localhost:8080/sessions \
  -H "Content-Type: application/json" \
  -d '{
    "work_dir": "/path/to/project"
  }'
```

#### Run Inference
```bash
curl -X POST http://localhost:8080/sessions/{session_id}/run \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "{agent_id}",
    "prompt": "Create a hello.py file that prints hello world"
  }'
```

#### Stream Inference (SSE)
```bash
curl -N http://localhost:8080/sessions/{session_id}/stream \
  -H "Content-Type: application/json" \
  -d '{
    "agent_id": "{agent_id}",
    "prompt": "Explain the code in main.go"
  }'
```

### Data Storage

The server stores all data as JSON files:

```
~/.local/share/wingman/
├── auth.json
├── agents/{id}.json
├── sessions/{id}.json
├── fleets/{id}.json
└── formations/{id}.json
```

## SDK Usage

### Basic Session

```go
package main

import (
    "context"
    "fmt"
    "wingman/agent"
    "wingman/provider/anthropic"
    "wingman/session"
    "wingman/tool"
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

### Streaming Session

```go
s := session.NewStreaming(
    session.WithWorkDir("/project"),
    session.WithAgent(a),
    session.WithProvider(p),
)

eventCh, err := s.Stream(ctx, "Write a fibonacci function")
if err != nil {
    panic(err)
}

for event := range eventCh {
    switch event.Type {
    case "content_delta":
        fmt.Print(event.Content)
    case "tool_use":
        fmt.Printf("\n[Using tool: %s]\n", event.ToolName)
    case "done":
        fmt.Println("\n[Complete]")
    case "error":
        fmt.Printf("\n[Error: %s]\n", event.Error)
    }
}
```

### Parallel Workers (Fleet)

```go
a := agent.New("Calculator",
    agent.WithInstructions("Solve math problems. Reply with only the number."),
)

fleet := actor.NewFleet(actor.FleetConfig{
    WorkerCount: 5,
    Agent:       a,
    Provider:    p,
})
defer fleet.Shutdown()

problems := []string{
    "What is 15 * 7?",
    "What is 123 + 456?",
    "What is 1000 / 8?",
}

fleet.SubmitAll(problems)
results := fleet.AwaitAll()

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
| `bash` | Execute shell commands | Yes |
| `read` | Read file contents | Yes |
| `write` | Create or overwrite files | Yes |
| `edit` | Edit files with string replacement | Yes |
| `glob` | Find files by pattern | Yes |
| `grep` | Search file contents with regex | Yes |
| `webfetch` | Fetch and parse web content | No |

### Key Concepts

- **Agent**: A stateless definition containing instructions, tools, and configuration. Can be reused across multiple sessions.

- **Session**: The execution context that combines an agent, provider, working directory, and conversation history. Sessions do not have a fixed agent - agents are passed per request, allowing agent swapping.

- **Fleet**: A pool of workers running the same agent in parallel. Useful for batch processing multiple prompts concurrently.

- **Formation**: A multi-agent topology defining roles (agent + count) and edges (handoff conditions). Enables complex workflows like planner → researchers → synthesizer.

- **WorkDir**: The working directory for filesystem tools. Acts as a security boundary.

- **Provider**: Interface for LLM communication. Currently supports Anthropic Claude.
