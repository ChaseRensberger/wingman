---
title: "Introduction"
group: "Overview"
draft: false
order: 1
---

# Wingman

> Disclaimer: This project is under active development and not ready for production use. This documentation is also under active development and constantly changing.

Wingman is a self-hostable, airgap-friendly agent runtime and orchestration engine written in Go. It gives you a small set of composable primitives for building agent systems without committing to a hosted control plane.

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

## Choose your entry point

Use the SDK if you want to:

- embed Wingman directly in a Go service
- control storage, lifecycle, and orchestration yourself
- create custom tools and custom runtime behavior in-process

Use the server if you want to:

- persist agents, sessions, fleets, and formations in SQLite
- drive workflows over HTTP from any client
- use formations and server-managed auth out of the box

## What to read next

- [Architecture](./architecture) for the system design and runtime model
- [SDK](./sdk) for embedding Wingman in Go
- [Server](./server) for running the HTTP service
- [Agents](./agents), [Sessions](./sessions), [Fleets](./fleets), and [Formations](./formations) for the core concepts
- [API](./api) for endpoint-level reference
