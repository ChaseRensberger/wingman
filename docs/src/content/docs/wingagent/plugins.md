---
title: "Plugins"
group: "Concepts"
draft: false
order: 107
---

# Plugins

A plugin is a bundle of [hook installations](./lifecycle), tools, sinks, and [Part type](../wingmodels/parts) registrations packaged behind a single `Install` call.

## Why plugins

The loop's `Hooks` struct allows exactly one function per seam (single call site, no surprise ordering). When multiple capabilities want the same seam — say, compaction wants `BeforeStep` and a budget enforcer also wants `BeforeStep` — wiring them by hand into both the loop config and the tool slice is mechanical and error-prone. A plugin is the aggregating abstraction: one `session.WithPlugin(...)` call installs everything the plugin contributes, and the registry composes contributions in install order.

The flip side: the loop core knows nothing about any specific plugin. Storage, compaction, gates, redaction — they all live outside `wingagent/loop` and hook in through the same registry as user-authored plugins. The loop stays minimal; capabilities ship as additive modules.

## Opt-in by default

Nothing is installed unless you ask for it. A bare `session.New()` runs the loop with no hooks, no extra tools, and no extra sinks.

```go
import "github.com/chaserensberger/wingman/plugins/compaction"

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

`Name()` must be unique within a session. Installing two plugins that return the same name fails the run with `plugin %q already installed in this session`. This catches misconfigurations like wiring two storage plugins (which would fight over initial history) before they corrupt anything.

## The `Registry`

```go
r.RegisterBeforeRun(h)         // BeforeRun hook (initial history)
r.RegisterBeforeStep(h)        // BeforeStep hook
r.RegisterTransformContext(h)  // TransformContext hook
r.RegisterBeforeToolCall(h)    // BeforeToolCall hook (returning ErrSkipTool short-circuits)
r.RegisterAfterToolCall(h)     // AfterToolCall hook
r.RegisterSink(s)              // event observer (fan-out)
r.RegisterTool(t)              // adds to session's tool slice
r.RegisterPart(typeName, fn)   // registers a Part decoder with wingmodels (process-global, idempotent)
```

`Build` folds everything into a `Built{Hooks, Tools, Sink}` value the session feeds to `loop.Run`. Composition rules:

- **Pipeline seams** (`BeforeRun`, `BeforeStep`, `TransformContext`, `BeforeToolCall`, `AfterToolCall`) chain in install order. Each hook receives the previous one's output. The first error short-circuits.
- **Sinks** fan out: every registered sink sees every event, in install order.
- **Tools** merge into the session's tool slice. Plugin tools are appended after user-supplied tools, so plugins can override built-ins by name (later wins in the loop's tool registry).
- **Parts** call `wingmodels.RegisterPart` directly. The part registry is process-global and idempotent across re-installs.

## Hooks vs sinks

A plugin commonly contributes both. The distinction is covered in detail on the [lifecycle](./lifecycle#hooks-vs-sinks) page; the short version:

- A **hook** participates in the loop. It can change behavior, must complete before the loop continues, and chains with other plugins' hooks for the same seam.
- A **sink** observes the loop. It receives events fan-out, can't change anything, and runs alongside other sinks.

The [storage plugin](./storage) is the worked example: a `BeforeRun` hook (load history from disk) plus a sink (persist new messages as they land).

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

`plugins/compaction` is the canonical hooks-only worked example. It demonstrates:

- a custom **Part type** registered via `RegisterPart` and serialized through `OpaquePart`
- a **`BeforeStep`** hook that summarizes the head of long histories and *appends* a marker (the original messages stay in the durable transcript)
- a **`TransformContext`** hook that walks the per-turn slice, finds the latest marker, and builds the model-facing view as `[summary text] + [messages after marker]`

This two-seam design is intentional: single-seam approaches (truncate-and-replace in `BeforeStep`) lose history irrecoverably and prevent UIs from rendering the pre-compaction transcript. Splitting write (append marker) from read (filter) keeps every byte addressable.

```go
import "github.com/chaserensberger/wingman/plugins/compaction"

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

## The storage plugin

`storage` ships a plugin that gives a session full-cycle persistence — both load and save — through a single `session.WithPlugin` call:

```go
import (
    "github.com/chaserensberger/wingman/plugins/storage"
    "github.com/chaserensberger/wingman/wingagent/session"
    wstorage "github.com/chaserensberger/wingman/storage"
)

store, _ := wstorage.NewSQLiteStore("/path/to/wingman.db")
sess, _ := store.GetSession(sessionID) // ensure the session row exists

s := session.New(
    session.WithModel(model),
    session.WithPlugin(storageplugin.NewPlugin(store, sess.ID)),
)
```

Internally the plugin contributes:

- a **`BeforeRun` hook** that calls `store.GetSession(sessionID)` and returns `sess.History` as the loop's initial messages
- a **sink** that filters for `loop.MessageEvent` and calls `store.AppendMessage` for each completed message

This is the same wiring the [HTTP server](../server) uses internally, expressed as a reusable capability rather than a hard-coded server detail. See the [storage](./storage) page for more.

## Loading model

v0.1 plugins are compile-time only: a `Plugin` is a Go value the program builds and passes to `session.WithPlugin`. Future versions may add MCP-style external plugins (for tools) and Yaegi-script plugins (for hooks).
