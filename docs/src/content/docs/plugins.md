---
title: "Plugins"
group: "Concepts"
draft: false
order: 107
---

# Plugins

A plugin is a bundle of [hook installations](./lifecycle), tools, sinks, and [Part type](./parts) registrations packaged behind a single `Install` call.

## Why plugins

The loop's `Hooks` struct allows exactly one function per seam (single call site, no surprise ordering). When multiple capabilities want the same seam — say, compaction wants `BeforeStep` and a budget enforcer also wants `BeforeStep` — wiring them by hand into both the loop config and the tool slice is mechanical and error-prone. A plugin is the aggregating abstraction: one `session.WithPlugin(...)` call installs everything the plugin contributes, and the registry composes contributions in install order.

## Opt-in by default

Nothing is installed unless you ask for it. A bare `session.New()` runs the loop with no hooks, no extra tools, and no extra sinks.

```go
import "github.com/chaserensberger/wingman/wingagent/plugin/compaction"

s := session.New(
    session.WithModel(p),
    session.WithPlugin(compaction.New()),
)
```

## The `Plugin` interface

```go
type Plugin interface {
    Name() string
    Install(*Registry) error
}
```

`Name` is a stable identifier used in error messages (and, later, observability). `Install` registers the plugin's contributions with the registry. It is called exactly once per `session.Run` invocation; an error fails the call.

## The `Registry`

```go
r.RegisterBeforeStep(h)        // BeforeStep hook
r.RegisterTransformContext(h)  // TransformContext hook
r.RegisterBeforeToolCall(h)    // BeforeToolCall hook (returning ErrSkipTool short-circuits)
r.RegisterAfterToolCall(h)     // AfterToolCall hook
r.RegisterSink(s)              // event observer (fan-out)
r.RegisterTool(t)              // adds to session's tool slice
r.RegisterPart(typeName, fn)   // registers a Part decoder with wingmodels (process-global, idempotent)
```

`Build` folds everything into a `Built{Hooks, Tools, Sink}` value the session feeds to `loop.Run`. Composition rules:

- **Pipeline seams** chain in install order. Each hook receives the previous one's output. The first error short-circuits.
- **Sinks** fan out: every registered sink sees every event, in install order.
- **Tools** merge into the session's tool slice. Plugin tools are appended after user-supplied tools, so plugins can override built-ins by name (later wins in the loop's tool registry).
- **Parts** call `wingmodels.RegisterPart` directly. The part registry is process-global and idempotent across re-installs.

## Authoring a plugin

```go
package mygate

import (
    "context"
    "fmt"
    "strings"

    "github.com/chaserensberger/wingman/wingagent/loop"
    "github.com/chaserensberger/wingman/wingagent/plugin"
)

type Plugin struct {
    blocked []string
}

func New(blocked ...string) *Plugin { return &Plugin{blocked: blocked} }

func (p *Plugin) Name() string { return "mygate" }

func (p *Plugin) Install(r *plugin.Registry) error {
    r.RegisterBeforeToolCall(func(ctx context.Context, call loop.ToolCall) (map[string]any, error) {
        if call.Name != "bash" {
            return nil, nil
        }
        cmd, _ := call.Args["command"].(string)
        for _, b := range p.blocked {
            if strings.Contains(cmd, b) {
                return nil, fmt.Errorf("blocked substring %q: %w", b, loop.ErrSkipTool)
            }
        }
        return nil, nil
    })
    return nil
}
```

Install:

```go
s := session.New(
    session.WithModel(p),
    session.WithPlugin(mygate.New("rm -rf /", "shutdown")),
)
```

Plugins should keep their `Name()` stable across versions so observability layers can attribute hook activity.

## The compaction plugin

`wingagent/plugin/compaction` is the canonical worked example. It demonstrates:

- a custom **Part type** registered via `RegisterPart` and serialized through `OpaquePart`
- a **`BeforeStep`** hook that summarizes the head of long histories and *appends* a marker (the original messages stay in the durable transcript)
- a **`TransformContext`** hook that walks the per-turn slice, finds the latest marker, and builds the model-facing view as `[summary text] + [messages after marker]`

This two-seam design is intentional: single-seam approaches (truncate-and-replace in `BeforeStep`) lose history irrecoverably and prevent UIs from rendering the pre-compaction transcript. Splitting write (append marker) from read (filter) keeps every byte addressable.

```go
import "github.com/chaserensberger/wingman/wingagent/plugin/compaction"

s := session.New(
    session.WithModel(p),
    session.WithPlugin(compaction.New(
        compaction.WithThreshold(0.7),  // compact at 70% of context window
        compaction.WithKeepTail(8),     // preserve last 8 messages untouched
        compaction.WithMinMessages(6),  // never run below this floor
    )),
)
```

| Option | Default | Purpose |
|---|---|---|
| `WithThreshold(f)` | `0.85` | input-tokens / context-window ratio that triggers compaction |
| `WithKeepTail(n)` | `4` | trailing messages preserved untouched |
| `WithMinMessages(n)` | `6` | floor below which compaction never runs (0 disables) |
| `WithSummaryPrompt(s)` | built-in | overrides the summarization system prompt |
| `WithModel(m)` | loop's model | use a separate model for the summarization sub-call |

The plugin calls `Model.CountTokens` against the current snapshot, which avoids two bugs in any "estimate from last turn's usage" approach: first-call blindness and lag-by-one. `CountTokens` errors fall back to a chars/4 heuristic so a flaky counter endpoint never blocks the loop.

When compaction runs, the loop emits a `ContextTransformedEvent` whose head message's first part has discriminator `compaction_marker`. The session stream classifier surfaces this as a dedicated `compaction` SSE event. Decode the marker with `compaction.DecodeMarker(part)`.

## Loading model

v0.1 plugins are compile-time only: a `Plugin` is a Go value the program builds and passes to `session.WithPlugin`. Future versions may add MCP-style external plugins (for tools) and Yaegi-script plugins (for hooks).
