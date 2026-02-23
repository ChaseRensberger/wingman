---
title: "Wingman — Master Reference"
draft: true
---

# Wingman — Master Reference

This document is the single authoritative reference for Wingman's design, architecture, data flow, and all major decisions made during development. It is a living document; update it when anything material changes.

---

## What Wingman Is

Wingman is a **self-hostable, airgap-friendly agent orchestration engine** written in Go. It can be used two ways:

1. **Go SDK** — import the packages directly. Run agents in-process. You own the persistence layer (or skip it entirely).
2. **HTTP server** — run `wingman serve`. Agents, sessions, and fleets are persisted in SQLite. Any HTTP client (curl, TypeScript, Python, another Go service) can talk to it.

The two modes are designed to be interchangeable. The same core types describe an agent, a session, and a message regardless of whether they live in memory or in a database.

**Design goals:**
- Entirely self-contained — no calls to external model/provider registries at runtime.
- Works in airgapped environments.
- Providers and models are first-class concepts, but capability metadata is left to the user for now (no built-in model database).
- Composable: agents → sessions → fleets → (formations, future).

---

## Repository Layout

```
core/                  Canonical types and interfaces (the foundation)
agent/                 Agent type and functional options
session/               Session type, blocking and streaming agentic loop
fleet/                 High-level concurrent work primitive
actor/                 Low-level actor system (mailbox-based; used by future formations)
provider/              Provider registry, interfaces, ProviderMeta
provider/anthropic/    Anthropic Messages API provider
provider/ollama/       Ollama chat API provider
tool/                  Tool interface, Registry, and all 7 built-in tools
models/                Backward-compat re-exports of core types (legacy; prefer core/)
internal/server/       HTTP server, route handlers
internal/storage/      SQLite store + storage types
examples/              Runnable examples for every major feature
resources/docs/        Documentation
```

---

## The `core` Package

`core/core.go` is the single source of truth for all shared types and interfaces. Every other package imports from `core`; `core` itself has no Wingman dependencies.

### Why a separate `core` package?

Without it, packages form circular import chains. `session` needs `agent`, `agent` needs `provider`, `provider` needs message types — a `core` package with no dependencies breaks all cycles.

It also gives a single file that explains the entire system's data model at a glance.

### Types in `core`

**Messages:**

| Type | Purpose |
|---|---|
| `Role` | `"user"` or `"assistant"` |
| `ContentType` | `"text"`, `"tool_use"`, or `"tool_result"` |
| `ContentBlock` | One piece of content in a message (text, tool call, or tool result) |
| `Message` | One conversation turn (role + list of content blocks) |

Helper constructors: `NewUserMessage`, `NewAssistantMessage`, `NewToolResultMessage`.

**Tool definitions** (sent to the LLM so it knows what tools exist):

| Type | Purpose |
|---|---|
| `ToolDefinition` | Name, description, input schema |
| `ToolInputSchema` | JSON Schema wrapper |
| `ToolProperty` | One parameter in the schema |

**Inference:**

| Type | Purpose |
|---|---|
| `Usage` | `InputTokens` + `OutputTokens` |
| `InferenceRequest` | Messages, tools, instructions, output schema |
| `InferenceResponse` | Content blocks, stop reason, usage |

`InferenceResponse` methods: `GetText()`, `GetToolCalls()`, `HasToolCalls()`.

**Streaming:**

| Type | Purpose |
|---|---|
| `StreamEventType` | One of 9 event type constants |
| `StreamEvent` | A single streaming event (all fields have `json` tags in snake_case) |
| `StreamContentBlock` | Metadata in a `content_block_start` event |

**Interfaces:**

| Interface | Methods |
|---|---|
| `Provider` | `RunInference`, `StreamInference` |
| `Stream` | `Next`, `Event`, `Err`, `Close`, `Response` |
| `Tool` | `Name`, `Description`, `Definition`, `Execute` |

---

## Provider System

### Design

Providers translate Wingman's provider-agnostic `InferenceRequest` into whatever wire format the LLM backend expects. They expose two methods: blocking (`RunInference`) and streaming (`StreamInference`).

