---
title: "Plugin Capabilities"
description: "Extension methods for Go and external plugins."
---

# Plugin Capabilities

Go plugins add tools, hooks, actions, and custom message parts to embedded sessions.
A hook is a function called at a session event.
External RPC plugins support tools, progress, cancellation, and health reports. They do not support session hooks or actions.

## Go registration methods

Register extensions through `plugin.Registry` during `Activate`:

| Method                                | Purpose                                             |
| ------------------------------------- | --------------------------------------------------- |
| `RegisterBeforeRun`                   | Supply initial history before a run.                |
| `RegisterAfterRun`                    | Observe the run result and failure.                 |
| `RegisterTransformHistory`            | Replace active history before a model step.         |
| `RegisterTransformContext`            | Change messages for one model request.              |
| `RegisterTransformToolDefs`           | Change tool definitions for one model request.      |
| `RegisterTransformParams`             | Change model request parameters.                    |
| `RegisterBeforeToolCall`              | Change, deny, or skip a tool call.                  |
| `RegisterAfterToolCall`               | Observe or change a tool result.                    |
| `RegisterTool`                        | Add a tool.                                         |
| `RegisterSink`, `RegisterSinkTimeout` | Observe events with a bounded wait.                 |
| `RegisterPart`                        | Add a custom message-part decoder.                  |
| `RegisterContextBoundary`             | Mark a message that replaces earlier model context. |
| `RegisterAction`                      | Add a named session operation.                      |

Hooks run in activation order. Each transform receives the previous hook's output.
`TransformHistory` currently ignores a returned replacement when its message count is unchanged.
Use `TransformContext` for request-only changes that keep the same count.

## Events and cleanup

An event sink receives session events. The default dispatch wait is one second.
`RegisterSinkTimeout` accepts another positive timeout. Only one callback per sink runs at a time.
While a callback remains blocked, that sink drops later events. It is not a reliable event archive.

`Activate` can return a cleanup function. `Session.Close` waits for active work, then releases plugin resources.
If replacement activation fails, the existing plugins remain active.

## History and actions

A context boundary marks saved content that replaces earlier model context.
Register only one boundary type across the active plugin set. Its message must contain all replacement context.
The compaction plugin uses this mechanism for summaries.

`Session.History`, hooks, and action handlers receive only the active history range.
Read the store for earlier messages. Context boundaries do not delete the saved transcript.

Action IDs start with the plugin name followed by a dot. Each action needs a handler and a unique command alias.
Emit action-created messages through `ActionInfo.Sink` so Wingman saves and publishes them.
Interrupted actions do not resume automatically after a restart.
