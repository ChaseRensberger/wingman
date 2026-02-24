---
title: "Architecture"
group: "Wingman"
order: 1
---
# Architecture

## Core primitives

### Provider

A provider is a configured client for a specific model API. It translates Wingman's provider-agnostic types into the provider's wire format and implements a minimal interface:

```go
type Provider interface {
    RunInference(ctx context.Context, req core.InferenceRequest) (*core.InferenceResponse, error)
    StreamInference(ctx context.Context, req core.InferenceRequest) (core.Stream, error)
}
```

Each provider package absorbs backend-specific differences so the rest of the system only speaks core types.

In the SDK, providers use a config struct with a generic `Options map[string]any` for inference parameters:

```go
p, err := anthropic.New(anthropic.Config{
    Options: map[string]any{
        "model":      "claude-sonnet-4-5",
        "max_tokens": 4096,
    },
})
```

In the HTTP API, provider configuration is stored on the agent as separate `provider` and `model` fields plus a free-form `options` map. The server resolves credentials from SQLite and builds the provider through the registry at inference time.

### Agent

An agent is a stateless template: name, instructions (system prompt), tools, optional output schema, and provider + model. The same agent instance can be reused across many sessions.

```go
a := agent.New("Summarizer",
    agent.WithInstructions("Summarize text concisely."),
    agent.WithProvider(p),
)
```

### Session

A session is a stateful container that holds conversation history and runs the agentic loop. It takes an agent and an optional working directory, then handles the full cycle: send messages, process tool calls, accumulate history, and repeat until the model produces a final response.

```go
s := session.New(session.WithAgent(a))
result, _ := s.Run(ctx, "Summarize this article...")
```

## Actor system

Wingman includes a lightweight actor system (`actor/`) for concurrent execution. An `AgentActor` wraps an agent, receives work messages, creates a session, runs inference, and sends results to a collector. The higher-level `fleet/` package is the recommended fan-out API; the actor system is the lower-level primitive that will eventually power formations.

This system is intentionally simple — no supervision trees, no mailbox persistence, no distributed actors. It provides clean concurrency semantics (message passing, no shared state) without requiring a full framework.

## HTTP server

The server is a thin layer over the same primitives the SDK exposes. It adds SQLite persistence for agents, sessions, fleets, and formations, plus an auth store for provider credentials. When a message arrives, the server loads the agent from the database, constructs a provider from the agent's config and stored credentials, builds a session with the persisted history, and runs the agent loop — the same flow as the SDK, just with persistence and HTTP transport on top.
