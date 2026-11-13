---
title: "Plugins"
description: "Choose a plugin type and load external tools."
---

# Plugins

Plugins add tools or change session behavior. The standard server registers compaction, `wingcode`, and orientation as enabled plugins.
These plugins are bundled with the server and apply to every agent.

Use a client on the HTTP API to manage multiple sessions.
See [Plugin Capabilities](/extend/plugin-capabilities) for extension methods.

## Inspect registered plugins

In Console, open Tools or the session Inspector. The Plugins section lists registered plugins and their enabled or disabled state.
The list is available before you create a session. Select Refresh to read the server's current registrations.

The API provides the same list at `GET /plugins`.
To read it on the local managed service, run `wingman api listPlugins`.

## Install a published plugin

Configure the server's release catalog in `wingman.json`:

```json
{
  "plugins": {
    "catalog": "https://your-catalog.example/catalog.json"
  }
}
```

Use the catalog URL supplied by its maintainer. Wingman does not include a hosted catalog endpoint.

The following commands connect to the managed server. Installation happens on the server, including when it runs on another machine.

```sh
wingman plugin search github
wingman plugin install publisher/plugin
wingman plugin list
wingman plugin inspect
wingman plugin update publisher/plugin
wingman plugin remove publisher/plugin
```

Use `publisher/plugin@v1.2.0` to select an exact version. Pinned installations require an explicit version for updates.
Wingman downloads a precompiled executable for the server's platform. Users do not need Go or a custom Wingman build.
Updates validate the replacement before activation. Failed activation preserves the previous selection.
Active runs keep their executable generation until they finish. Removal disables subsequent use and retains executable files and plugin data.

Plugins execute native code with the server's operating-system permissions. A checksum verifies a download against the catalog, not whether the publisher is trustworthy.

## Load a local plugin

Native plugins run as external programs and can add tools, hooks, session actions, model providers, and custom operations.
Put a `wingman-plugin.json` manifest in `~/.config/wingman/plugins/` or one of its immediate subdirectories.
Files ending in `.plugin.json` also load from the plugin directory.
The default plugin path does not follow `XDG_CONFIG_HOME`.

```json
{
  "id": "example.greet",
  "name": "Greeting Plugin",
  "command": ["/absolute/path/to/greet-plugin"]
}
```

Install the program and its runtime before you start Wingman. The manifest starts it directly, without shell expansion.
The program declares its tools during initialization. See [RPC Plugin Protocol](/extend/rpc-plugin-protocol).

Add global directories with `plugins.dirs` in `wingman.json` or a repeated `--plugin-dir` flag.
Disable external plugins with `--no-plugins`.
Project manifests in `<working-directory>/.wingman/plugins/` apply only after you approve that directory:

```sh
wingman plugin trust /absolute/path/to/project
wingman plugin untrust /absolute/path/to/project
```

These paths refer to directories on the server. Approval permits native code execution, not only model tool calls.
Revoking approval prevents new project activations. Already active runs retain their captured plugins until completion.
An unapproved project's plugin manifests do not execute.

A trusted project manifest with the same ID replaces the global definition in that directory. To disable a global external plugin in a project, use:

```json
{
  "id": "publisher/plugin",
  "disabled": true
}
```

## Use and reload tools

Add plugin tool names to an agent's `tools` list.
Stored agent validation uses the global tool catalog. A tool that exists only in a project plugin cannot pass that validation.
An inline agent in `/run` can select project tools for its working directory.

Names must be unique across built-in, plugin, and MCP tools. The name `skill` is reserved.
Plugins run with Wingman's operating system permissions. Install only plugins that you trust.

To read external plugin status on the local managed service, run `wingman api listRPCPlugins`.
To reload external plugins, run `wingman api reloadRPCPlugins`.
If the replacement fails to load, Wingman keeps the current plugins.
These operations use `GET /rpc-plugins` and `POST /rpc-plugins/reload` for global plugin directories.

## Write a Go plugin

The [Go executable plugin guide](/extend/go-executable-plugins) covers the SDK, release format, and publishing workflow.
The SDK handles the internal protocol. Plugin authors register Go handlers and compile a standalone program.

## Embedded Go plugins

Go plugins implement `plugin.Plugin` and register tools or hooks in `Activate`.
Pass them to `session.WithPlugin` when constructing a session.
The standard server does not load Go packages from disk.

Follow [Go Plugin Quickstart](/extend/plugin-quickstart) for a working example.
