---
title: "Introduction"
group: "Wingman"
order: 0
---
# Wingman

Wingman is a highly performant agent orchestration framework written in Go. It provides opinionated primitives for building, running, and scaling LLM agents.

## Two ways to use it

**HTTP Server** — A batteries included REST API designed to make it easy to interface with SOTA agent orchestration without leaving your language of choice.

**Go SDK** — For more granular control, you can use the Wingman Go SDK for full control over storage, providers, context, and execution flow. Good for embedding agents into a Go application or building something the server doesn't support out of the box.

## Core primitives

- **Provider** — A configured client for a specific model API (Anthropic, Ollama, etc.). Owns the model ID, API key, and inference parameters.
- **Agent** — A stateless template: instructions, tools, output schema, and a provider. Defines how to handle a unit of work.
- **Session** — A stateful container that holds conversation history and runs the agent loop. Send a message; it handles tool calls and multi-step inference until a final response is produced.
- **Fleet** — A pool of agent workers that process tasks concurrently.
- **Formation** — A directed graph of agents that pass work between roles (inspired by the actor framework).

## For more info

- [Architecture](https://wingman.actor/docs/architecture)
- [Server](https://wingman.actor/docs/server)
- [SDK](https://wingman.actor/docs/sdk)
