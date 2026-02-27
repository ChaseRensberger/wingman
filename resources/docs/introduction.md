---
title: "Introduction"
group: "Wingman"
draft: true
order: 1
---
# Wingman

Wingman is a self-hostable, airgap-friendly agent orchestration engine written in Go. It provides a small set of primitives for building, running, and scaling LLM agents.

## Two ways to use it

**Go SDK** — Run agents in-process. You own the persistence layer (or skip it entirely). Ideal for embedding in Go apps.

**HTTP Server** — Run `wingman serve`. Agents, sessions, and fleets are persisted in SQLite. Any HTTP client can talk to it.

## Core primitives

- **Provider** — Translates Wingman's provider-agnostic request into a specific model API.
- **Agent** — A stateless template: instructions, tools, output schema, and a provider + model.
- **Session** — A stateful container that holds conversation history and runs the agentic loop.
- **Fleet** — A fan-out primitive for running many tasks concurrently.
- **Formation** — A declarative DAG runtime for multi-agent workflows.
- **Actor system** — A low-level mailbox-based runtime used by concurrent primitives.

## For more info

- [Architecture](https://wingman.actor/docs/architecture)
- [Server](https://wingman.actor/docs/server)
- [SDK](https://wingman.actor/docs/sdk)
