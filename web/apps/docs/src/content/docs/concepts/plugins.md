---
title: "Plugins"
group: "Core"
order: 104
---

# Plugins

Plugins extend a Wingman session. They add tools, observe lifecycle events, transform model context, change tool calls, and add custom message parts.

Plugins are session-scoped. They cannot create sessions, list other sessions, or orchestrate multi-agent work. For orchestration, build a client on the Wingman HTTP API.

## Plugin Forms

Wingman uses one plugin model with two loading paths:

| Form                | Use it when                                                                                     |
| ------------------- | ----------------------------------------------------------------------------------------------- |
| Go plugin           | Use this form when you embed Wingman or ship a custom binary that needs typed in-process hooks. |
| External RPC plugin | Use this form when the stock `wingman serve` binary loads a subprocess from disk.               |

Go plugins provide lifecycle hooks. RPC plugins provide tool execution in the stock server.

See [Plugin Capabilities](/extend/plugin-capabilities) for the full matrix.

## Go Plugins

Go plugins are normal Go packages that implement the Wingman plugin interface:

```go
type Plugin interface {
    Name() string
    Activate(*plugin.Registry) (plugin.Cleanup, error)
}
```

`Activate` receives a session-scoped registry. Register hooks, tools, event sinks, transforms, or custom message-part decoders with this registry. Return cleanup for resources that activation acquires.

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

Add a Go plugin when you construct a session:

```go
sess := session.New(
    session.WithClient(client),
    session.WithModelRef(modelRef, modelInfo),
    session.WithPlugin(myplugin.New()),
)
defer sess.Close(context.Background())
```

Plugins activate before the first run. If replacement activation fails, the existing plugins stay active. `Session.Close` waits for active work. It then releases the plugins.

`RegisterSink` uses a one-second dispatch timeout by default. It permits one callback in flight for each sink. Use `RegisterSinkTimeout` to set another positive timeout. A blocked sink drops later events until its callback returns.

Use Go plugins for embedded applications, custom binaries, performance-sensitive hooks, and code that needs typed access to hook inputs.

The stock `wingman serve` binary does not discover Go plugins from disk. See [Go Plugin Quickstart](/extend/plugin-quickstart) for a step-by-step example.

## External Plugins

Wingman discovers external plugins in global plugin directories. If a session has a working directory, Wingman also discovers plugins in `.wingman/plugins/` under that directory. Wingman starts each plugin as a subprocess. It calls the plugin through newline-delimited JSON-RPC on stdio.

The default plugin directory is:

```text
~/.config/wingman/plugins/
```

Add another global plugin directory to the managed service with:

```bash
wingman service start --plugin-dir /path/to/plugins
```

Disable external plugin loading with:

```bash
wingman service start --no-plugins
```

Project-local plugins are available only to sessions in that working directory. Tool names must be unique across native, RPC, and MCP sources. A name collision prevents the tool catalog from loading.

An external plugin uses a `wingman-plugin.json` file. Files ending in `.plugin.json` also load.

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

Wingman runs `command` directly. Shell expansion is not applied. Pass each argument as a separate array item.

See [RPC Plugin Protocol](/extend/rpc-plugin-protocol) for manifest fields, the JSON-RPC request shape, and a minimal Node plugin.

## Using Plugin Tools

Plugin tools are selected like built-in tools. Include the tool name in an agent `tools` allow-list.

```json
{
  "name": "Greeter",
  "instructions": "Use greet when the user asks for a greeting.",
  "model_ref": "anthropic/claude-sonnet-5",
  "tools": ["greet"]
}
```

Plugin tool names must not collide with built-in, MCP, or other plugin tools. Wingman rejects duplicate names rather than choosing an implicit winner.

## Inspect Plugins

Use this command to list loaded plugins and non-fatal load errors:

These commands find and authenticate with the managed daemon.

```bash
wingman api listPlugins
```

Use this command to reload plugins in the directoryless scope:

```bash
wingman api reloadPlugins
```

External plugins run with the permissions of the Wingman process that starts them. Install plugins only from trusted sources.
