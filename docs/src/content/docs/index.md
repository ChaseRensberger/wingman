---
title: "Introduction"
group: "Overview"
draft: false
order: 1
---

# Wingman

Wingman is a collection of tools designed to make it as easy as possible to build llm enabled products and features. I built it because I wanted an agent harness that was performant, portable, and client agnostic.

## Products

**WingModels** - A provider agnostic (llm) model api
**WingHarness** - The agent harness

Wingman is a self-hostable, airgap-friendly agent runtime written in Go. It gives you a small set of composable primitives for building agent systems and ships them as two products that share one repository:

- **`wingmodels`** — provider-agnostic model interface, parts/messages, AI SDK v3 streaming, and built-in providers.
- **`wingharness`** — the inference loop, sessions, tools, lifecycle hooks, plugins, storage, and HTTP server.

You can use Wingman in two ways:

- **Go SDK** — drive sessions in-process with `session.New(...)` and integrate them directly into a Go program.
- **HTTP server** — run `wingman serve` and operate over JSON APIs against persisted agents and sessions, with SSE for streaming.

## Core mental model

Wingman is built around four primitives:

- **Provider** — a `wingmodels.Model` implementation that performs inference for a specific backend.
- **Session** — a stateful conversation that owns history and runs the inference loop.
- **Tool** — a capability the model may invoke during a session (bash, read, write, custom).
- **Plugin** — an opt-in bundle of hooks, tools, sinks, and Part types installed into a session at construction time.

The HTTP server adds two persisted concepts on top:

- **Agent** — a stored configuration record (provider, model, options, instructions, tool set) that the server uses to materialize a session at request time.
- **Storage** — a SQLite-backed `Store` that persists agents, sessions, message history, and provider credentials.

## Design anchors

- **Small core, opt-in extensions.** The loop has one extension point per seam. Plugins compose into those seams; nothing is installed by default.
- **Append-only history.** Compaction never deletes — it appends a marker that a read-side hook uses to elide stale context for the model. Storage round-trips every byte.
- **AI SDK v3 wire format.** Streaming events follow Vercel AI SDK v3 `LanguageModelV3StreamPart` shapes (with two additions: `FinishPart` carries the assembled message, and a `FinishReasonAborted` value).
- **Capability-based design.** Plugins ship their own hooks, sinks, tools, and Part types together; the core never imports plugin packages.

## Next steps

- [Architecture](./architecture) for the system design and runtime model
- [Quickstart](./getting-started) to build and run your first session
- [SDK](./sdk) for embedding Wingman in Go
- [Server](./server) for running the HTTP service
- [Sessions](./wingharness/sessions), [Streaming](./wingharness/streaming), [Plugins](./wingharness/plugins) for the core concepts
- [API](./api) for an endpoint-level reference
