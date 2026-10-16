---
title: "Go Plugin Quickstart"
description: "Create a Go plugin that logs session events."
---

# Go Plugin Quickstart

Go plugins run in an embedded application or custom binary.
They implement `plugin.Plugin`. The standard `wingman serve` binary does not load Go packages from disk.

## Create the plugin

This package logs each event type:

```go
package traceplugin

import (
	"fmt"
	"log/slog"

	"github.com/chaserensberger/wingman/agent/plugin"
	"github.com/chaserensberger/wingman/agent/run"
)

// Plugin logs session event types.
type Plugin struct {
	logger *slog.Logger
}

// New creates an event logger.
func New(logger *slog.Logger) *Plugin {
	return &Plugin{logger: logger}
}

// Name returns the plugin ID.
func (p *Plugin) Name() string { return "trace" }

// Activate registers the event callback.
func (p *Plugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
	err := r.RegisterSink(run.SinkFunc(func(event run.Event) {
		p.logger.Info("wingman event", "type", fmt.Sprintf("%T", event))
	}))
	return nil, err
}
```

Keep the plugin name stable and unique within the session.
Pass a non-nil logger to `New`.
Return cleanup from `Activate` if your plugin opens resources.

## Install it in a session

In your application, create a model client and obtain its model reference and metadata.
Import `context`, `agent/session`, and your `traceplugin` package, then pass the plugin to the session:

```go
sess := session.New(
	session.WithClient(client),
	session.WithModelRef(modelRef, modelInfo),
	session.WithPlugin(traceplugin.New(logger)),
)
defer sess.Close(context.Background())
```

Plugins activate before the first run. Event callbacks must return promptly because slow callbacks can lose later events.
See [Plugin Capabilities](/extend/plugin-capabilities) for hooks, tools, actions, and callback limits.
To add external tools without building a custom binary, use [RPC plugins](/extend/rpc-plugin-protocol).
