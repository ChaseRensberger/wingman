---
title: "Introduction"
group: "Wingman"
order: 0
---
# Wingman

Wingman is an agent orchestration framework written in Go. It provides primitives for building, running, and scaling LLM agents that can use tools, maintain conversation history, and execute work concurrently.

## Two ways to use it

**HTTP Server** — A REST API backed by SQLite. Create agents and sessions, configure provider credentials, and send messages over HTTP. No Go required. Good for integrating agents into an existing application or service regardless of language.

**Go SDK** — Import the primitives directly for full control over storage, providers, context, and execution flow. Good for embedding agents into a Go application or building something the server doesn't support out of the box.

## Core primitives

- **Provider** — A configured client for a specific model API (Anthropic, Ollama, etc.). Owns the model ID, API key, and inference parameters.
- **Agent** — A stateless template: instructions, tools, output schema, and a provider. Defines how to handle a unit of work.
- **Session** — A stateful container that holds conversation history and runs the agent loop. Send a message; it handles tool calls and multi-step inference until a final response is produced.
- **Fleet** — A pool of agent workers that process tasks concurrently.
- **Formation** — A directed graph of agents that pass work between roles.

## When to use Wingman

- You want a simple, dependency-light way to wire up LLM agents in Go
- You need concurrent agent execution (fleets)
- You want a language-agnostic HTTP interface to an agentic backend
- You don't want to maintain provider-specific SDK integrations yourself

## Getting started

- **HTTP Server** → [Server](https://wingman.actor/docs/server)
- **Go SDK** → [SDK](https://wingman.actor/docs/sdk)
- **How it works** → [Architecture](https://wingman.actor/docs/architecture)
