---
title: "Lifecycle hooks"
group: "Concepts"
draft: false
order: 106
---

# Lifecycle hooks

The loop exposes a small set of extension seams. Each seam allows exactly one function — there is one `Hooks` struct per `loop.Run` call, one call site per seam, and no surprise ordering at the loop level. When multiple plugins want the same seam, the [plugin registry](./plugins) composes them in install order.

You attach hooks in two places:

- **As a plugin contribution.** `Plugin.Install(*plugin.Registry)` calls `RegisterBeforeStep`, `RegisterTransformContext`, etc. Plugin hooks for the same seam chain.
- **As a one-off raw hook on the session.** `session.WithBeforeStep(h)` and `session.WithTransformContext(h)` install a single function. Raw hooks run *after* the plugin chain (so the user's hook sees the post-plugin slice and has the final word).

Hooks run synchronously on the loop goroutine. Slow hooks slow the loop.

## The `Hooks` struct

```go
type Hooks struct {
    BeforeIteration  func(ctx context.Context, step int) error
    AfterIteration   func(ctx context.Context, step int, turn Turn) error
    BeforeStep       BeforeStepHook
    TransformSystem  func(ctx context.Context, system string) (string, error)
    TransformContext TransformContextHook
    BeforeToolCall   BeforeToolCallFunc
    AfterToolCall    AfterToolCallFunc
}
```

Order within an iteration:

1. `BeforeIteration(step)`
2. `BeforeStep(info)` — may rewrite running history (persistent)
3. `TransformSystem(system)` — may rewrite the system prompt (per-turn)
4. `TransformContext(info)` — may rewrite the message slice (per-turn)
5. Provider call
6. For each tool call: `BeforeToolCall(call)` → `Tool.Execute` → `AfterToolCall(call, result, isError)`
7. `AfterIteration(step, turn)`

## `BeforeStep` vs `TransformContext`

Both hooks rewrite the message slice. They differ in *persistence*:

- **`BeforeStep`** mutates the loop's running history. The returned slice replaces `r.messages` and persists across subsequent turns. Use it for compaction, budget enforcement, or anything that should outlive a single turn.
- **`TransformContext`** is per-turn. The returned slice is sent to the model in place of the loop's running history; the running history itself is unaffected. Use it for redaction, just-in-time injection, or ephemeral trimming.

If a hook returns a slice with a different length, the loop emits a `ContextTransformedEvent` so observers can react.

```go
type BeforeStepInfo struct {
    Step     int
    Messages []wingmodels.Message
    Usage    wingmodels.Usage
    Model    wingmodels.Model
    Sink     loop.Sink
}
```

`info.Sink` is the loop's event sink. Hooks that synthesize new history messages — compaction markers, redaction notices, etc. — should emit a `MessageEvent` for each so observers (storage, UIs) see them on the same channel as loop-produced messages.

## Tool-call hooks

```go
type BeforeToolCallFunc func(ctx context.Context, call ToolCall) (newArgs map[string]any, err error)
type AfterToolCallFunc  func(ctx context.Context, call ToolCall, result string, isError bool) (newResult string, err error)
```

`BeforeToolCall` may return rewritten args or return `loop.ErrSkipTool` to skip execution. The loop synthesizes a tool result message containing the error message and `isError=true`. Wrap `ErrSkipTool` to provide a custom denial message:

```go
func gateBash(ctx context.Context, call loop.ToolCall) (map[string]any, error) {
    if call.Name == "bash" {
        if cmd, _ := call.Args["command"].(string); strings.Contains(cmd, "rm -rf") {
            return nil, fmt.Errorf("not permitted: %w", loop.ErrSkipTool)
        }
    }
    return nil, nil
}
```

`BeforeToolCall` fires even for unknown tools so hooks can synthesize a custom error.

`AfterToolCall` may rewrite the result string (truncation, redaction, formatting). It receives the actual `isError` flag.

## Composition order

When more than one plugin (or a plugin + a raw user hook) targets the same seam:

- **Pipeline seams** (`BeforeStep`, `TransformContext`, `BeforeToolCall`, `AfterToolCall`) chain. Each hook receives the previous one's output. Errors short-circuit the chain.
- **Sink subscribers** run independently. Every registered sink sees every event.
- **Tool registrations** merge into the session's tool slice; later wins on name collision via the loop's tool registry.

Within a session: plugin hooks for a seam compose first (in install order), then the user's raw hook composes on top so it sees the post-plugin view.

## Errors and stop reasons

A hook returning a non-`ErrSkipTool` error fails the loop. `loop.Run` returns that error and a `Result` whose `StopReason` is `error`. `Run` always returns a non-nil `*Result`, so callers can persist partial state.

`StopReason` values:

| Value | Meaning |
|---|---|
| `end_turn` | Assistant produced a tool-call-free turn |
| `max_steps` | Loop hit `Config.MaxSteps` |
| `aborted` | Context was cancelled |
| `error` | Unrecoverable error (provider error, hook error) |

`MaxSteps` defaults to 0 (infinity).

## Adding a new seam

If you find yourself wanting a new seam, the recipe is:

1. Declare an `Info` struct and `Hook` function type next to the existing ones.
2. Add a field to `loop.Hooks`.
3. Add a call site in `loop/run.go` at the appropriate point.
4. Add an event type (and `isEvent` method) if observers should see it cross the `Sink` boundary.
5. Add a `Register*` method to `plugin.Registry` and a composition function.
