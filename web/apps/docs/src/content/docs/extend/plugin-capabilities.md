---
title: "Plugin Capabilities"
description: "Hooks and tools supported by Go and RPC plugins."
group: "Reference"
order: 1004
---

# Plugin Capabilities

Go plugins add tools and session hooks in embedded applications or custom binaries.
RPC plugins add tools through external programs loaded by `wingman serve`.

## Capability Matrix

| Capability                       | Go plugin | RPC plugin |
| -------------------------------- | --------: | ---------: |
| Custom tools                     |       yes |        yes |
| `BeforeRun`                      |       yes |         no |
| `AfterRun`                       |       yes |         no |
| `TransformHistory`               |       yes |         no |
| `TransformContext`               |       yes |         no |
| `TransformToolDefs`              |       yes |         no |
| `TransformParams`                |       yes |         no |
| `BeforeToolCall`                 |       yes |         no |
| `AfterToolCall`                  |       yes |         no |
| Event sink                       |       yes |         no |
| Custom message-part decoder      |       yes |         no |
| External process isolation       |        no |        yes |
| Works with stock `wingman serve` |        no |        yes |

## Go Plugin Hooks

Go plugins register hooks with `plugin.Registry`.

| Registry method             | Purpose                                                    |
| --------------------------- | ---------------------------------------------------------- |
| `RegisterBeforeRun`         | Observe or prepend messages before a run starts.           |
| `RegisterAfterRun`          | Observe run completion, including paths with errors.       |
| `RegisterTransformHistory`  | Rewrite saved history before a turn.                       |
| `RegisterTransformContext`  | Rewrite the messages sent to the model for one turn.       |
| `RegisterTransformToolDefs` | Rewrite tool definitions for one turn.                     |
| `RegisterTransformParams`   | Rewrite request parameters for one turn.                   |
| `RegisterBeforeToolCall`    | Mutate, deny, or skip a tool call.                         |
| `RegisterAfterToolCall`     | Observe or rewrite a tool result.                          |
| `RegisterSink`              | Receive every session event.                               |
| `RegisterSinkTimeout`       | Receive events with an explicit positive callback timeout. |
| `RegisterTool`              | Add a tool to the session.                                 |
| `RegisterPart`              | Register a custom message-part decoder.                    |
| `RegisterContextBoundary`   | Declare a part that replaces earlier execution context.    |

Hooks run in activation order. Transform hooks receive the output from the previous hook.
`Activate` can return cleanup that uses a context. By default, sink dispatch waits one second.
It drops events while a callback remains blocked.

A context boundary is a saved message that replaces earlier context.
Register one boundary part type per plugin generation. Its message must contain all context needed to replace the earlier messages.
The compaction plugin registers its marker as this boundary. Sessions load and retain the active range from the latest completed boundary.
`Session.History()` and action handlers receive that active range. Use stored history for earlier messages.
Transform hooks also receive active history. Do not rely on them to read the complete transcript.

## RPC Plugin Support

RPC manifests specify the program and its configuration. The program returns its identity and tools during `plugin.initialize`.
Protocol version 1 supports tools, cancellation, progress, and health checks, but not session hooks.
See [RPC Plugin Protocol](/extend/rpc-plugin-protocol) for request and response fields.
