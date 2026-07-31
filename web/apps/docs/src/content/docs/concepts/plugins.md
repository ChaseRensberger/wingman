---
title: "Plugins"
group: "Core"
order: 104
---

# Plugins

Plugins extend a Wingman session. They can add tools, observe lifecycle events, transform model context, change tool calls, and teach Wingman about custom message parts.

Plugins are session-scoped. They do not create sessions, list other sessions, or orchestrate multi-agent workflows. If you need orchestration, build a client that uses the Wingman HTTP API.

## Plugin Forms

Wingman has one plugin model with two loading paths:

| Form | Use it when |
|---|---|
| Go plugin | You embed Wingman or ship a custom binary and want typed in-process hooks. |
| External RPC plugin | You want the stock `wingman serve` binary to load a subprocess from disk. |

Go plugins have the full lifecycle surface. RPC plugins currently expose tool execution for the stock server.

See [Plugin Capabilities](/extend/plugin-capabilities) for the full matrix.

## Go Plugins

Go plugins are normal Go packages that implement Wingman's plugin interface:

```go
type Plugin interface {
    Name() string
    Activate(*plugin.Registry) (plugin.Cleanup, error)
}
```

`Activate` receives a session-scoped registry. Use it to register hooks, tools,
event sinks, transforms, or custom message-part decoders. Return cleanup for any
resources acquired during activation.

```go
func (p *Plugin) Activate(r *plugin.Registry) (plugin.Cleanup, error) {
    if err := r.RegisterTransformContext(p.transformContext); err != nil {
        return nil, err
    }
    if err := r.RegisterSink(run.SinkFunc(p.sink)); err != nil {
        return nil, err
    }
    return func(context.Context) error {
        return p.close()
    }, nil
}
```

Install a Go plugin when constructing a session:

```go
sess := session.New(
    session.WithClient(client),
    session.WithModelRef(modelRef, modelInfo),
    session.WithPlugin(myplugin.New()),
)
defer sess.Close(context.Background())
```

Plugins activate lazily before the first run. `SetPlugins` stages a complete
replacement and swaps it only after successful activation; failure preserves the
current generation. `Session.Close` waits for active work and releases plugins
in reverse activation order. Custom part decoders are scoped to that generation
and never mutate a process-global registry.

`RegisterSink` uses a one-second dispatch timeout by default and permits at most
one callback in flight per sink. Use `RegisterSinkTimeout` to choose another
positive timeout. A blocked sink drops later events until its callback returns.

Use Go plugins for embedded applications, custom binaries, performance-sensitive hooks, and code that needs typed access to hook inputs.

The stock `wingman serve` binary does not discover Go plugins from disk. See [Go Plugin Quickstart](/extend/plugin-quickstart) for a step-by-step example.

## External Plugins

External plugins are discovered from global plugin directories and, when a
session has a working directory, from `.wingman/plugins/` under that directory.
They are started as subprocesses and called over newline-delimited JSON-RPC on
stdio.

The default plugin directory is:

```text
~/.config/wingman/plugins/
```

Add another global plugin directory with:

```bash
wingman serve --plugin-dir /path/to/plugins
```

Disable external plugin loading with:

```bash
wingman serve --no-plugins
```

Wingman discovers project-local plugins when it first builds a session with that
project as its working directory. The external-plugin manager is shared by the
daemon, so a discovered project-local plugin remains available to later
sessions, including sessions without a working directory, until a plugin reload
or daemon restart. Tool names must be globally unique across native, RPC, and MCP
sources; a collision makes catalog composition fail instead of replacing a tool.

An external plugin is declared by a `wingman-plugin.json` file. Files ending in `.plugin.json` are also loaded.

Minimal manifest:

```json
{
  "id": "example.greet",
  "name": "Greeting Plugin",
  "command": ["node", "/absolute/path/to/greet-plugin.js"],
  "tools": [
    {
      "name": "greet",
      "description": "Greet someone by name",
      "input_schema": {
        "type": "object",
        "properties": {
          "name": { "type": "string", "description": "Name to greet" }
        },
        "required": ["name"]
      }
    }
  ]
}
```

`command` is executed directly. Shell expansion is not applied, so pass every argument as a separate array item.

See [RPC Plugin Protocol](/extend/rpc-plugin-protocol) for the manifest fields, JSON-RPC request shape, and a minimal Node plugin.

## Using Plugin Tools

Plugin tools are selected like built-in tools: include the tool name in an agent's `tools` allow-list.

```json
{
  "name": "Greeter",
  "instructions": "Use greet when the user asks for a greeting.",
  "model_ref": "anthropic/claude-sonnet-5",
  "tools": ["greet"]
}
```

Plugin tool names must not collide with built-in, MCP, or other plugin tools.
Wingman rejects duplicate names rather than choosing an implicit winner.

## Inspect Plugins

List loaded plugins and non-fatal load errors:

```bash
curl http://127.0.0.1:2323/plugins/
```

Reload global plugins:

```bash
curl -X POST http://127.0.0.1:2323/plugins/reload
```

External plugins run with the same permissions as the Wingman process that starts them. Install plugins only from sources you trust.