Adding a new provider requires:
1. Writing a package that implements `core.Provider`.
2. Registering a `ProviderFactory` in `init()` via `provider.Register(ProviderMeta{..., Factory: ...})`.

That's it. The server's `buildProvider` picks up the factory automatically through the default registry — no switch statements to update.

### `provider.ProviderMeta`

```go
type ProviderMeta struct {
    ID        string          // e.g. "anthropic"
    Name      string          // e.g. "Anthropic"
    AuthTypes []AuthType      // ["api_key"] or [] for no auth
    Factory   ProviderFactory // func(opts map[string]any) (core.Provider, error)
}
```

`Factory` is not serialised to JSON (tagged `json:"-"`). It's present only in-process for instantiation.

### `provider.New()` — the registry factory

```go
p, err := provider.New("anthropic", map[string]any{
    "model":      "claude-opus-4-6",
    "max_tokens": 4096,
    "api_key":    "sk-...",
})
```

This is the same code path the server uses. SDK users who prefer it to the direct constructor can import the provider package as a blank import and call `provider.New`.

### Provider/model split

Agent definitions carry two separate string fields — `Provider` and `Model` — not a combined `"provider/model"` string. This distinction matters because:

- The same model ID (`claude-opus-4-6`) might be served by multiple providers (Anthropic direct, AWS Bedrock, GitHub Copilot) each with different context limits or capabilities.
- Parsing `"anthropic"` out of `"anthropic/claude-opus-4-6"` is fragile and hides the boundary between identity and configuration.

**HTTP API:**
```json
POST /agents
{
  "provider": "anthropic",
  "model":    "claude-opus-4-6",
  "options":  { "max_tokens": 4096 }
}
```

**SDK:**
```go
p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-opus-4-6",
        "max_tokens": 4096,
    },
})
a := agent.New("MyAgent", agent.WithProvider(p))
```

### The `Options` map

Model configuration (temperature, max_tokens, top_p, etc.) flows through a single `Options map[string]any` on the agent. The entire map is forwarded to the provider factory. This is intentionally untyped:

- Different models support different parameters. Forcing typed fields would make every model's quirks a compile-time concern.
- It mirrors how the HTTP API works — the same JSON object, the same keys.
- The tradeoff is that valid keys are documented per-provider rather than enforced by the type system. This is acceptable for now.

**Recognised keys by provider:**

| Key | Anthropic | Ollama |
|---|---|---|
| `"model"` | ✅ (default: claude-sonnet-4-5) | ✅ (required) |
| `"max_tokens"` | ✅ (default: 4096) | ✅ (maps to `num_predict`) |
| `"temperature"` | ✅ | ✅ |
| `"api_key"` | ✅ | — |
| `"base_url"` | — | ✅ (default: localhost:11434) |

**`max_tokens` note:** Anthropic's API requires `max_tokens` and returns a 400 if absent. The Anthropic provider applies a default of `4096` if not specified in Options. This default is documented but not enforced at the framework level — if you're on a different provider, no default applies.

### Auth — server path

The server stores API keys in SQLite (`auth` table, keyed by provider ID). When building a provider for a request, `buildProvider` reads the key and injects it as `opts["api_key"]` before calling the factory. The factory reads it from `opts["api_key"]` or the explicit `Config.APIKey` field.

Auth is **not** read from environment variables by the server — only from the SQLite store. This is deliberate for predictability in self-hosted deployments. The SDK path (where the user calls `anthropic.New()` directly) still falls back to `ANTHROPIC_API_KEY` as a convenience.

---

## Agent

An `agent.Agent` is a named configuration bundle: instructions (system prompt), a provider, tools, and an optional output schema.

### SDK construction

```go
p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-opus-4-6",
        "max_tokens": 4096,
    },
})
if err != nil { log.Fatal(err) }

a := agent.New("Coder",
    agent.WithInstructions("You are a senior Go developer."),
    agent.WithProvider(p),
    agent.WithTools(tool.NewBashTool(), tool.NewReadTool(), tool.NewWriteTool()),
)
```

### Server construction

The server stores agents in SQLite with `provider` and `model` as separate string fields. At request time, `buildAgent` reconstructs a live `*agent.Agent` by:
1. Resolving tool names to live `Tool` instances (built-ins only via the server).
2. Calling `buildProvider(providerID, model, options)` to get a live `core.Provider`.
3. Building `agent.New(...)` with all the pieces.

