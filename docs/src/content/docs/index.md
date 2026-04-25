---
title: "Introduction"
group: "Overview"
draft: false
order: 1
---

# Wingman

> Disclaimer: This project is under active development and not ready for production use. This documentation is also under active development and constantly changing.

Wingman is a self-hostable, airgap-friendly agent runtime and orchestration engine written in Go. It gives you a small set of composable primitives for building agent systems of arbitrary complexity.

You can use Wingman in two ways:

- **Go SDK** - run agents in-process and integrate them directly into a Go application.
- **HTTP server** - run `wingman serve` and interact with persisted agents, sessions, fleets, and formations over JSON APIs.

## Core mental model

Wingman is built around a small runtime model:

- **Provider** - a model backend adapter that implements inference.
- **Agent** - a reusable configuration bundle: instructions, tools, output schema, provider, and model.
- **Session** - a stateful conversation loop that manages history and tool execution.
- **Fleet** - a fan-out primitive for running one agent template across many tasks concurrently.
- **Formation** - a declarative DAG runtime for multi-step, multi-agent workflows.

These pieces are designed to compose cleanly. A provider powers an agent, a session executes that agent, fleets run many sessions in parallel, and formations orchestrate larger workflows across agents and fleets.

## Next steps

- [Architecture](./architecture) for the system design and runtime model
- [SDK](./sdk) for embedding Wingman in Go
- [Server](./server) for running the HTTP service
- [Agents](./agents), [Sessions](./sessions), [Fleets](./fleets), and [Formations](./formations) for the core concepts
- [API](./api) for an endpoint-level reference
