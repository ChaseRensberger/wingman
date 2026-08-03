---
title: "Execution Scopes"
description: "How Wingman shares and releases runtime resources by working directory."
group: "Core"
order: 102
---

# Execution Scopes

An execution scope is the resource boundary for sessions that run in the same
canonical working directory. Sessions without a working directory use the
directoryless scope.

Each scope owns:

- An external RPC plugin process generation.
- MCP client connections and their discovered tools.
- A composed tool catalog.

All scopes share the daemon's immutable provider and model catalog.

Wingman resolves absolute paths and symbolic links before it selects a scope.
Two paths that identify the same directory share resources. A missing path or a
path to a file causes session construction to fail.

## Session Pinning

A session acquires its scope before it resolves models or tools. The session
keeps that scope until `Session.Close` completes.

Persistent queued runs use the working-directory snapshot stored at admission.
A later Workspace or session edit does not move admitted work to another scope.

After the last session releases a directory scope, Wingman keeps it idle for one
minute. It then closes MCP connections and plugin processes. The directoryless
scope stays active for daemon management APIs.

## Catalog Generations

Provider configuration produces an immutable provider and model catalog when the
application starts. It does not change the embedded WingModels catalog.
Separate embedded applications can use different provider configurations without
sharing models or routes.

Plugin and MCP tools bind to the process or connection generation that supplied
them. Replacement publishes a complete candidate before it retires the old
generation. Calls that already started can drain. New calls through stale tools
fail instead of moving silently to a new process or connection.

Project configuration files and live configuration watching are not supported.
The global `wingman.json` file remains the only authored daemon configuration.
