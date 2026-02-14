---
title: "Architecture"
group: "Wingman"
order: 1
---

# Architecture

This document describes the design decisions behind Wingman. These are likely imperfect and will evolve, but what follows is the current rationale for how everything fits together.

## Core Building Blocks

Wingman has three core primitives:

1. **[Provider](/docs/providers)** — A typed runtime that knows how to talk to a specific model API. It owns connection details and inference parameters (model, max tokens, temperature). Each provider package exports its own `Config` struct for full type safety. Providers implement a common interface (`RunInference`, `StreamInference`) so agents can swap them interchangeably.

2. **[Agent](/docs/agents)** — A stateless template designed to handle a unit of work: a name, instructions, tools, an output schema, and a provider. The provider is attached directly to the agent because the two are fundamentally linked — a creative writing agent might need a different temperature than a transaction categorizer.

3. **[Session](/docs/sessions)** — A stateful container that maintains conversation history and executes the agent loop. It takes an agent and an optional working directory, then handles the run/stream cycle: send messages, process tool calls, accumulate history, repeat until the model produces a final response.

## Provider Interface

The provider interface is intentionally minimal:

```go
type Provider interface {
    RunInference(ctx context.Context, req WingmanInferenceRequest) (*WingmanInferenceResponse, error)
    StreamInference(ctx context.Context, req WingmanInferenceRequest) (Stream, error)
}
```

Each provider translates Wingman's internal types into provider-specific API calls. This is where model differences get absorbed — Anthropic uses `max_tokens`, Ollama uses `options.num_predict`, but the rest of the system doesn't care. The tradeoff is that adding a new provider means implementing this translation layer, but it keeps the core dependency-free.

## Actor Model

Wingman uses a lightweight actor system for concurrent execution. An `AgentActor` wraps an agent, receives work messages, creates a session, runs inference, and sends results to a collector. A `Fleet` spawns multiple agent actors and distributes work across them with round-robin scheduling.

This is intentionally simple — no supervision trees, no mailbox persistence, no distributed actors. The actor model gives clean concurrency semantics (no shared state, message-passing only) without the complexity of a full framework.

## HTTP Server

The [server](/docs/server) is a thin layer over the same primitives the [SDK](/docs/sdk) uses. It adds SQLite persistence for agents and sessions, plus an auth store for provider credentials. When a message comes in, the server loads the agent from the database, constructs a provider from the agent's config + stored credentials, builds a session, and runs the agent loop — the same flow as the SDK, just with persistence and HTTP transport.