### `agent.Agent` fields

| Field | Type | Notes |
|---|---|---|
| `id` | `string` | ULID when server-managed; user-supplied or empty in SDK |
| `name` | `string` | Human display name |
| `instructions` | `string` | System prompt sent on every inference call |
| `tools` | `[]core.Tool` | Live tool instances |
| `outputSchema` | `map[string]any` | JSON Schema for structured output |
| `provider` | `core.Provider` | Live provider (nil if not set) |
| `providerID` | `string` | String provider identifier (informational) |
| `model` | `string` | String model identifier (informational) |

All fields are unexported; use getter methods and `With*` option functions.

---

## Session

A `session.Session` holds an ongoing conversation: an agent, a working directory for tool execution, and a `[]core.Message` history.

Sessions are **ephemeral in the SDK** — they live as Go structs. The server persists them to SQLite: on every `POST /sessions/{id}/message`, the server creates a fresh `session.Session`, replays the stored history into it via `AddMessage`, runs inference, then writes the updated history back.

### The agentic loop

Both `Run` (blocking) and `RunStream` (streaming) implement the same loop:

```
1. Append user message to history.
2. Build InferenceRequest from history, tools, instructions, output schema.
3. Call p.RunInference (or StreamInference).
4. Append assistant response to history.
5. If stop_reason == "tool_use":
     a. Execute all tool calls.
     b. Append tool_result message to history.
     c. Go to step 2.
6. Return result.
```

There is no hard cap on iterations. If the model keeps requesting tools, the loop continues until either the model stops or the context is cancelled.

### Tool call ID pairing

