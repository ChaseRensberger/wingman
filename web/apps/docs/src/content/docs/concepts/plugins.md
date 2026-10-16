---
title: "Plugins"
description: "Choose a plugin type and load external tools."
---

# Plugins

Plugins add tools or change session behavior. Go plugins run inside an embedded application or custom Wingman binary.
RPC plugins run as external programs and add tools to the standard server.

Use a client on the HTTP API to manage multiple sessions.
See [Plugin Capabilities](/extend/plugin-capabilities) for extension methods.

## Install an external plugin

Put a `wingman-plugin.json` manifest in `~/.config/wingman/plugins/` or one of its immediate subdirectories.
Files ending in `.plugin.json` also load from the plugin directory.
The default plugin path does not follow `XDG_CONFIG_HOME`.

```json
{
  "id": "example.greet",
  "name": "Greeting Plugin",
  "command": ["node", "/absolute/path/to/greet-plugin.js"]
}
```

Install the program and its runtime before you start Wingman. The manifest starts it directly, without shell expansion.
The program declares its tools during initialization. See [RPC Plugin Protocol](/extend/rpc-plugin-protocol).

Add global directories with `plugins.dirs` in `wingman.json` or a repeated `--plugin-dir` flag.
Disable external plugins with `--no-plugins`.
Project manifests in `<working-directory>/.wingman/plugins/` apply only to sessions in that directory.

## Use and reload tools

Add plugin tool names to an agent's `tools` list.
Stored agent validation uses the global tool catalog. A tool that exists only in a project plugin cannot pass that validation.
An inline agent in `/run` can select project tools for its working directory.

Names must be unique across built-in, plugin, and MCP tools. The name `skill` is reserved.
Plugins run with Wingman's operating system permissions. Install only plugins that you trust.

To read plugin status on the local managed service, run `wingman api listPlugins`.
To reload plugins, run `wingman api reloadPlugins`.
If the replacement fails to load, Wingman keeps the current plugins.

## Go plugins

Go plugins implement `plugin.Plugin` and register tools or hooks in `Activate`.
Pass them to `session.WithPlugin` when constructing a session.
The standard server does not load Go packages from disk.

Follow [Go Plugin Quickstart](/extend/plugin-quickstart) for a working example.
