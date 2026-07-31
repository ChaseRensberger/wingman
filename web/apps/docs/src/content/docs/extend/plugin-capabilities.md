---
title: "Plugin Capabilities"
description: "Supported extension surfaces for Go and RPC plugins."
group: "Reference"
order: 1004
---

# Plugin Capabilities

Wingman has one plugin model with two authoring paths:

- Go plugins for typed lifecycle extensions in embedded applications or custom binaries.
- RPC plugins for out-of-process extensions loaded by the stock server.

## Capability Matrix

| Capability | Go plugin | RPC plugin |
|---|---:|---:|
| Custom tools | yes | yes |
| `BeforeRun` | yes | no |
| `AfterRun` | yes | no |
| `TransformHistory` | yes | no |
| `TransformContext` | yes | no |
| `TransformToolDefs` | yes | no |
| `TransformParams` | yes | no |
| `BeforeToolCall` | yes | no |
| `AfterToolCall` | yes | no |
| Event sink | yes | no |
| Custom message-part decoder | yes | no |
| External process isolation | no | yes |
| Works with stock `wingman serve` | no | yes |

## Go Plugin Hooks

Go plugins register hooks with `plugin.Registry`.

| Registry method | Purpose |
|---|---|
| `RegisterBeforeRun` | Observe or prepend messages before a run starts. |
| `RegisterAfterRun` | Observe run completion, including error paths. |
| `RegisterTransformHistory` | Rewrite durable loop history before a turn. |
| `RegisterTransformContext` | Rewrite model-facing context for one turn. |
| `RegisterTransformToolDefs` | Rewrite tool definitions for one turn. |
| `RegisterTransformParams` | Rewrite request parameters for one turn. |
| `RegisterBeforeToolCall` | Mutate, deny, or skip a tool call. |
| `RegisterAfterToolCall` | Observe or rewrite a tool result. |
| `RegisterSink` | Receive every session event. |
| `RegisterSinkTimeout` | Receive events with an explicit positive callback timeout. |
| `RegisterTool` | Add a tool to the session. |
| `RegisterPart` | Register a custom message-part decoder. |

Hooks compose in activation order. Transform hooks receive the previous hook's
output. Runtime hooks, tools, and sinks belong to one immutable session
generation. Custom part decoders are rebuilt from the built-in decoder base and
are scoped to that generation.

`Activate` may return context-aware cleanup. `Session.SetPlugins` stages
replacement, waits for active work before swapping, and cleans the previous
generation in reverse order. `Session.Close` performs terminal cleanup. Sink
dispatch waits one second by default, permits one callback in flight, and drops
events while a timed-out callback remains blocked.

## RPC Plugin Support

RPC plugins declare tools in a manifest and implement `tool.execute` over stdio JSON-RPC.

The RPC protocol page documents the tool execution support exposed by the stock server.
