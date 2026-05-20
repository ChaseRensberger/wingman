---
title: "Introduction"
draft: false
order: 1
---

# Introduction

Wingman is an open-source client-agnostic agent harness.

## Why Wingman Exists

I built Wingman because I wanted a performant agent harness that wasn't tied to any specific kind of application (like all the harnesses that are mostly just coding TUIs). What this means practically is that Wingman is not tied to one interface; a Wingman client can be a web app, terminal UI, editor extension, internal tool, automation script, eval framework, etc. Most agentic tools in this space ship as a finished application first and an extension surface second. Wingman starts one layer lower, with the idea that the harness is the product.

Wingman gives builders:

- A provider-agnostic model SDK
- A local agent server with persistent agents, sessions, messages, and provider auth.
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

- **Agent:** a stateless representation of behavior (specify instructions, tools, output schema, etc.)
- **Session:** a stateful execution of an agent (runtime record that holds message history and executes turns)
- **Tool:** canonical function definition the model can use during a session.
- **Plugin:** session-scoped extension that contributes tools, hooks, event sinks, or custom content.
- **Client:** any app or integration that consumes the Wingman HTTP API.

## Inspiration

I have taken inspiration from more projects than I can count but to name a few (that I reference daily):

- [Pi](https://pi.dev)
- [OpenCode](https://opencode.ai/)
- [Shelley](https://github.com/boldsoftware/shelley)
- [Vercel's entire AI stack](https://open-agents.dev/)

and many many more...

## Notes

- Perhaps unsurprisingly, some non-negligible percentage of this project was built with coding agents and eventually Wingman itself.

- Wingman is written in golang and can thus has serveral features (like [WingModels](/core/wingmodels)) that can be imported and used as an SDK for your go application. This documentation is focused largely on the complete Wingman runtime but I suspect there will be people that want to use just the SDK without all the extra fluff. I will update the documentation in time to streamline that process.
