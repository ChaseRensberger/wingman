---
title: "Plugins"
group: "Core"
order: 104
---

# Plugins

Plugins add tools or change how a session handles instructions, messages, and tool calls.

Plugins work within one session. To manage multiple sessions, build a client on the HTTP API.

## Plugin Forms

Choose a plugin type:

| Form                | Use it when                                                |
| ------------------- | ---------------------------------------------------------- |
| Go plugin           | Embed Wingman or build a custom binary with session hooks. |
| External RPC plugin | Add tools to `wingman serve` through an external program.  |

See [Plugin Capabilities](/extend/plugin-capabilities) for supported hooks and tools.

## Go Plugins

Go plugins are normal Go packages that implement the Wingman plugin interface:

```go
type Plugin interface {
    Name() string
    Activate(*plugin.Registry) (plugin.Cleanup, error)
}
```

Register hooks and tools in `Activate`. Return a cleanup function for resources that the plugin opens.

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

The stock `wingman serve` binary does not discover Go plugins from disk. See [Go Plugin Quickstart](/extend/plugin-quickstart) for a step-by-step example.

## External Plugins

Wingman loads external plugins from global directories and `.wingman/plugins/` under the session's working directory.
Each plugin runs as a separate program and communicates through JSON-RPC on standard input and output.

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

Project plugins are available only to sessions in that directory.

An external plugin uses a `wingman-plugin.json` file. Files ending in `.plugin.json` also load.
The manifest starts the process. The process returns its tools during `plugin.initialize`.

Minimal manifest:

```json
{
  "id": "example.greet",
  "name": "Greeting Plugin",
  "command": ["node", "/absolute/path/to/greet-plugin.js"]
}
```

Wingman runs `command` directly. Shell expansion is not applied. Pass each argument as a separate array item.

See [RPC Plugin Protocol](/extend/rpc-plugin-protocol) for manifest fields and the initialization response that declares tools.

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

Tool names must be unique across built-in, plugin, and MCP tools. Duplicate names prevent the tool catalog from loading.

`skill` is reserved for Wingman's native Agent Skill loader. Plugins cannot use
this name.

## Inspect Plugins

List plugins and load errors on the local managed service:

```bash
wingman api listPlugins
```

Use this command to reload plugins:

```bash
wingman api reloadPlugins
```
