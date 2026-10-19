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
| `RegisterBeforeComplete`              | Accept an answer or request another iteration.      |
| `RegisterTransformHistory`            | Replace active history before a model step.         |
| `RegisterTransformContext`            | Change messages for one model request.              |
| `RegisterTransformSystem`             | Change instructions for one model request.          |
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

## System instructions

`RegisterTransformSystem` receives `run.TransformSystemInfo` before each model call.
Its `System` field contains the current instructions. The other fields provide the current model, directory, tools, and permissions.
Return the complete replacement instruction text. Treat the supplied model, tools, and permission data as read-only values.
Direct users of `run.Hooks.TransformSystem` use the same input type.

The `plugins/orientation` package supplies general environment, model, reference, and MCP context.
The `plugins/wingcode` package supplies coding guidance and Git context.
For embedded sessions, pass their plugins to `session.WithPlugin`:

```go
session.WithPlugin(
    orientation.New(orientation.Config{}),
    wingcode.New(),
)
```

The server includes orientation automatically. The standard application also enables compaction and `wingcode` for every agent.
Server applications register shared plugins through `server.Config.Plugins`:

```go
server.PluginRegistration{
    ID: "wingcode",
    Enabled: true,
    Factory: func() plugin.Plugin { return wingcode.New() },
}
```

Use the plugin's `Name()` as its registration ID. Each enabled factory supplies a fresh plugin for session activation.
Set `Enabled` to `false` to keep a registration visible without activating it.
`GET /plugins` reads these registrations without creating plugin instances or loading project directories.

## Completion validation

A candidate is an answer that awaits acceptance. Register `BeforeComplete` to accept it or supply feedback for another iteration:

```go
registry.RegisterBeforeComplete(func(ctx context.Context, info run.CompletionInfo) (run.CompletionDecision, error) {
    if missingTest(info.Candidate) {
        return run.CompletionDecision{Action: "continue", Feedback: "Add the missing regression test."}, nil
    }
    return run.CompletionDecision{Action: "accept"}, nil
})
```

`missingTest` represents your application-specific validation. Return an error if the validator fails to run.
Validators run in plugin order. The first continuation stops the chain. All validators must accept the candidate before completion.
`session.WithBeforeComplete` adds an application validator after plugin validators.

The default limit is three continuations. Set `run.Config.MaxCompletionContinuations` for direct runs or `session.WithCompletionOptions` for embedded sessions.
Stored runs use the completion configuration saved at admission. A continuation beyond the limit fails with `run.ErrCompletionLimit`.

Saved runs retain decisions and feedback across restarts. An interrupted validator can run again before its decision is saved.
Use `info.Candidate.ID` to prevent duplicate external effects. Treat the candidate and history as read-only values.
`AfterRun` does not run when execution pauses for a remote decision.

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
