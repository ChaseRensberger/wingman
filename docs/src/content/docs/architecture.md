---
title: "Architecture"
group: "Overview"
draft: false
order: 2
---

# Architecture

Wingman is intentionally built from a small number of packages that can be used independently or composed behind the HTTP server.

## Two products, one repo

The repository is a single Go module split into two products:

- **`wingmodels/`** is the model layer. It defines `Model`, `Message`, `Part`, `StreamPart`, and `Usage`, and ships built-in providers under `wingmodels/providers/{anthropic,ollama}` plus a small `provider.Registry`.
- **`wingagent/`** is the agent layer. It contains the `loop`, `session`, `tool`, `plugin`, `storage`, and `server` packages. Everything in `wingagent` is built on top of `wingmodels` and never the reverse.

The HTTP server (`wingagent/server`) is a thin transport layer over the same primitives the SDK exposes.

## Layered design

```
+--------------------+     wingagent/server (HTTP + SSE)
+--------------------+     wingagent/storage (SQLite)
+--------------------+     wingagent/session  (state + sinks)
+--------------------+     wingagent/loop     (agentic loop, hooks, events)
+--------------------+     wingagent/plugin   (Plugin / Registry)
+--------------------+     wingagent/tool     (built-ins + Tool interface)
+--------------------+     wingmodels/providers (Anthropic, Ollama, Registry)
+--------------------+     wingmodels         (Model, Message, Part, StreamPart)
```

Lower layers never import higher ones.

## The `wingmodels` package

`wingmodels` is the shared contract. The `Model` interface is small:

```go
type Model interface {
    Info() ModelInfo
    CountTokens(ctx context.Context, msgs []Message) (int, error)
    GenerateText(ctx context.Context, req InferenceRequest) (*InferenceResponse, error)
    StreamText(ctx context.Context, req InferenceRequest) (Stream, error)
}
```

A `Message` is a role plus a list of `Part`s. `Part` is a discriminated union sealed to the `wingmodels` package; plugins extend it through an open registry by registering a discriminator string and decoder, and serializing payloads as `OpaquePart` so the union stays sealed. See [Parts](./parts).

`StreamPart` mirrors Vercel AI SDK v3 `LanguageModelV3StreamPart` exactly. Wingman adds two things on top of the AI SDK enum: `FinishPart` carries the assembled `*Message`, and `FinishReasonAborted` exists alongside the standard reasons. See [Streaming](./streaming).

## The `wingagent/loop` package

The loop is the agentic kernel. One call to `loop.Run` drives a sequence of turns:

1. Append the user message to the running history.
2. Run `BeforeStep` (may persistently rewrite history) and `TransformContext` (may rewrite the per-turn slice without persisting).
3. Stream from the model.
4. Append the assistant message; if it includes tool calls, execute them (parallel by default) and append tool results.
5. Repeat until an assistant turn produces no tool calls, `MaxSteps` is reached, or context is cancelled.

Hooks are a struct of optional functions: `BeforeIteration`, `AfterIteration`, `BeforeStep`, `TransformSystem`, `TransformContext`, `BeforeToolCall`, `AfterToolCall`. There is exactly one of each. Plugins compose into those seams via the plugin registry. See [Lifecycle hooks](./lifecycle).

The loop emits typed events on a `Sink` (`IterationStartEvent`, `MessageEvent`, `ToolExecutionStartEvent`/`EndEvent`, `StreamPartEvent`, `ContextTransformedEvent`, `ErrorEvent`, `IterationEndEvent`). The session forwards these to whatever observers are attached.

## Sessions

A `*session.Session` is a thin stateful wrapper around the loop. It owns:

- a KSUID identifier (`ses_…`)
- a working directory passed to tool executions
- the active `Model`, system prompt, and tool registry
- the running message history
- installed plugins, optional raw hooks, and an optional `MessageSink`

`Run` and `RunStream` snapshot inputs, build the plugin registry per-call, and drive `loop.Run`. After the loop returns, the session adopts the loop's terminal message slice wholesale — so plugin mutations (e.g. compaction markers) end up in history. The session exposes only `History()`, `AddMessage`, `SetHistory`, `Clear`, plus setters for model/system/tools/work-dir.

See [Sessions](./sessions).

## Plugins

Plugins are the v0.1 extension mechanism. A `Plugin` bundles hooks, sinks, tools, and Part registrations behind a single `Install(*plugin.Registry) error`. The session builds a fresh `Registry` per call and folds it into a `loop.Hooks` value, a merged tool slice, and a fan-out sink. Composition is install-order: pipeline seams chain, sinks fan out, tool name collisions resolve last-wins.

The canonical plugin is `wingagent/plugin/compaction`, which summarizes long histories into an inline marker. See [Plugins](./plugins).

## Storage

`wingagent/storage.Store` is the persistence interface. It covers agents, sessions, message history, and provider credentials. Sessions can be persisted incrementally during a run by wiring `session.WithMessageSink(store.AppendMessage)`; the server does this automatically.

IDs are KSUIDs with stable prefixes: `agt_`, `ses_`, `msg_`, `prt_`, `tlu_`. See [Storage](./storage).

## HTTP server

`wingagent/server` is a chi router with SQLite-backed persistence. It does not introduce a separate execution model: every request reconstructs the same primitives the SDK uses. For example, when a message arrives:

1. Load the session and its history.
2. Load the referenced agent definition.
3. Build the provider from `provider`/`model`/`options`, injecting stored credentials.
4. Construct a `*session.Session` with `WithMessageSink` wired to incremental storage.
5. Drive `Run` or `RunStream`; stream events over SSE if requested.
6. The session adopts the loop's final history; storage already has each message appended.

See [Server](./server) and [API](./api).
