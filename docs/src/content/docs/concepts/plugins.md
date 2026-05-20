---
title: "Plugins"
group: "Core"
order: 104
---

Plugins extend the behavior of Wingman. They are the main way to add tools, hook into session lifecycle events, transform model context, observe runs, and teach Wingman about custom message parts.

Plugins are session-scoped. They do not create sessions, list other sessions, or orchestrate multi-agent workflows. If you need orchestration, build a client that uses the Wingman HTTP API.

Choose a plugin form based on where the plugin runs:

- Go plugins linked into a Wingman binary or embedding application.
- External RPC plugins discovered from global plugin directories and run as subprocesses.

Go plugins are the first-class lifecycle extension path. External RPC plugins contribute custom tools to the stock server.

## Which Plugin Form Should I Use?

Use a Go plugin when you control the Go process that creates sessions or can ship a custom Wingman binary. This is the stable, typed path.

Use an external RPC plugin when you want to add a custom tool to the stock `wingman serve` binary without rebuilding Wingman.

## Go Plugins

Go plugins are normal Go packages that implement Wingman's plugin interface and are installed with `session.WithPlugin(...)`.

This is the best fit for:

- Application code that embeds Wingman.
- Behavior shipped inside a custom Wingman binary.
- Plugins that need typed access to hook inputs.
- Performance-sensitive hooks that should avoid process boundaries.

Use a Go plugin when you control the Go process that creates the session.

The plugin contract is small:

```go
type Plugin interface {
    Name() string
    Install(*plugin.Registry) error
}
```

`Install` receives a registry. Use the registry to contribute hooks, tools, event sinks, and custom message-part decoders.

```go
package myplugin

import (
    "context"

    "github.com/chaserensberger/wingman/agent/loop"
    "github.com/chaserensberger/wingman/agent/plugin"
    "github.com/chaserensberger/wingman/models"
)

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Name() string { return "my-plugin" }

func (p *Plugin) Install(r *plugin.Registry) error {
    r.RegisterTransformContext(p.transformContext)
    return nil
}

func (p *Plugin) transformContext(ctx context.Context, info loop.TransformContextInfo) ([]models.Message, error) {
    return info.Messages, nil
}
```

Install it when constructing a session:

```go
sess := session.New(
    session.WithClient(client),
    session.WithModelRef(modelRef, modelInfo),
    session.WithPlugin(myplugin.New()),
)
```

### Go Plugin Capabilities

Go plugins can register these capabilities:

- `RegisterBeforeRun`
- `RegisterTransformHistory`
- `RegisterTransformContext`
- `RegisterBeforeToolCall`
- `RegisterAfterToolCall`
- `RegisterAfterRun`
- `RegisterTransformToolDefs`
- `RegisterTransformParams`
- `RegisterSink`
- `RegisterTool`
- `RegisterPart`

When multiple Go plugins are installed, composition is deterministic:

- Transform hooks chain in install order; each hook receives the previous hook's output.
- Tool hooks chain in install order; `BeforeToolCall` can skip execution with `loop.ErrSkipTool`.
- Sinks fan out; every registered sink sees every event.
- Plugin tools are appended to the session tool list.
- Part decoders register into the process-global model part registry.

### Go Plugin Example: Compaction

`plugins/compaction` is the canonical in-process plugin. It uses two hooks:

- `TransformHistory` appends a compacted marker into durable history.
- `TransformContext` rewrites the model-facing context so old messages are replaced by the latest summary.

Use it with:

```go
sess := session.New(
    session.WithPlugin(compaction.New()),
)
```

Go plugins are not discovered from disk by the stock `wingman serve` binary. They must be linked into a binary or installed by an embedding application. See [Go Plugin Quickstart](/reference/plugin-quickstart) for a step-by-step example.

## External Plugins

External plugins are discovered from global plugin directories, started as subprocesses, and called over stdio JSON-RPC.

Use an external plugin when you want to add a tool to the stock `wingman serve` binary without rebuilding Wingman. Lifecycle hooks and stateful runtime extensions use Go plugins.

### Discovery

Wingman loads global plugins from:

```text
~/.config/wingman/plugins/
```

You can add more global plugin directories when starting the server:

```bash
wingman serve --plugin-dir /path/to/plugins
```

Disable out-of-process plugins entirely with:

```bash
wingman serve --no-plugins
```

### Manifest

A plugin is declared by a `wingman-plugin.json` file in a plugin directory or inside a subdirectory. Files ending in `.plugin.json` are also loaded.

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
          "name": {
            "type": "string",
            "description": "Name to greet"
          }
        },
        "required": ["name"]
      }
    }
  ]
}
```

`command` is executed directly. Shell expansion is not applied, so pass every argument as a separate array item.

The tool schema uses Wingman's current typed JSON Schema subset:

- Object-shaped inputs.
- Property `type`.
- Optional property `description`.
- Optional property `enum`.
- Optional top-level `required`.

### Protocol

Wingman starts the plugin process and sends JSON-RPC requests on stdin. The plugin replies on stdout.

Tool execution request:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tool.execute",
  "params": {
    "tool": "greet",
    "params": { "name": "Chase" },
    "work_dir": "/home/chase/project"
  }
}
```

Tool execution response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "text": "Hello, Chase"
  }
}
```

Error response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "message": "missing name"
  }
}
```

### Minimal Node Plugin

```js
#!/usr/bin/env node

process.stdin.setEncoding("utf8")

let buffer = ""
process.stdin.on("data", (chunk) => {
  buffer += chunk
  for (;;) {
    const idx = buffer.indexOf("\n")
    if (idx < 0) return
    const line = buffer.slice(0, idx).trim()
    buffer = buffer.slice(idx + 1)
    if (line) handle(JSON.parse(line))
  }
})

function handle(req) {
  if (req.method !== "tool.execute" || req.params.tool !== "greet") {
    reply(req.id, null, { message: "unknown method or tool" })
    return
  }
  reply(req.id, { text: `Hello, ${req.params.params.name}` })
}

function reply(id, result, error) {
  const body = error
    ? { jsonrpc: "2.0", id, error }
    : { jsonrpc: "2.0", id, result }
  process.stdout.write(`${JSON.stringify(body)}\n`)
}
```

### Using External Plugin Tools

Plugin tools are selected the same way as built-in tools: include the tool name in an agent's `tools` allow-list.

```json
{
  "name": "Greeter",
  "instructions": "Use greet when the user asks for a greeting.",
  "model_ref": "openai/gpt-4.1",
  "tools": ["greet"]
}
```

If a plugin tool has the same name as a built-in tool, the plugin tool wins during session tool resolution. Avoid collisions unless you intentionally want to replace behavior.

### HTTP API

List loaded plugins and non-fatal load errors:

```bash
curl http://127.0.0.1:2323/plugins/
```

Reload global plugins:

```bash
curl -X POST http://127.0.0.1:2323/plugins/reload
```

See [RPC Plugin Protocol](/reference/rpc-plugin-protocol) for the manifest and JSON-RPC contract.

### Supported RPC Surface

External plugins run with the same operating-system permissions as the Wingman process that starts them. Only install plugins from sources you trust.

External RPC plugins support custom tools. Lifecycle hooks, event sinks, state APIs, and custom part decoders are Go-plugin capabilities.

## Parity Target

Wingman's target is one plugin model with two transports: typed Go first, then RPC parity where the transport makes sense. See [Plugin Capabilities](/reference/plugin-capabilities) for the supported surface.
