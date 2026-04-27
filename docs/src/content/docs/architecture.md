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

`StreamPart` mirrors Vercel AI SDK v3 `LanguageModelV3StreamPart` exactly. Wingman adds three things on top of the AI SDK enum: `FinishPart` carries the assembled `*Message`, `FinishReasonAborted` exists alongside the standard reasons, and the assembled `*Message` is stamped with both `FinishReason` and a `MessageOrigin` (`Provider`, `API`, `ModelID`) so downstream code can reason about what produced each turn. See [Streaming](./streaming).

## Mid-session model switching

Wingman expects callers to swap the active model mid-session (different turns may run on different providers entirely). Three things make that safe:

- **`MessageOrigin` on every assistant message.** Providers stamp it on the assembled message inside `FinishPart`. `MessageOrigin.SameModel` lets the next provider tell whether the prior turn came from the exact same wire API + model.
- **`wingmodels/transform`.** Each provider calls `transform.Apply(messages, target)` at the top of its `Stream` implementation. The pure function drops failed-turn assistant messages (`FinishReason` `error`/`aborted`) and their orphan tool calls, drops reasoning blocks unless the next call is `SameModel`, and downgrades image parts to a text placeholder when the target model can't accept them (`Capabilities.Images == false`). The loop and the session never see this rewriting — it lives entirely inside the provider boundary.
- **`ModelInfo.Capabilities`.** Providers populate a `ModelCapabilities` struct (`Tools`, `Images`, `Reasoning`, `StructuredOutput`) from catalog data at construction time. The transform layer reads `Capabilities.Images`; the agent loop can inspect the others to decide which features to use.

## The `wingagent/loop` package

The loop is the agentic kernel. One call to `loop.Run` drives a sequence of turns:

1. Run `BeforeRun` once to seed the loop's initial message history (the storage plugin uses this to load prior turns from disk).
2. Run `BeforeStep` (may persistently rewrite history) and `TransformContext` (may rewrite the per-turn slice without persisting).
3. Stream from the model.
4. Append the assistant message; if it includes tool calls, execute them (parallel by default) and append tool results.
5. Repeat until an assistant turn produces no tool calls, `MaxSteps` is reached, or context is cancelled.

Hooks are a struct of optional functions: `BeforeRun`, `BeforeIteration`, `AfterIteration`, `BeforeStep`, `TransformSystem`, `TransformContext`, `BeforeToolCall`, `AfterToolCall`. There is exactly one of each. Plugins compose into those seams via the plugin registry. See [Lifecycle hooks](./lifecycle).

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

Plugins are the v0.1 extension mechanism. A `Plugin` bundles hooks, sinks, tools, and Part registrations behind a single `Install(*plugin.Registry) error`. The session builds a fresh `Registry` per call and folds it into a `loop.Hooks` value, a merged tool slice, and a fan-out sink. Composition is install-order: pipeline seams chain, sinks fan out, tool name collisions resolve last-wins. Plugin names must be unique within a session.

Two canonical plugins ship in-tree:

- `wingagent/plugin/compaction` — summarizes long histories into an inline marker, demonstrating the two-seam (`BeforeStep` + `TransformContext`) pattern.
- `wingagent/storage` — packages persistence as a capability: a `BeforeRun` hook loads prior history and a sink appends new messages. Used by the HTTP server to wire sessions to SQLite without the loop or session core importing storage.

See [Plugins](./plugins).

## Storage

`wingagent/storage.Store` is the persistence interface. It covers agents, sessions, message history, and provider credentials. The recommended way to give a session both load and save is `session.WithPlugin(storage.NewPlugin(store, sessionID))`; the lower-level `session.WithMessageSink` remains supported for ad-hoc message observation.

IDs are KSUIDs with stable prefixes: `agt_`, `ses_`, `msg_`, `prt_`, `tlu_`. See [Storage](./storage).

## HTTP server

`wingagent/server` is a chi router with SQLite-backed persistence. It does not introduce a separate execution model: every request reconstructs the same primitives the SDK uses. For example, when a message arrives:

1. Load the agent definition and the session record.
2. Build the provider from `provider`/`model`/`options`, injecting stored credentials.
3. Construct a `*session.Session` with `session.WithPlugin(storage.NewPlugin(store, sess.ID))`. The plugin's `BeforeRun` rehydrates history; its sink appends new messages as they land.
4. Drive `Run` or `RunStream`; stream events over SSE if requested.
5. Return the response. Storage already has each message appended.

See [Server](./server) and [API](./api).
