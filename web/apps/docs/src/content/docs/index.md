---
title: "Introduction"
order: 1
---

# Introduction

Wingman is an open-source client-agnostic agent harness.

## Why Wingman Exists

Wingman provides the agent runtime layer without tying it to one interface. A Wingman client can be a web app, terminal UI, editor extension, internal tool, automation script, eval framework, or another application.

Wingman gives builders:

- A provider-agnostic model SDK
- A local agent server with persistent agents, sessions, messages, and provider auth.
- A session API for persistent conversations and ephemeral in-memory runs.
- A plugin model for tools, hooks, context transforms, event sinks, and custom content.
- A small Go core that can be embedded, customized, and shipped as a single binary.

## The First Path

If you are new to Wingman, follow this path:

1. [Quick Start](/docs/start-here/quickstart): run the server and send the first message.
2. [Configure Providers](/docs/configure/providers): store provider credentials and route models through gateways.
3. [Run Sessions](/docs/concepts/sessions): understand persistent and ephemeral session flows.
4. [HTTP API Basics](/docs/build-clients/http-api-basics): call Wingman from your own client.
5. [Plugins](/docs/concepts/plugins): extend Wingman with tools and lifecycle behavior.

## Core Concepts

- **Agent:** a stateless representation of behavior (specify instructions, tools, output schema, etc.)
- **Session:** a stateful execution of an agent (runtime record that holds message history and executes turns)
- **Tool:** canonical function definition the model can use during a session.
- **Plugin:** session-scoped extension that contributes tools, hooks, event sinks, or custom content.
- **Client:** any app or integration that consumes the Wingman HTTP API.
