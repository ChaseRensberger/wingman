---
title: "Introduction"
group: "Wingman"
order: 0
---

If you're just looking to get up and running with Wingman as fast as possible, take a look at the Usage docs. If you're curious to learn more about what this project is, continue.

Welcome to the Wingman documentation. I don't have a habit of writing very good documentation so learners beware, if you encounter a spelling mistake or something that doesn't make sense, I apologize (also feel free to put up a PR).

## Some Backstory

I got the idea for Wingman back in May 2025, while playing with [OpenCode](https://opencode.ai). I quite liked OpenCode's approach to agents, specifically the client/server relationship between the TUI frontend and the agentic backend. If you come from the web application world like I do, this idea is nothing to write home about. Still, I couldn't help but feel like the idea of composing agents via the universal language agnostic information transport layer (aka http) was interesting enough to persue.

"So how is this different then OpenCode's server then?" you might say. The short answer is that there's less cool features. The slightly longer answer is that this Wingman is opinionated in it's relationships between providers, agents, and sessions (wingman also introduces two new primitves called "fleets" and "formations") while also being that written in Go to take advantage of some of the language's built in concurrency features (as opposed to OpenCode's typescript backend) for doing things like running agents in parallel as efficiently as possible.

Also I've found OpenCode's server backend to be very well suited for their specific use case (powering a great agentic coding application) but not as flexible if you're trying to make it fit your specific use case.

## What is Wingman?/Wingman's Philosophy

Wingman is an agent orchestration framework written in pure Go. It provides primitives for building, running, and scaling agents that can use tools, maintain conversation state, and execute work concurrently.

At its core, Wingman treats agents as actors (inspired by [the actor model](https://en.wikipedia.org/wiki/Actor_model)), independent units with their own message queues that process work from an inbox, execute tools, and produce responses. This model enables natural concurrency, horizontal scaling, and a clear separation of concerns. The goal of Wingman is to build rock-solid primitives that you can compose

Wingman can be used in two ways:

1. **HTTP Server** - A batteries-included REST API that stores data in SQLite while allowing you to interact with agents, sessions, and providers over HTTP.

2. **Go SDK** - Import the primitives and helper functions directly for maximum control over storage, providers, context, and agent execution flow.

## Why use Wingman?

If you're building a feature for a project and you want to invole language models in some capacity and 1+ of the following are true:

1. Don't want to worry about maintaining relationships with different model providers
2. Don't want to deal with a vebose SDK (of something like LangChain)
3. Want to run agents in parallel (and with speed)

## Next Steps

If you're curious to learn more about how Wingman works, click on [one of the Primitive sections](https://wingman.actor/docs/providers) or check out [the source](https://github.com/chaserensberger/wingman).

If you want to get started using Wingman, head on over to [one of the Usage sections](https://wingman.actor/docs/server).





