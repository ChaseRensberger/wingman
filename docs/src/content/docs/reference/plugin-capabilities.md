---
title: "Plugin Capabilities"
description: "Supported extension surfaces for Go and RPC plugins."
group: "Reference"
order: 1004
---

# Plugin Capabilities

Wingman has one plugin model with two authoring paths:

- Go plugins for typed lifecycle extensions in embedded applications or custom binaries.
- RPC plugins for external custom tools in the stock server.

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
| `RegisterTool` | Add a tool to the session. |
| `RegisterPart` | Register a custom message-part decoder. |

Hooks compose in install order. Transform hooks receive the previous hook's output. Sinks fan out independently.

## RPC Plugin Surface

RPC plugins declare tools in a manifest and implement `tool.execute` over stdio JSON-RPC.

The RPC path is intentionally narrow: it lets users extend the stock server without compiling Go. Use Go plugins when you need lifecycle hooks, event sinks, custom part decoders, or typed access to runtime structures.

## Design Target

Wingman's long-term target is parity where transport boundaries allow it: custom tools, lifecycle events, context transforms, permission hooks, commands, and provider/model contributions should share one vocabulary across Go and RPC.

Go comes first because it defines the typed lifecycle. RPC follows once the hook contracts are proven by real Go plugins and can be exposed without weakening the model.
