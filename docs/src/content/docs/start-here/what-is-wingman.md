---
title: "What Wingman Is"
description: "Understand Wingman's role as an agent harness, service, and library."
order: 2
---

# What Wingman Is

Wingman is a client-agnostic agent harness. It provides the runtime pieces needed to build an AI agent application without locking you into one UI or workflow.

You can use Wingman as:

- A local HTTP service for custom clients.
- A Go library embedded into another application.
- A plugin host for tools and lifecycle extensions.
- A model-routing layer through WingModels.
- A persistent session store for agent conversations.

## What Wingman Is Not

Wingman is not only a coding TUI, editor plugin, or hosted chat product. Those can all be built on top of Wingman, but they are clients of the harness.

This distinction matters because the core API should stay useful to many clients:

- A web UI can manage providers, agents, and sessions.
- A terminal UI can stream events into a transcript.
- An editor extension can create sessions scoped to a project.
- A script can run an ephemeral session and discard the transcript.
- An internal tool can register its own agents and tools.

## Mental Model

Think of Wingman as infrastructure for agent applications:

```text
client or app
  -> Wingman HTTP API
    -> session runtime
      -> model provider
      -> tools
      -> plugins
      -> storage
```

The default path is intentionally simple: run the server locally, configure provider auth, create an agent, create a session, send messages. The extension path is intentionally deep: write clients, embed the runtime, add tools, and hook into the lifecycle.
