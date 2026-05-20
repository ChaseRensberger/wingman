---
title: "RPC Plugin Protocol"
description: "Manifest and JSON-RPC protocol for external Wingman tool plugins."
group: "Reference"
order: 1003
---

# RPC Plugin Protocol

RPC plugins are external executables started by Wingman. They are discovered from global plugin directories and communicate with Wingman over newline-delimited JSON-RPC on stdio.

RPC plugins are the right fit when you want to add a custom tool to the stock `wingman serve` binary without rebuilding Wingman.

## Discovery

Wingman always checks the default global plugin directory:

```text
~/.config/wingman/plugins/
```

Add more global plugin directories with config or CLI flags:

```jsonc
{
  "plugins": {
    "dirs": ["~/wingman-plugins"]
  }
}
```

```bash
wingman serve --plugin-dir ~/wingman-plugins
```

Disable external plugin loading with:

```bash
wingman serve --no-plugins
```

## Manifest

A plugin is declared by a `wingman-plugin.json` file. Files ending in `.plugin.json` are also loaded.

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

| Field | Type | Required | Description |
|---|---:|---:|---|
| `id` | string | yes | Stable plugin identifier. |
| `name` | string | no | Human-readable plugin name. |
| `command` | string array | yes | Executable and arguments. Shell expansion is not applied. |
| `tools` | array | no | Tool declarations contributed by this plugin. |

## Tool Schema

Tool input schemas use Wingman's JSON Schema subset:

- Object-shaped inputs.
- Property `type`.
- Optional property `description`.
- Optional property `enum`.
- Optional top-level `required`.

## `tool.execute`

Wingman calls `tool.execute` when the model invokes a plugin tool.

Request:

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

Response:

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

## Minimal Node Plugin

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

## Use The Tool

Plugin tools are selected the same way as built-in tools: include the tool name in an agent's `tools` allow-list.

```json
{
  "name": "Greeter",
  "instructions": "Use greet when the user asks for a greeting.",
  "model_ref": "anthropic/claude-sonnet-4-6",
  "tools": ["greet"]
}
```

## Inspect Loaded Plugins

List loaded plugins and non-fatal load errors:

```bash
curl http://127.0.0.1:2323/plugins/
```

Reload global plugins:

```bash
curl -X POST http://127.0.0.1:2323/plugins/reload
```

External plugins run with the same operating-system permissions as the Wingman process. Install plugins only from sources you trust.
