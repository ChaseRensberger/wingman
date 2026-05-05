---
title: "SDK"
group: "Usage"
draft: true
order: 10
---

# SDK

Use the Go SDK when you want direct access to Wingman's runtime primitives inside your own application. The SDK has no `agent` package — the unit of execution is a `*session.Session`.

## Installation

```bash
go get github.com/chaserensberger/wingman
```

## Minimal example

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/joho/godotenv"

    "github.com/chaserensberger/wingman/agent/session"
    "github.com/chaserensberger/wingman/tool"
    "github.com/chaserensberger/wingman/models/providers/anthropic"
)

func main() {
    godotenv.Load(".env.local")

    workDir, _ := os.Getwd()

    p, err := anthropic.New(anthropic.Config{})
    if err != nil {
        log.Fatal(err)
    }

    s := session.New(
        session.WithWorkDir(workDir),
        session.WithModel(p),
        session.WithSystem("You are a senior Go developer."),
        session.WithTools(tool.NewBashTool(), tool.NewWriteTool()),
    )

    result, err := s.Run(context.Background(), "Write hello.go and run it")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Response)
}
```

## Build from primitives

The typical SDK flow is:

1. Construct a model (provider).
2. Construct a session with `session.New(...)` and the options you need.
3. Optionally install plugins for cross-cutting behavior.
4. Drive `Run` or `RunStream` with your own context and persistence.

The SDK owns no persistence and no transport; the caller decides how to store history (or whether to at all).

## Providers

Provider packages expose typed constructors and accept a free-form `Options map[string]any` for inference settings.

```go
p, err := anthropic.New(anthropic.Config{
    APIKey: "sk-ant-...",
    Options: map[string]any{
        "model":      "claude-haiku-4-5",
        "max_tokens": 4096,
        "temperature": 0.2,
    },
})
```

If you prefer the registry path the server uses, blank-import a provider and construct it by ID:

```go
import (
    provider "github.com/chaserensberger/wingman/models/providers"
    _ "github.com/chaserensberger/wingman/models/providers/anthropic"
)

m, err := provider.New("anthropic", map[string]any{
    "model":      "claude-haiku-4-5",
    "max_tokens": 4096,
    "api_key":    "sk-ant-...",
})
```

See [Providers](./models/providers).

## Session options

`session.New` accepts these options:

| Option | Purpose |
|---|---|
| `WithModel(m)` | Set the active `models.Model`. Required before `Run`. |
| `WithSystem(s)` | System prompt sent on every turn. |
| `WithTools(...)` | Tools the model may invoke. |
| `WithWorkDir(dir)` | Working directory passed to tool executions. |
| `WithPlugin(...)` | Install plugins (compaction, custom). Opt-in; nothing is installed by default. |
| `WithBeforeStep(h)` | One-off raw `BeforeStepHook`. Composes after plugin hooks. |
| `WithTransformContext(h)` | One-off raw `TransformContextHook`. |
| `WithMessageSink(fn)` | Synchronous callback fired for every message added to history. |

Setters (`SetModel`, `SetSystem`, `SetTools`, `SetWorkDir`) let handlers configure a session lazily. `History()`, `AddMessage`, `SetHistory`, and `Clear` manage the running transcript — useful when rehydrating from store.

## Multi-turn

```go
s := session.New(session.WithModel(p))

r1, _ := s.Run(ctx, "What is 2 + 2?")
r2, _ := s.Run(ctx, "Multiply that by 10")

_ = r1
_ = r2
```

## Streaming

`RunStream` runs the loop on a background goroutine and exposes a single-consumer iterator.

```go
stream, err := s.RunStream(ctx, "Tell me a story")
if err != nil {
    log.Fatal(err)
}

for stream.Next() {
    ev := stream.Event()
    fmt.Printf("%s\t%v\n", ev.Type, ev.Data)
}
if err := stream.Err(); err != nil {
    log.Fatal(err)
}
result := stream.Result()
_ = result
```

If the consumer stops calling `Next`, the loop blocks on the event channel. Cancel the context to abort. See [Streaming](./agent/streaming).

## Plugins

Plugins are opt-in. The canonical example is `compaction.New()`:

```go
import "github.com/chaserensberger/wingman/plugins/compaction"

s := session.New(
    session.WithModel(p),
    session.WithPlugin(compaction.New()),
)
```

See [Plugins](./agent/plugins) for authoring your own.

## Tools

Built-in tools live under `tool`. Custom tools implement `tool.Tool`. See [Tools](./tools).
