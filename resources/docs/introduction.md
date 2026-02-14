---
title: "Introduction"
group: "Wingman"
order: 0
---

If you just want to get up and running, head to the [Server](/docs/server) or [SDK](/docs/sdk) usage guides. If you're curious about what Wingman is and why it exists, read on.

## Backstory

I got the idea for Wingman back in May 2025, while playing with [OpenCode](https://opencode.ai). I liked OpenCode's approach to agents — specifically the client/server relationship between the TUI frontend and the agentic backend. If you come from the web application world like I do, this idea is nothing to write home about. Still, the idea of composing agents via the universal, language-agnostic transport layer (HTTP) felt interesting enough to pursue.

"So how is this different from OpenCode's server?" you might say. Wingman is opinionated in its relationships between [providers](/docs/providers), [agents](/docs/agents), and [sessions](/docs/sessions) while being written in Go to take advantage of built-in concurrency features for things like running agents in parallel. I've also found OpenCode's backend to be well suited for their specific use case (powering a coding application) but less flexible if you're trying to make it fit yours.

## What is Wingman?

Wingman is an agent orchestration framework written in pure Go. It provides primitives for building, running, and scaling agents that can use tools, maintain conversation state, and execute work concurrently.

At its core, Wingman treats agents as actors (inspired by [the actor model](https://en.wikipedia.org/wiki/Actor_model)) — independent units with their own message queues that process work from an inbox, execute tools, and produce responses. This enables natural concurrency, horizontal scaling, and a clear separation of concerns. For more on the design, see [Architecture](/docs/architecture).

Wingman can be used in two ways:

1. **[HTTP Server](/docs/server)** — A batteries-included REST API that stores data in SQLite while exposing agents, sessions, and providers over HTTP.
2. **[Go SDK](/docs/sdk)** — Import the primitives directly for maximum control over storage, providers, context, and execution flow.

## Why use Wingman?

If you're building something that involves language models and one or more of the following are true:

1. You don't want to manage relationships with different model providers
2. You don't want a verbose SDK (looking at you, LangChain)
3. You want to run agents in parallel with minimal overhead

## Next Steps

- **Learn the design** — [Architecture](/docs/architecture)
- **Start building** — [Server](/docs/server) or [SDK](/docs/sdk)
- **Understand the primitives** — [Providers](/docs/providers), [Agents](/docs/agents), [Sessions](/docs/sessions), [Tools](/docs/tools)
- **Browse the source** — [GitHub](https://github.com/chaserensberger/wingman)
