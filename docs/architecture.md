---
title: "Architecture"
group: "Wingman"
order: 1
draft: true
---

Wingman's design decisions are likely imperfect and will continue to evolve with the project but below serves as my best effort towards a semi-comprehensive description of why I decided to do what and my vision for how everything works together.

## Core Building Blocks

1. **Provider** — A typed runtime that knows how to talk to a specific model generation API. It owns connection details (like a base URL) and inference parameters (model, max tokens, temperature, ...). Each provider package (`anthropic`, `ollama`, ...) exports its own `Config` struct so you get full type safety for provider specific options (at least when using the Wingman SDK). *Providers* implement a common interface (`RunInference`, `StreamInference`) so that *Agents* are able to swap them in and out as needed.

2. **Agent** — A stateless template that is designed to handle some unit of work. In Wingman this means a name, instructions, a set of tools, an output schema, and a *Provider* (to actually fufill that unit of work when needed). Originally I had the *Provider* disconnected from the *Agent* and it wasn't until you constructed a *Session* that the two were used together but I couldn't help but feel like the two were fundamentally linked (a creative writing agent might need a different temperature than a transaction categorizer). 

3. **Session** — A stateful container that maintains conversation history and executes the agent loop. It takes an agent and an optional working directory, then handles the run/stream cycle: send messages, process tool calls, accumulate history, repeat until the model produces a final response.

In the SDK, this looks like:

```go
p := anthropic.New(anthropic.Config{
    Model:     "claude-sonnet-4-5",
    MaxTokens: 4096,
})

a := agent.New("Summarizer",
    agent.WithInstructions("Summarize text concisely."),
    agent.WithProvider(p),
)

s := session.New(session.WithAgent(a))
result, _ := s.Run(ctx, "Summarize this article...")
```

In the HTTP API, the provider config is a JSON object on the agent:

```json
{
  "name": "Summarizer",
  "instructions": "Summarize text concisely.",
  "provider": {
    "id": "anthropic",
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096
  }
}
```

The server reads the `provider.id`, looks up API credentials from the auth store, and constructs the typed provider instance at inference time.

## Provider Interface

The provider interface is intentionally minimal:

```go
type Provider interface {
    RunInference(ctx context.Context, req WingmanInferenceRequest) (*WingmanInferenceResponse, error)
    StreamInference(ctx context.Context, req WingmanInferenceRequest) (Stream, error)
}
```

Each provider translates Wingman's types into provider-specific API calls. This is where model differences get absorbed — Anthropic uses `max_tokens`, Ollama uses `options.num_predict`, but the rest of the system doesn't care. The tradeoff is that adding a new provider means implementing this translation, but it keeps the core dependency-free.

## Actor Model

Wingman uses a lightweight actor system for concurrent execution. An `AgentActor` wraps an agent, receives work messages, creates a session, runs inference, and sends results to a collector. A `Fleet` spawns multiple agent actors and distributes work across them with round-robin scheduling.

This is intentionally simple right now — no supervision trees, no mailbox persistence, no distributed actors. The actor model gives us clean concurrency semantics (no shared state, message-passing only) without the complexity of a full framework.

## HTTP Server

The server is a thin layer over the same primitives the SDK uses. It adds SQLite persistence for agents, sessions, fleets, and formations, plus an auth store for provider credentials. When a message comes in, the server loads the agent from the database, constructs a provider from the agent's config + stored credentials, builds a session, and runs the agent loop — the same flow as the SDK, just with persistence and HTTP transport.
