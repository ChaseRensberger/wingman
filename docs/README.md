# Wingman Documentation

Wingman is a highly performant agent orchestration framework written in pure Go. This project is heavily inspired by [OpenCode](https://opencode.ai) and specifically OpenCode's client & server approach to running agents. I wanted to build my own version of OpenCode's server but with improved performance

## Philosophy

**Agents as Actors.** Wingman treats agents as independent actors with their own message queues. Each agent processes work from its inbox, executes tools, and produces responses. This model enables natural concurrency, horizontal scaling, and clear separation of concerns.

**Language Agnostic.** While Wingman ships with a production-ready HTTP server, the underlying SDK is fully independent. Use the HTTP server for quick integration from any language, or import the Go packages directly for maximum control over storage, providers, context, and execution flow.
