---
title: "Introduction"
draft: false
order: 1
---

# Wingman Documentation

Wingman is an open-source agent harness for building AI-powered clients and workflows. Run it as a local service, drive it over HTTP, embed the Go runtime directly, or extend it with plugins.

Wingman is not tied to one interface. A Wingman client can be a web app, terminal UI, editor extension, internal tool, automation script, eval harness, or anything else that can call an HTTP API.

## Why Wingman Exists

Most agent tools ship as a finished application first and an extension surface second. Wingman starts one layer lower: the harness is the product.

Wingman gives builders:

- A local agent server with persistent agents, sessions, messages, and provider auth.
- A provider-neutral model layer for OpenAI, Anthropic, and OpenCode Zen routes.
- A session API for persistent conversations and ephemeral in-memory runs.
- A plugin model for tools, hooks, context transforms, event sinks, and custom content.
- A small Go core that can be embedded, customized, and shipped as a single binary.

## The First Path

If you are new to Wingman, follow this path:

1. [Quick Start](/start-here/quickstart): run the server and send the first message.
2. [Configure Providers](/use-wingman/configure-providers): store your model provider credentials.
3. [Run Sessions](/concepts/sessions): understand persistent and ephemeral session flows.
4. [HTTP API Basics](/build-clients/http-api-basics): call Wingman from your own client.
5. [Plugins](/concepts/plugins): extend Wingman with tools and lifecycle behavior.

## Core Concepts

- **Agent:** reusable instructions, tools, model default, and options.
- **Session:** runtime record that holds message history and executes turns.
- **Tool:** callable function the model can use during a session.
- **Plugin:** session-scoped extension that contributes tools, hooks, event sinks, or custom content.
- **Client:** any app or integration that consumes the Wingman HTTP API.

## Influences

Wingman's docs and design borrow useful patterns from projects like [OpenCode](https://opencode.ai/), [Pi](https://pi.dev), [Shelley](https://github.com/boldsoftware/shelley), and [Vercel's open agents work](https://open-agents.dev/). The difference is the product boundary: Wingman is the harness you build on, not only the final client you use.
