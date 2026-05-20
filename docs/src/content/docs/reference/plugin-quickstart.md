---
title: "Go Plugin Quickstart"
description: "Create an in-process Wingman plugin that observes session events."
group: "Reference"
order: 1002
---

# Go Plugin Quickstart

Go plugins are normal Go packages that implement Wingman's `plugin.Plugin` interface. Use this path when you embed Wingman, ship a custom binary, or need typed lifecycle hooks.

This guide creates a plugin that observes session events through a sink.

## 1. Create A Plugin Package

```go
package traceplugin

import (
    "fmt"
    "log/slog"

    "github.com/chaserensberger/wingman/agent/loop"
    "github.com/chaserensberger/wingman/agent/plugin"
)

type Plugin struct {
    logger *slog.Logger
}

func New(logger *slog.Logger) *Plugin {
    return &Plugin{logger: logger}
}

func (p *Plugin) Name() string {
    return "trace"
}

func (p *Plugin) Install(r *plugin.Registry) error {
    r.RegisterSink(loop.SinkFunc(p.sink))
    return nil
}

func (p *Plugin) sink(event loop.Event) {
    p.logger.Info("wingman event", "type", fmt.Sprintf("%T", event))
}
```

`Name` should stay stable across versions. Wingman uses it for attribution in errors and observability.

## 2. Install The Plugin

Install the plugin when constructing a session:

```go
sess := session.New(
    session.WithClient(client),
    session.WithModelRef(modelRef, modelInfo),
    session.WithPlugin(traceplugin.New(logger)),
)
```

Go plugins are linked into the Go process. The stock `wingman serve` binary does not discover Go plugins from disk.

## 3. Add More Capabilities

Inside `Install`, register any capabilities your plugin contributes:

```go
func (p *Plugin) Install(r *plugin.Registry) error {
    r.RegisterBeforeRun(p.beforeRun)
    r.RegisterTransformContext(p.transformContext)
    r.RegisterBeforeToolCall(p.beforeToolCall)
    r.RegisterAfterToolCall(p.afterToolCall)
    r.RegisterSink(p.sink)
    r.RegisterTool(p.tool)
    return nil
}
```

Hooks compose in install order. Transform hooks receive the previous hook's output. Sinks fan out so every registered sink sees every event.

## When To Use Go Plugins

Use Go plugins for:

- Lifecycle hooks.
- Event sinks.
- Context, history, tool definition, and parameter transforms.
- Custom tools in embedded applications.
- Custom message-part decoders.
- Performance-sensitive extensions.

Use [RPC plugins](/reference/rpc-plugin-protocol) when you only need to add a custom tool to the stock server without rebuilding Wingman.
