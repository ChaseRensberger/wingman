---
title: "Plugin Capabilities"
description: "Supported extension surfaces for Go and RPC plugins."
group: "Reference"
order: 1004
---

# Plugin Capabilities

Wingman has one plugin model with two ways to write plugins:

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
| `RegisterAfterRun` | Observe run completion, including paths with errors. |
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

Hooks run in activation order. Transform hooks receive the output from the previous hook.
`Activate` can return cleanup that uses a context. By default, sink dispatch waits one second.
It drops events while a callback remains blocked.

## RPC Plugin Support

RPC plugin manifests contain only bootstrap identity, command, and configuration.
The process negotiates protocol version 1 through `plugin.initialize`.
It returns its authoritative identity, capabilities, and tool contributions.
Tool calls can run concurrently. They receive session, run, agent, call, message, part, model-call, and working-directory identity.

RPC plugins can negotiate request cancellation, progress notifications, and
health checks.

Protocol version 1 supports tool contributions only.

The RPC protocol page defines the wire contract for the stock server.
