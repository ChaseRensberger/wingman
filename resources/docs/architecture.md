---
title: "Architecture"
group: "Overview"
draft: false
order: 2
---

# Architecture

Wingman is intentionally built from a small number of packages that can be used independently or composed into larger orchestration flows.

## Layered design

At a high level, the system stacks like this:

1. **`core/`** defines the canonical shared types and interfaces.
2. **Providers** implement inference against specific model backends.
3. **Agents** bundle configuration such as instructions, tools, and output schema.
4. **Sessions** run the agentic loop and maintain conversation history.
5. **Fleets** fan one agent template out across many concurrent tasks.
6. **Formations** coordinate larger DAG-shaped workflows across agents and fleets.

The HTTP server is a thin persistence and transport layer over those same primitives.

## The `core` package

`core/core.go` is the shared contract for the rest of the system. It defines message types, inference requests and responses, streaming events, tool definitions, and the key interfaces (`Provider`, `Stream`, and `Tool`).

Separating `core` avoids circular imports and gives the rest of the codebase one stable type system to build against.

## Providers

Providers translate Wingman's provider-agnostic `InferenceRequest` into the wire format expected by a backend such as Anthropic or Ollama.

Each provider implements the same minimal interface:

```go
type Provider interface {
    RunInference(ctx context.Context, req core.InferenceRequest) (*core.InferenceResponse, error)
    StreamInference(ctx context.Context, req core.InferenceRequest) (core.Stream, error)
}
```

This keeps the rest of the runtime independent from provider-specific APIs.

## Agents

An agent is a reusable configuration bundle. It does not hold conversation state. Instead, it defines how work should be performed:

- system instructions
- provider and model
- tool set
- optional output schema

Because agents are stateless templates, the same agent can be reused across many sessions and fleet workers.

## Sessions and the agentic loop

A session is where execution happens. It owns message history, optional working directory, and the tool-calling loop.

The loop is the same in blocking and streaming modes:

1. Append the user message to history.
2. Build an inference request from history, tools, instructions, and schema.
3. Call the provider.
4. Append the assistant response.
5. If the model requested tools, execute them, append tool results, and repeat.
6. Return the final result when the model stops.

This makes the session the main bridge between static agent configuration and live runtime behavior.

## Fleets

Fleets are Wingman's primary fan-out primitive. A fleet takes one agent template and a list of tasks, then runs those tasks concurrently with optional worker limits.

Each task can override:

- `message`
- `work_dir`
- `instructions`
- `data`

This makes fleets useful for parallel exploration, batch analysis, and map-reduce style agent workflows.

## Formations

Formations are a declarative DAG runtime exposed by the server. They persist workflow definitions, then execute runs ephemerally on demand.

Current node kinds are:

- `agent`
- `fleet`
- `join`

Edges map structured outputs from one node into the inputs of downstream nodes. In practice, formations sit above agents, sessions, and fleets and orchestrate them into larger workflows.

## HTTP server

The server does not introduce a separate execution model. It persists definitions in SQLite, reconstructs live runtime objects at request time, and executes the same primitives the SDK exposes.

For example, when a session message arrives the server:

1. loads the stored session history
2. loads the referenced agent definition
3. reconstructs the provider from `provider`, `model`, and `options`
4. injects any stored provider credentials
5. runs the session loop
6. persists the updated history back to SQLite

The same pattern applies to fleets and formations: persisted configuration, ephemeral execution.

## Actor system

Wingman also includes a lightweight actor system in `actor/`. It remains available for lower-level concurrency and compatibility with older flows, but it is no longer the main mental model for most users.

If you are building new orchestration on Wingman, start with `agent`, `session`, `fleet`, and `formation` concepts first.
