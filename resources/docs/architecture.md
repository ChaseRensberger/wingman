---
title: "Architecture"
group: "Wingman"
order: 1
---
# Architecture

## Core primitives

### Provider

A provider is a configured client for a specific model API. It owns the connection details and inference parameters (model, max tokens, temperature, etc.) and implements a minimal interface:

```go
type Provider interface {
    RunInference(ctx context.Context, req WingmanInferenceRequest) (*WingmanInferenceResponse, error)
    StreamInference(ctx context.Context, req WingmanInferenceRequest) (Stream, error)
}
```

Each provider package translates Wingman's generic request/response types into the wire format for its specific API. This is where provider differences get absorbed — the rest of the system only speaks Wingman types. The tradeoff is that adding a new provider requires implementing this translation layer, but it keeps the core dependency-free.

In the SDK, providers are constructed with typed config structs:

```go
p := anthropic.New(anthropic.Config{
    Model:     "claude-sonnet-4-5",
    MaxTokens: 4096,
})
```

In the HTTP API, provider configuration is encoded on the agent as a `model` string (`"provider/model"`) and a free-form `options` map. The server splits the model string at the first `/` to identify the provider, looks up credentials from the auth store, and constructs the typed provider instance at inference time.

### Agent

An agent is a stateless template that defines how to handle a unit of work: a name, instructions (system prompt), a set of tools, an optional output schema, and a provider. The same agent instance can be used by many concurrent sessions.

```go
a := agent.New("Summarizer",
    agent.WithInstructions("Summarize text concisely."),
    agent.WithProvider(p),
)
```

### Session

A session is a stateful container that holds conversation history and runs the agent loop. It takes an agent and an optional working directory, then handles the full cycle: send messages, process tool calls, accumulate history, and repeat until the model produces a final response.

```go
s := session.New(session.WithAgent(a))
result, _ := s.Run(ctx, "Summarize this article...")
```

## Actor model

Wingman uses a lightweight actor system for concurrent execution. An `AgentActor` wraps an agent, receives work messages, creates a session, runs inference, and sends results to a collector. A `Fleet` spawns multiple agent actors and distributes work across them.

This is intentionally simple — no supervision trees, no mailbox persistence, no distributed actors. The actor model provides clean concurrency semantics (no shared state, message-passing only) without requiring a full framework.

## HTTP server

The server is a thin layer over the same primitives the SDK exposes. It adds SQLite persistence for agents, sessions, fleets, and formations, plus an auth store for provider credentials. When a message arrives, the server loads the agent from the database, constructs a provider from the agent's config and stored credentials, builds a session with the persisted history, and runs the agent loop — the same flow as the SDK, just with persistence and HTTP transport on top.
