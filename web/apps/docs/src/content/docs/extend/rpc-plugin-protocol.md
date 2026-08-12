---
title: "RPC Plugin Protocol"
description: "Manifest and JSON-RPC protocol for external Wingman tool plugins."
group: "Reference"
order: 1003
---

# RPC Plugin Protocol

RPC plugins are external executables that Wingman supervises. They exchange
newline-delimited JSON-RPC 2.0 messages with Wingman over stdin and stdout.
Protocol version 1 supports tool contributions.

Use RPC plugins when the stock `wingman serve` binary must load a polyglot,
out-of-process extension. RPC isolates Wingman from plugin crashes. RPC is not an
OS security sandbox. The plugin runs with the same operating-system permissions
as Wingman.

## Discovery

Wingman loads global plugins from:

```text
~/.config/wingman/plugins/
```

Add global directories with configuration or CLI flags:

```json
{
  "plugins": {
    "dirs": ["/home/me/wingman-plugins"]
  }
}
```

```bash
wingman serve --plugin-dir /home/me/wingman-plugins
```

For a session with a working directory, Wingman also loads manifests from
`<work_dir>/.wingman/plugins/`. If a project-plugin generation fails, the session
does not start. The previous plugin generation remains active.

Disable external plugins with `wingman serve --no-plugins`.

## Bootstrap Manifest

Name a manifest `wingman-plugin.json` or use the suffix `.plugin.json`.

```json
{
  "id": "example.greet",
  "name": "Greeting Plugin",
  "command": ["node", "/absolute/path/to/greet-plugin.js"],
  "config": {
    "language": "en"
  }
}
```

| Field | Type | Required | Description |
|---|---:|---:|---|
| `id` | string | yes | Stable plugin identifier. The initialized process must return this exact ID. |
| `name` | string | no | Bootstrap display name. Initialized process metadata is authoritative. |
| `command` | string array | yes | Executable and arguments. Wingman does not use shell expansion. |
| `config` | object | no | Plugin-specific configuration sent during initialization. |

The manifest starts only the process. The initialization result provides tools
and capabilities.

## Initialization

The first host request is `plugin.initialize`:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "plugin.initialize",
  "params": {
    "host_name": "Wingman",
    "host_version": "v0.1.0",
    "supported_protocol_versions": [1],
    "supported_contribution_kinds": ["tools"],
    "plugin": {
      "id": "example.greet",
      "name": "Greeting Plugin",
      "config": {
        "language": "en"
      }
    }
  }
}
```

The plugin selects one offered protocol version. It returns its authoritative
identity, capabilities, and contributions:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocol_version": 1,
    "plugin": {
      "id": "example.greet",
      "name": "Greeting Plugin",
      "version": "1.0.0"
    },
    "capabilities": ["cancellation", "progress", "health"],
    "contributions": {
      "tools": [
        {
          "name": "greet",
          "description": "Greet someone by name",
          "input_schema": {
            "type": "object",
            "properties": {
              "name": { "type": "string" }
            },
            "required": ["name"],
            "additionalProperties": false
          }
        }
      ]
    }
  }
}
```

Wingman rejects the complete candidate generation if initialization fails. It
also rejects the generation if the protocol or plugin ID does not match. It
rejects unsupported capabilities, invalid schemas, and duplicate contribution names.

## Capabilities

| Capability | Plugin behavior |
|---|---|
| `cancellation` | Handle `$/cancelRequest` notifications. |
| `progress` | Send `tool.progress` notifications for active tool requests. |
| `health` | Implement `plugin.health`. Initialization includes a required health check. |

Do not return capabilities that the plugin does not implement. Protocol version
1 rejects unknown capabilities.

## Tool Contributions

Every tool requires `name`, `description`, and an object-shaped `input_schema`.
Input and output values use JSON Schema. Wingman compiles both schemas before it
publishes the generation.

| Field | Type | Description |
|---|---:|---|
| `output_schema` | JSON Schema object | Required shape of a successful result's `structured` value. |
| `sequential` | boolean | Run a model-request batch sequentially when it contains this tool. |
| `directory_scoped` | boolean | Require the session to have a working directory. |
| `permission` | object | Permission `action` and optional input `resource_fields`. |

## Tool Execution

Wingman can send concurrent `tool.execute` requests to one plugin process.

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tool.execute",
  "params": {
    "tool": "greet",
    "input": { "name": "Chase" },
    "context": {
      "session_id": "ses_123",
      "run_id": "run_123",
      "agent_id": "agt_123",
      "tool_use_id": "tlu_123",
      "call_id": "call_123",
      "message_id": "msg_123",
      "part_id": "prt_123",
      "model_call_id": "mcl_123",
      "work_dir": "/home/chase/project"
    }
  }
}
```

Return model-facing text separately from structured content and client metadata:

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

| Result field | Type | Required | Description |
|---|---:|---:|---|
| `text` | string | no | Model-facing tool output. |
| `structured` | any JSON value | when `output_schema` is declared | Structured result validated by Wingman. |
| `metadata` | object | no | Client-facing data persisted with the result. |

Do not put model instructions in `metadata`. Providers do not receive metadata
as tool output.

Return JSON-RPC errors for failed calls:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "error": {
    "code": -32602,
    "message": "name is required"
  }
}
```

## Progress And Cancellation

With the `progress` capability, report progress for the active request ID:

```json
{
  "jsonrpc": "2.0",
  "method": "tool.progress",
  "params": {
    "request_id": 2,
    "output_delta": "Looking up greeting preferences...",
    "metadata": { "stage": "lookup" }
  }
}
```

With the `cancellation` capability, Wingman sends this notification when the
request context ends:

```json
{
  "jsonrpc": "2.0",
  "method": "$/cancelRequest",
  "params": { "id": 2 }
}
```

Cancel only that request. Keep the plugin process and other requests running.

## Health And Diagnostics

With the `health` capability, implement `plugin.health`:

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "plugin.health"
}
```

```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "status": "ok",
    "message": "ready"
  }
}
```

After initialization, a failed health check marks the plugin as degraded. It
does not immediately remove the plugin tools.

Write human-readable diagnostics to stderr, or send structured `plugin.log`
notifications:

```json
{
  "jsonrpc": "2.0",
  "method": "plugin.log",
  "params": {
    "level": "info",
    "message": "cache refreshed",
    "fields": { "entries": 42 }
  }
}
```

Wingman keeps a bounded recent diagnostic buffer. Stdout is reserved for JSON-RPC.
If a plugin process exits during `tool.execute`, the active call fails. The
plugin becomes `failed`. Later calls through that generation are rejected.
Wingman records a bounded process-exit diagnostic. It does not retry the call
because the plugin can complete an external side effect before exit.
Reload the plugin to stage a new generation.

## Shutdown And Replacement

Before retirement, Wingman stops new generation-bound calls. It waits for active
calls to drain within a bounded timeout. It then sends `plugin.shutdown`. It
closes stdin and waits for the process to exit. Wingman kills the process if the
shutdown deadline expires.

Reload builds and validates a complete candidate generation before publication.
If candidate startup or validation fails, Wingman keeps the previous generation
active. After a successful atomic swap, it retires the previous generation.

## Inspect Plugins

List plugin status, capabilities, health, process data, contributions, and
recent diagnostics. These commands use HTTP Basic authentication. Load
managed-service credentials with `source ~/.config/wingman/service.env`; the
username defaults to `wingman`. See [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl http://127.0.0.1:2323/plugins \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}"
```

Reload global and accepted project plugin directories:

```bash
curl -X POST http://127.0.0.1:2323/plugins/reload \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}"
```

Install plugins only from sources you trust.