The Anthropic API (and Wingman's internal model) uses a two-step handshake for tool calls:
- The model emits a `tool_use` content block with an `ID` (e.g. `"toolu_abc123"`) and `Name` (e.g. `"bash"`).
- Wingman executes the tool and sends back a `tool_result` content block where `ToolUseID` matches the `tool_use` block's `ID`.

`ToolCallResult.ToolName` stores the call ID (not the human-readable tool name) so the caller can easily build the tool_result block. This is a deliberate design: the ID is what matters for protocol correctness.

### Blocking — `Run`

Returns `*Result` when the loop completes. `Result` contains:

```go
type Result struct {
    Response  string           // final text from the model
    ToolCalls []ToolCallResult // all tool calls made across all steps
    Usage     core.Usage       // token counts summed across all steps
    Steps     int              // number of inference calls made
}
```

### Streaming — `RunStream`

Returns `*SessionStream` immediately and starts a goroutine. Callers iterate:

```go
stream, err := s.RunStream(ctx, "Write a Go HTTP server")
for stream.Next() {
    event := stream.Event()
    if event.Type == core.EventTextDelta {
        fmt.Print(event.Text)
    }
}
result := stream.Result()
```

The SSE wire format (used by the server's `/sessions/{id}/message/stream` endpoint) serialises `StreamEvent` as JSON. All fields use snake_case tags (e.g. `"type"`, `"text"`, `"input_json"`, `"stop_reason"`).

---

## Fleet

A fleet runs one agent template against N tasks concurrently. It is the primary "fan-out" primitive.

### Mental model

You have an "Explore" agent. You have 4 directories to explore. Instead of 4 separate sessions run sequentially, you create a fleet: the same agent, 4 tasks, each with its own `WorkDir`. All 4 sessions run in parallel; results arrive as workers finish.

### Task overrides

Each `fleet.Task` can override:
- `Message` — the prompt for this worker's session (required).
- `WorkDir` — overrides the fleet's default working directory.
- `Instructions` — overrides the template agent's system prompt. If set, a new agent copy is made sharing the provider/tools/schema but with different instructions.
- `Data` — arbitrary passthrough metadata (not sent to the model; returned in `FleetResult`).

### Concurrency

`Config.MaxWorkers` (default 0 = unlimited) caps concurrent goroutines. If set, a semaphore limits how many workers run at once; excess tasks queue.

### Blocking vs. streaming

```go
// Blocking: wait for all workers
results, err := f.Run(ctx)

// Streaming: process results as they arrive
fs, err := f.RunStream(ctx)
for fs.Next() {
    r := fs.Result()
    fmt.Printf("worker %s done: %s\n", r.WorkerName, r.Result.Response)
}
```

### SDK usage

```go
p, _ := anthropic.New(anthropic.Config{
    Options: map[string]any{"model": "claude-opus-4-6"},
})
template := agent.New("Explorer",
    agent.WithInstructions("Explore the given directory and summarise its contents."),
    agent.WithProvider(p),
    agent.WithTools(tool.NewReadTool(), tool.NewGlobTool()),
)

f := fleet.New(fleet.Config{
    Agent: template,
    Tasks: []fleet.Task{
        {Message: "Explore this directory", WorkDir: "/src/auth"},
        {Message: "Explore this directory", WorkDir: "/src/api"},
        {Message: "Explore this directory", WorkDir: "/src/models"},
        {Message: "Explore this directory", WorkDir: "/src/storage"},
    },
})

results, err := f.Run(context.Background())
```

### Server usage

```
POST /fleets            Create a fleet definition (stores agent_id, worker_count, work_dir)
GET  /fleets            List all fleet definitions
GET  /fleets/{id}       Get a fleet definition
PUT  /fleets/{id}       Update a fleet definition
DELETE /fleets/{id}     Delete a fleet definition

POST /fleets/{id}/run           Run fleet (blocking)
POST /fleets/{id}/run/stream    Run fleet (SSE, results arrive per worker)
```

The `run` and `run/stream` endpoints accept a `tasks` array:

```json
POST /fleets/{id}/run
{
  "tasks": [
    { "message": "Explore /src/auth", "work_dir": "/src/auth" },
    { "message": "Explore /src/api",  "work_dir": "/src/api" }
  ]
}
```

The streaming endpoint emits one `event: result` SSE event per completed worker, followed by `event: done`.

---

## Actor System (`actor/`)

The `actor` package is a lightweight in-process actor model. It is the **lower-level primitive** that will eventually underpin formations.

### Core types

- **`Actor`** interface: `Receive(ctx, Message) error`. Anything that processes messages.
- **`System`**: manages actors, their mailboxes, and goroutines.
- **`Ref`**: a handle to a spawned actor. Send messages via `ref.Send(msg)`.
- **`Message`**: `{ID, From, Type, Payload, Timestamp}`.

### `AgentActor`

Wraps an `*agent.Agent`. When it receives a `"work"` message with a `WorkPayload{Message, Data}`, it creates a session, runs inference, and sends the result as a `"result"` message to a target `Ref` (or calls an `onResult` callback).

### `Fleet` (old, in `actor/`)

The `actor.Fleet` type still exists for backward compatibility. It uses the actor system under the hood (N `AgentActor` workers + a collector actor). The new `fleet.Fleet` (in the `fleet/` package) is the recommended API for most use cases — it is simpler and doesn't require understanding the actor primitives.

The `actor` package's value will become clearer when formations are implemented: a formation is a user-defined graph of actors that communicate via messages, which the actor system handles natively.

---

## Formations (future)

Formations are deferred — the design is still evolving. The current direction is an **actor-model-inspired** approach:

- A formation is a graph where each node is an actor (agent, fleet, or pure function).
- Actors have mailboxes. They communicate by sending messages to named actors.
- The graph is defined upfront (edges declared at formation-creation time), but message passing within it is dynamic.

This allows workflows like:

```
PDF Parser Agent → transaction_list
transaction_list → Category Summing Function → budget_summary
budget_summary   → Budget Advisor Agent → final_advice
```

Or:

```
Outline Agent → outline
outline        → Fleet of N Researcher Agents (one per section) → sections[]
sections[]     → Assembly Agent → final_document
```

The `storage.Formation` type (with `Roles` and `Edges`) exists in SQLite but has no runtime implementation yet. The `actor/` package provides the execution primitives.

---

## Built-in Tools

| Tool | Name | What it does |
|---|---|---|
| Bash | `"bash"` | Executes shell commands; maintains a persistent shell per session |
| Read | `"read"` | Reads a file from disk |
| Write | `"write"` | Creates or overwrites a file |
| Edit | `"edit"` | Makes exact string replacements in a file |
| Glob | `"glob"` | Finds files matching a glob pattern |
| Grep | `"grep"` | Searches file content with regex |
| WebFetch | `"webfetch"` | Fetches a URL and returns its content as text/markdown |

All 7 are available both in the SDK (import `tool.NewBashTool()` etc.) and the server (reference by name in the agent's `tools` array).

**Custom tools (SDK only):** Implement `core.Tool` and pass via `agent.WithTools(myTool)`. The server does not support custom tools; it only resolves the 7 built-in names.

---

## HTTP API

All endpoints return JSON. All request bodies are JSON. Error responses are `{"error": "..."}`.

The timeout middleware is set to 60 seconds for all routes. Streaming endpoints bypass this by using `http.Flusher` directly.

### Health

```
GET /health → {"status": "ok"}
```

### Providers

```
GET    /provider                         List registered providers
GET    /provider/{id}                    Get provider metadata
GET    /provider/{id}/models             List models (fetched from models.dev, cached 1hr)
GET    /provider/{id}/models/{model}     Get model metadata

GET    /provider/auth                    Get configured credentials (keys redacted)
PUT    /provider/auth                    Set credentials for one or more providers
DELETE /provider/auth/{provider}         Remove credentials for a provider
```

### Agents

```
POST   /agents                 Create agent
GET    /agents                 List agents
GET    /agents/{id}            Get agent
PUT    /agents/{id}            Update agent (partial update)
DELETE /agents/{id}            Delete agent
```

Agent fields:

| Field | Type | Notes |
|---|---|---|
| `name` | string | Required |
| `provider` | string | e.g. `"anthropic"` |
| `model` | string | e.g. `"claude-opus-4-6"` |
| `options` | object | max_tokens, temperature, etc. |
| `instructions` | string | System prompt |
| `tools` | string[] | Built-in tool names |
| `output_schema` | object | JSON Schema for structured output |

### Sessions

```
POST   /sessions               Create session
GET    /sessions               List sessions
GET    /sessions/{id}          Get session (includes history)
PUT    /sessions/{id}          Update session (work_dir only)
DELETE /sessions/{id}          Delete session

POST   /sessions/{id}/message        Send message, block until done
POST   /sessions/{id}/message/stream Send message, stream events via SSE
```

Message request:

```json
{ "agent_id": "<ulid>", "message": "Write a Python script" }
```

Message response (blocking):

```json
{
  "response":   "Here is the script...",
  "tool_calls": [{ "tool_name": "<call-id>", "output": "...", "steps": 1 }],
  "usage":      { "input_tokens": 120, "output_tokens": 45 },
  "steps":      2
}
```

Streaming SSE events (one per `StreamEvent`):
```
event: text_delta
data: {"type":"text_delta","text":"Here ","index":0}

event: message_stop
data: {"type":"message_stop"}

event: done
data: {"usage":{"input_tokens":120,"output_tokens":45},"steps":2}
```

### Fleets

```
POST   /fleets                     Create fleet definition
GET    /fleets                     List fleet definitions
GET    /fleets/{id}                Get fleet definition
PUT    /fleets/{id}                Update fleet definition
DELETE /fleets/{id}                Delete fleet definition

POST   /fleets/{id}/run            Run fleet (blocking, returns all results)
POST   /fleets/{id}/run/stream     Run fleet (SSE, one event per worker result)
```

Run request:

```json
{
  "tasks": [
    { "message": "Explore this dir", "work_dir": "/src/auth", "data": "auth" },
    { "message": "Explore this dir", "work_dir": "/src/api",  "data": "api" }
  ]
}
```

Run response (blocking):

```json
[
  { "task_index": 0, "worker_name": "worker-0", "response": "...", "steps": 1, "data": "auth" },
  { "task_index": 1, "worker_name": "worker-1", "response": "...", "steps": 1, "data": "api" }
]
```

Streaming emits `event: result` per worker, then `event: done`.

---

## Storage

SQLite is the only persistence backend. The database lives at `~/.local/share/wingman/wingman.db` by default.

### Schema

**`agents`**

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | ULID |
| `name` | TEXT | |
| `instructions` | TEXT | |
| `tools` | TEXT | JSON array of strings |
| `provider` | TEXT | e.g. `"anthropic"` |
| `model` | TEXT | e.g. `"claude-opus-4-6"` |
| `options` | TEXT | JSON object |
| `output_schema` | TEXT | JSON object |
| `created_at` | TEXT | RFC3339 UTC |
| `updated_at` | TEXT | RFC3339 UTC |

**`sessions`**

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | ULID |
| `work_dir` | TEXT | |
| `history` | TEXT | JSON array of `core.Message` |
| `created_at` | TEXT | |
| `updated_at` | TEXT | |

**`fleets`**

| Column | Type | Notes |
|---|---|---|
| `id` | TEXT PK | ULID |
| `name` | TEXT | |
| `agent_id` | TEXT | References `agents.id` (no FK constraint) |
| `worker_count` | INTEGER | 0 = unlimited |
| `work_dir` | TEXT | |
| `status` | TEXT | `"stopped"` or `"running"` |
| `created_at` | TEXT | |
| `updated_at` | TEXT | |

**`auth`** — singleton row (id=1)

| Column | Type | Notes |
|---|---|---|
| `id` | INTEGER PK | Always 1 |
| `providers` | TEXT | JSON: `{ "anthropic": { "type": "api_key", "key": "sk-..." } }` |
| `updated_at` | TEXT | |

### Migration

The schema is created with `CREATE TABLE IF NOT EXISTS`. When new columns are added (like `provider` in agents), `ALTER TABLE ... ADD COLUMN` is issued at startup and its error is silently ignored if the column already exists. This provides forward-compatible schema evolution for existing databases.

---

## SDK — How It All Fits Together

### Minimal working example

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
    if err != nil { log.Fatal(err) }

    a := agent.New("Coder",
        agent.WithInstructions("You are a senior Go developer."),
        agent.WithProvider(p),
        agent.WithTools(tool.NewBashTool(), tool.NewWriteTool()),
    )

    s := session.New(session.WithAgent(a))
    result, err := s.Run(context.Background(), "Write hello.go and run it")
    if err != nil { log.Fatal(err) }

    fmt.Println(result.Response)
}
```

### Multi-turn conversation

Keep the `*session.Session` alive and call `Run` again:

```go
s := session.New(session.WithAgent(a))

result1, _ := s.Run(ctx, "What is 2 + 2?")
result2, _ := s.Run(ctx, "Multiply that by 10")  // context is preserved
```

### Using the provider registry

Instead of calling the provider constructor directly, use the registry factory. This is the same code path the server uses internally:

```go
import _ "github.com/chaserensberger/wingman/provider/anthropic"
import "github.com/chaserensberger/wingman/provider"

p, err := provider.New("anthropic", map[string]any{
    "model":      "claude-opus-4-6",
    "max_tokens": 4096,
    "api_key":    os.Getenv("ANTHROPIC_API_KEY"),
})
```

### Streaming

```go
stream, err := s.RunStream(ctx, "Tell me a story")
for stream.Next() {
    if stream.Event().Type == core.EventTextDelta {
        fmt.Print(stream.Event().Text)
    }
}
result := stream.Result()
```

### Fleet (concurrent tasks)

```go
f := fleet.New(fleet.Config{
    Agent: myAgent,
    Tasks: []fleet.Task{
        {Message: "task 1", WorkDir: "/dir1"},
        {Message: "task 2", WorkDir: "/dir2"},
        {Message: "task 3", WorkDir: "/dir3"},
    },
    MaxWorkers: 2, // run at most 2 at a time
})

results, err := f.Run(ctx)
for _, r := range results {
    if r.Error != nil {
        fmt.Printf("Task %d failed: %v\n", r.TaskIndex, r.Error)
    } else {
        fmt.Printf("Task %d: %s\n", r.TaskIndex, r.Result.Response)
    }
}
```

---

## Design Decisions Log

### Why `core` instead of expanding `models`?

The `models` package existed before `core` and contained message types. But it had no interfaces, so `provider` and `tool` couldn't import from it without the provider packages importing `tool` (circular). Creating `core` as a zero-dependency package with both types and interfaces solves this cleanly.

`models` is kept as a thin re-export layer for backward compatibility. New code should import `core` directly.

### Why `anthropic.New` returns `(Provider, error)` instead of `Provider?`

The previous design returned `nil` on missing API key. This propagated `nil` silently through `buildProvider` into `agent.WithProvider(nil)` and only panicked at inference time. Returning an explicit error surfaces the failure immediately at construction time.

### Why `max_tokens` defaults to 4096 in the Anthropic provider?

Anthropic's Messages API requires `max_tokens` — the request fails with a 400 if absent. Rather than forcing all users to always set it, the provider applies a sensible default. Users who want a different default set `Options["max_tokens"]`. Users who want no limit have to check Anthropic's documentation for the model's actual maximum.

### Why keep the `Options map[string]any` instead of typed fields?

Different providers support different inference parameters. Even different models from the same provider may not support all parameters (e.g., some models don't support temperature). A typed struct would need to be the union of all possible parameters across all providers and models, which is impractical and would need constant updating.

The tradeoff: keys are untyped and undocumented in the Go type system. Mitigation: per-provider documentation of recognised keys.

### Why is `agent_id` per-message instead of bound to the session at creation?

Sessions are conversation containers. The per-message `agent_id` allows a single session to involve multiple agents — e.g., a "router" agent that hands off to specialist agents within the same conversation thread. This is unusual for simple single-agent workflows but becomes useful for multi-agent patterns.

The tradeoff: a session has no stable identity ("what model does this session use?"). For simple single-agent workflows this is slightly inconvenient; for multi-agent workflows it's essential.

### Why is provider/model split instead of `"provider/model"` string?

The combined string is convenient but fragile. It assumes model IDs never contain `/` (they do — e.g., `meta-llama/Llama-3.1-8B`). It also makes the data model ambiguous: when you see `"anthropic/claude-opus-4-6"`, is `"anthropic"` part of the model name or a provider identifier?

Two separate fields — `provider: "anthropic"`, `model: "claude-opus-4-6"` — are unambiguous and allow each to evolve independently.

### Why does the new `fleet.Fleet` coexist with `actor.Fleet`?

`actor.Fleet` (in `actor/`) uses the lower-level actor system (mailbox-based). It was the original implementation and is kept for existing consumers. The new `fleet.Fleet` (in `fleet/`) is a simpler, higher-level API that doesn't require understanding actors. It is the recommended API for straightforward fan-out patterns.

When formations are implemented, they will use the `actor` package directly to build dynamic actor graphs. The `fleet` package is then the "easy mode" abstraction on top.

### Why is `ToolCallResult.ToolName` the call ID, not the tool name?

The tool_result content block that gets sent back to the model must reference the `ID` of the corresponding tool_use block (e.g., `"toulu_abc123"`), not the tool name. Storing the call ID on `ToolCallResult.ToolName` makes building that block trivial for the caller. The human-readable tool name is available from the registry or from the original content block but is not separately persisted on the result.

---

## Open Questions / Future Work

1. **Formations runtime** — the `Formation` storage type (with `Roles` and `Edges`) exists but there is no execution engine. Design is pending.

2. **Provider capability metadata** — no database of "what models does provider X support" or "what's the context window for claude-opus-4-6 via bedrock". Users must know this themselves. A future `models.dev`-style internal registry is planned.

3. **OpenAI provider** — not yet implemented. Trivially addable following the same pattern as `provider/anthropic`.

4. **Custom tools on the server** — the server only resolves the 7 built-in tools by name. Custom tools require the SDK. MCP or webhook-based extension is a future possibility.

5. **Max steps / turn limit** — the agentic loop has no configurable ceiling. A runaway model loops until context exhaustion. Adding `WithMaxSteps(n)` to `session.Session` and returning `ErrMaxSteps` is a near-term improvement.

6. **Fleet streaming with persistence** — `RunStream` emits results but doesn't persist them. If the connection drops mid-flight, results are lost. A durable run model (like a job queue) is future work.

7. **Session-level tool overrides** — tools are set on the agent, not the session. You can't add a one-off tool for a specific conversation without creating a new agent. Per-session tool injection is a possible future addition.
