---
title: "Go Plugin Quickstart"
description: "Create an in-process Wingman plugin that observes session events."
group: "Reference"
order: 1002
---

# Go Plugin Quickstart

Go plugins are Go packages that implement Wingman's `plugin.Plugin` interface.
Use this path when you embed Wingman or ship a custom binary.

This guide creates a plugin that observes session events through a sink.

## 1. Create A Plugin Package

```go
package traceplugin

import (
	"context"
    "fmt"
    "log/slog"

    "github.com/chaserensberger/wingman/agent/run"
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

func (p *Plugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
    if err := r.RegisterSink(run.SinkFunc(p.sink)); err != nil {
        return nil, err
    }
    return nil, nil
}

func (p *Plugin) sink(event run.Event) {
    p.logger.Info("wingman event", "type", fmt.Sprintf("%T", event))
}
```

Keep `Name` stable across versions. Wingman uses it to identify the plugin in errors.

## 2. Install The Plugin

Install the plugin when you construct a session:

```go
sess := session.New(
    session.WithClient(client),
    session.WithModelRef(modelRef, modelInfo),
    session.WithPlugin(traceplugin.New(logger)),
)
defer sess.Close(context.Background())
```

Go plugins are linked into the Go process. The stock `wingman serve` binary does
not discover Go plugins from disk.

## 3. Add More Capabilities

Inside `Activate`, register the capabilities that your plugin contributes. Return
cleanup for resources such as files, workers, or subscriptions:

```go
func (p *Plugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
    if err := r.RegisterBeforeRun(p.beforeRun); err != nil { return nil, err }
    if err := r.RegisterTransformContext(p.transformContext); err != nil { return nil, err }
    if err := r.RegisterBeforeToolCall(p.beforeToolCall); err != nil { return nil, err }
    if err := r.RegisterAfterToolCall(p.afterToolCall); err != nil { return nil, err }
    if err := r.RegisterSink(p.sink); err != nil { return nil, err }
    if err := r.RegisterTool(p.tool); err != nil { return nil, err }
    return func(ctx context.Context) error { return p.close(ctx) }, nil
}
```

Hooks run in activation order. Transform hooks receive the previous hook's
output. Sinks use a bounded callback time. If activation fails, existing plugins
remain active.

## When To Use Go Plugins

Use Go plugins for:

- Lifecycle hooks.
- Event sinks.
- Context, history, tool definition, and parameter transforms.
- Custom tools in embedded applications.
- Custom message-part decoders.
- Performance-sensitive extensions.

Use [RPC plugins](/extend/rpc-plugin-protocol) when the stock server must load
an out-of-process plugin from disk.
