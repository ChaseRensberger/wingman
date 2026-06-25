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

See [Plugin Capabilities](/docs/reference/plugin-capabilities) for the full matrix.

## Go Plugins

Go plugins are normal Go packages that implement Wingman's plugin interface:

```go
type Plugin interface {
    Name() string
    Install(*plugin.Registry) error
}
```

`Install` receives a registry. Use it to register hooks, tools, event sinks, transforms, or custom message-part decoders.

```go
func (p *Plugin) Install(r *plugin.Registry) error {
    r.RegisterTransformContext(p.transformContext)
    r.RegisterSink(run.SinkFunc(p.sink))
    return nil
}
```

Install a Go plugin when constructing a session:

```go
sess := session.New(
    session.WithClient(client),
    session.WithModelRef(modelRef, modelInfo),
    session.WithPlugin(myplugin.New()),
)
```

Use Go plugins for embedded applications, custom binaries, performance-sensitive hooks, and code that needs typed access to hook inputs.

The stock `wingman serve` binary does not discover Go plugins from disk. See [Go Plugin Quickstart](/docs/reference/plugin-quickstart) for a step-by-step example.

## External Plugins

External plugins are discovered from global plugin directories, started as subprocesses, and called over newline-delimited JSON-RPC on stdio.

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

See [RPC Plugin Protocol](/docs/reference/rpc-plugin-protocol) for the manifest fields, JSON-RPC request shape, and a minimal Node plugin.

## Using Plugin Tools

Plugin tools are selected like built-in tools: include the tool name in an agent's `tools` allow-list.

```json
{
  "name": "Greeter",
  "instructions": "Use greet when the user asks for a greeting.",
  "model_ref": "anthropic/claude-sonnet-4-6",
  "tools": ["greet"]
}
```

If a plugin tool has the same name as a built-in tool, the plugin tool wins during session tool resolution. Avoid collisions unless you intentionally want to replace behavior.

## Inspect Plugins

List loaded plugins and non-fatal load errors:

```bash
curl http://127.0.0.1:2323/plugins/
```

Reload global plugins:

```bash
curl -X POST http://127.0.0.1:2323/plugins/reload
```

External plugins run with the same operating-system permissions as the Wingman process that starts them. Install plugins only from sources you trust.
