# Wingman

Wingman is a highly performant agent orchestration framework written in pure Go. This project is heavily inspired by [OpenCode](https://opencode.ai) and specifically OpenCode's client & server approach to running agents. I wanted to build my own version of OpenCode's server but with improved performance and opinionated decisions about how agents should be run/composed together.

## Installation

## Wingman's Philosophy

**Agents as Actors.** Wingman treats agents as independent actors with their own message queues. Each agent processes work from its inbox, executes tools, and produces responses. This model enables natural concurrency, horizontal scaling, and clear separation of concerns.

**Language Agnostic.** The primary way to use Wingman is through its HTTP server. It is batteries included (stores data in sqlite3) and performant (built on [chi](https://github.com/go-chi/chi)). If you long for more control over the *batteries* though, there is the underlying Go SDK that is fully independent and easy to use. Use the HTTP server for quick integration from any language, or import the Go packages directly for maximum control over storage, providers, context, and execution flow.

**Concurrency** - Wingman is built in go for a reason. Work that can be done concurrently should be.

## Documentation

Check out the [docs directory](./docs) for more detailed information about Wingman's primitives and architecture.
