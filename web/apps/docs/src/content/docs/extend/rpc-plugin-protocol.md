---
title: "RPC Plugin Protocol"
description: "Build an external tool plugin using JSON-RPC."
---

# RPC Plugin Protocol

RPC plugins are external programs that exchange JSON-RPC 2.0 messages with Wingman.
Send one JSON object per line through standard input and output. The formatted examples below show message fields, not line breaks to transmit.
Protocol version `2` supports tools, hooks, actions, model providers, custom parts, custom operations, runtime transforms, and sign-in methods.
Use the [Go SDK](/extend/go-executable-plugins) instead of implementing these messages yourself. See [Plugins](/concepts/plugins) for installation and access rules.

## Manifest and initialization

The manifest requires `id` and `command`. Optional `name` and `config` fields supply a display name and plugin-specific configuration.
`command` is an array containing the executable and each argument. Wingman does not expand shell expressions.

Wingman first sends `plugin.initialize`:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "plugin.initialize",
  "params": {
    "host_name": "Wingman",
    "host_version": "v0.1.0",
    "supported_protocol_versions": [2],
    "supported_contribution_kinds": [
      "tools",
      "hooks",
      "actions",
      "providers",
      "methods",
      "parts",
      "transforms",
      "auth"
    ],
    "plugin": { "id": "example.greet", "name": "Greeting Plugin", "config": {} }
  }
}
```

Return the same plugin ID, an offered protocol version, and your tool definitions:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocol_version": 2,
    "plugin": { "id": "example.greet", "name": "Greeting Plugin", "version": "1.0.0" },
    "capabilities": [],
    "contributions": {
      "tools": [
        {
          "name": "greet",
          "description": "Greet someone by name",
          "input_schema": {
            "type": "object",
            "properties": { "name": { "type": "string" } },
            "required": ["name"],
            "additionalProperties": false
          }
        }
      ]
    }
  }
}
```

Each tool needs `name`, `description`, and an object-shaped `input_schema`.
Wingman rejects invalid schemas, duplicate tool names, mismatched identities, and unsupported capabilities.

Optional tool fields are:

| Field              | Purpose                                                     |
| ------------------ | ----------------------------------------------------------- |
| `output_schema`    | Required shape of a successful result's `structured` value. |
| `sequential`       | Run the entire batch in order when it includes this tool.   |
| `directory_scoped` | Require a session working directory.                        |
| `permission`       | Declare `action` and optional input `resource_fields`.      |

## Execute a tool

Wingman can send concurrent requests to one plugin process:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tool.execute",
  "params": {
    "tool": "greet",
    "input": { "name": "Chase" },
    "context": {
      "session_id": "ses_example",
      "run_id": "run_example",
      "work_dir": "/home/me/project"
    }
  }
}
```

Context also supplies available agent, tool-use, provider call, message, part, and model-call IDs.
Return text for the model separately from structured data and display metadata:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "text": "Hello, Chase",
    "structured": { "greeting": "Hello, Chase" },
    "metadata": { "language": "en" }
  }
}
```

`structured` is required only when the tool declares `output_schema`.
Wingman saves `structured` and `metadata` for clients. It does not send them as model instructions.
For failed calls, return a JSON-RPC `error` with `code` and `message` instead of `result`.

## Optional capabilities

Declare only capabilities that your program implements:

| Capability     | Contract                                                                                    |
| -------------- | ------------------------------------------------------------------------------------------- |
| `progress`     | Send `tool.progress` notifications for an active request ID.                                |
| `cancellation` | Handle `$/cancelRequest` with `params.id`. Cancel that request only.                        |
| `health`       | Answer `plugin.health` with a status. Initialization requires a successful health response. |

For progress, send:

```json
{
  "jsonrpc": "2.0",
  "method": "tool.progress",
  "params": {
    "request_id": 2,
    "output_delta": "Looking up a greeting...",
    "metadata": { "stage": "lookup" }
  }
}
```

For `plugin.health`, return `{"status":"ok"}` as the result, with optional `message` and `details`.
Tool progress has a per-request queue limited to 256 notifications and 8 MiB. Overflow fails that request without blocking other RPC responses.
A later failed health response marks the plugin as degraded without immediately removing its tools.
Cancellation must leave the plugin process and other requests running.

## Runtime extension messages

The [runtime reference](/extend/plugin-runtime) describes ownership, limits, and API operations.

| Declaration                | Host request                                                     | Result                                                               |
| -------------------------- | ---------------------------------------------------------------- | -------------------------------------------------------------------- |
| `hooks: ["prompt"]`        | `prompt.prepare` with context, request ID, and message.          | Replacement message string.                                          |
| `transforms: true`         | `domain.transform` with context, domain, and draft value.        | Replacement draft value.                                             |
| `hooks: ["http.request"]`  | `http.request` with request metadata and body availability.      | Replacement exchange metadata.                                       |
| `hooks: ["http.response"]` | `http.response` with response metadata and body availability.    | Replacement exchange metadata.                                       |
| `auth: [...]`              | `auth.execute` with method, operation, input, and private state. | Status, public instructions, private state, and optional credential. |

Transform domains are `agents`, `providers`, `models`, `mcp`, `skills`, and `references`.
`provider.prepare` and `provider.generate` include `options` with configured provider routing options, including `baseURL`, `query`, `authHeader`, and `authScheme`.
HTTP exchange metadata includes method, URL, headers, status, content length, and body availability.
A nonempty `body_id` identifies a plugin-owned replacement stream.
Wingman reads that stream with `http.body.read` and releases it with `http.body.close`.
Each read returns base64-encoded `data` and an `eof` boolean.

Plugin-to-host requests use `host.call` with an active `invocation_id` or a background `scope_token`.
Within an HTTP callback or body read, the `http.body.read` host operation reads the source stream.
`http.body.bind` grants source reads for the exchange lifetime, including asynchronous body transforms.
This grant permits only source reads and `scope.release`, not general host operations.
The `events.subscribe`, `events.next`, `events.close`, and `events.publish` host operations manage client-owned events.
`scope.bind` creates a background grant. `scope.release` removes that grant.

## Logs and shutdown

Reserve standard output for JSON-RPC. Write diagnostics to standard error, or send `plugin.log` notifications with `message`, optional `level`, and optional `fields`.
Wingman keeps a limited buffer of recent diagnostics.

If the process exits during a tool call, that call fails. Later calls fail until the plugin reloads.
Wingman does not repeat the failed call because it can already have changed external state.

For replacement, Wingman loads the new plugin set before retiring the old one.
Active runs retain their tools and hooks from the old generation until completion.
After those runs release the generation, Wingman drains remaining calls within a timeout and sends `plugin.shutdown`.
It waits for the shutdown response, then closes standard input and waits for exit. If the deadline expires, Wingman kills the process.
Shutdown errors propagate to the host.
If loading the replacement fails, the old plugins remain active.
Managed package changes and rollback exclude new scope creation until activation succeeds or the previous manifest is restored.
