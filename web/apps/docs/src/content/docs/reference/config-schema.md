---
title: "Config Schema"
description: "Reference for ~/.config/wingman/wingman.json."
group: "Reference"
order: 1001
---

# Config Schema

Wingman reads global configuration from:

```text
~/.config/wingman/wingman.json
```

This file is for daemon-wide settings that apply across clients.

Set `XDG_CONFIG_HOME` to change the config root. For example, when it is set to
`~/settings`, Wingman reads `~/settings/wingman/wingman.json`. The default path
is the same on Linux and macOS.

## Precedence

Configuration is resolved in this order:

1. Built-in defaults.
2. `~/.config/wingman/wingman.json`.
3. CLI flags passed to `wingman serve` or `wingman up`.

CLI flags always win.

## Format

The file is parsed as strict JSON:

- Comments are not allowed.
- Trailing commas are not allowed.

## Example

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 2323,
    "db": "~/.local/share/wingman/wingman.db",
    "log_level": "info",
    "log_format": "json"
  },
  "provider": {
    "exe-openai": {
      "name": "exe.dev OpenAI Gateway",
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      },
      "models": {
        "gpt-5.6-terra": {
          "api": "openai_responses",
          "context_window": 1050000,
          "max_output": 128000,
          "capabilities": {
            "tools": true,
            "images": true,
            "reasoning": true,
            "structured_output": true
          }
        }
      }
    }
  },
  "plugins": {
    "dirs": ["~/.config/wingman/plugins"]
  }
}
```

## Top-Level Object

| Field | Type | Required | Description |
|---|---:|---:|---|
| `server` | object | no | Server defaults used by `wingman serve` and `wingman up`. |
| `provider` | object | no | Provider route overlays and config-defined provider/model metadata. |
| `mcp` | object | no | Configured Model Context Protocol servers. |
| `plugins` | object | no | External plugin discovery defaults. |
| `models` | object | no | Reserved model-related defaults. |
| `permissions` | string, object, or rule array | no | Daemon-wide tool permission rules. |
| `agent_permissions` | object | no | Daemon-local permission overlays keyed by agent ID or name. |

Only the documented fields are supported.

## `server`

| Field | Type | Default | CLI override | Description |
|---|---:|---|---|---|
| `host` | string | `127.0.0.1` | `--host` | Address the HTTP server binds to. |
| `port` | number | `2323` | `--port` | Port the HTTP server listens on. |
| `db` | string | `~/.local/share/wingman/wingman.db` | `--db` | SQLite database path. `~` and `~/...` are expanded. |
| `log_level` | string | `info` | `--log-level` | Log level: `debug`, `info`, `warn`, or `error`. |
| `log_format` | string | `json` | `--log-format` | Log format: `json` or `text`. |

Example:

```json
{
  "server": {
    "host": "127.0.0.1",
    "port": 2424,
    "db": "~/wingman-dev.db",
    "log_level": "debug",
    "log_format": "text"
  }
}
```

## `plugins`

| Field | Type | Default | CLI override | Description |
|---|---:|---|---|---|
| `dirs` | string array | `[]` | `--plugin-dir` | Extra global plugin directories. Each path supports `~` and `~/...` expansion. |

Wingman includes the default global plugin directory:

```text
~/.config/wingman/plugins/
```

`plugins.dirs` adds more directories. It does not replace the default directory.

Example:

```json
{
  "plugins": {
    "dirs": [
      "~/wingman-plugins",
      "/opt/wingman/plugins"
    ]
  }
}
```

Disable external plugin loading entirely with the CLI flag:

```bash
wingman serve --no-plugins
```

There is no config-file equivalent for `--no-plugins`.

## `mcp`

`mcp` is a map of Model Context Protocol server names to server definitions. Enabled servers connect when Wingman starts.

| Field | Type | Required | Description |
|---|---:|---:|---|
| `type` | string | yes | `local` for a stdio subprocess or `remote` for an HTTP MCP endpoint. |
| `command` | string array | local | Executable followed by arguments for a local server. |
| `cwd` | string | no | Working directory for a local server. Supports `~` and `~/...` expansion. |
| `environment` | object | no | Environment variables supplied to a local server. |
| `url` | string | remote | Remote MCP endpoint URL. Wingman tries streamable HTTP, then SSE. |
| `headers` | object | no | HTTP headers supplied to a remote server. |
| `enabled` | boolean | no | Whether the server connects at startup. Defaults to `true`. |
| `timeout` | number | no | Connection and tool-discovery timeout in milliseconds. Defaults to `30000`. |

See [MCP Servers](/configure/mcp) for local and remote examples, status checks, and current limits.

## `permissions`

`permissions` defines daemon-wide tool policy. It is evaluated at run time and is not written into stored agents.

Supported effects are `allow`, `ask`, and `deny`.

Example:

```json
{
  "permissions": {
    "read": "allow",
    "grep": "allow",
    "glob": "allow",
    "edit": "ask",
    "bash": {
      "*": "ask",
      "go test *": "allow",
      "git push *": "deny"
    }
  }
}
```

See [Permissions](/configure/permissions) for actions, resources, precedence, and current limits.

Permission failures include structured tool-result metadata so clients can render policy failures without parsing text output.

## `agent_permissions`

`agent_permissions` overlays daemon-local rules onto stored SQLite agents by agent ID or name.

Example:

```json
{
  "agent_permissions": {
    "Plan": {
      "edit": "deny",
      "bash": "deny"
    }
  }
}
```

ID-specific overlays run after name-specific overlays when both match.

## `provider`

`provider` is a map keyed by provider ID. It overlays WingModels catalog provider routes and can define custom providers and models at daemon startup. It is not persisted in SQLite and does not store credentials.

Supported provider fields:

| Field | Type | Required | Description |
|---|---:|---:|---|
| `name` | string | no | Display name for a config-defined provider. Defaults to the provider ID for unknown providers. |
| `auth_types` | object array | no | Auth methods exposed through `/provider`. Defaults to one `api_key` auth type for unknown providers. |
| `options` | object | no | Runtime route options for this provider. |
| `models` | object | no | Model metadata keyed by model ID. Required if this is a new provider you want to select from the API or web UI. |

Supported `options` fields:

| Field | Type | Default | Description |
|---|---:|---|---|
| `baseURL` | string | catalog default | Base URL used for model requests for this provider. |
| `auth` | boolean | `true` | When `false`, Wingman sends no stored or environment credential for this provider route. |

Example:

```json
{
  "provider": {
    "openai": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      }
    }
  }
}
```

Omit `auth` for normal providers. Set it to `false` for unauthenticated gateways or local endpoints.

Supported model fields under `provider.<id>.models.<model-id>`:

| Field | Type | Required | Description |
|---|---:|---:|---|
| `provider` | string | no | Provider ID. Defaults to the enclosing provider key. |
| `id` | string | no | Model ID. Defaults to the enclosing model key. |
| `api` | string | yes | Wire protocol. One of `openai_responses`, `openai_completions`, or `anthropic_messages`. |
| `base_url` | string | no | Model-specific base URL. Defaults to `provider.<id>.options.baseURL` when present. |
| `env` | string array | no | Environment variables checked for credentials when auth is enabled. |
| `context_window` | number | no | Context window used for UI/API metadata and context usage percentage. |
| `max_output` | number | no | Maximum output tokens used for UI/API metadata. |
| `capabilities` | object | no | Capability flags for runtime gating and UI metadata. |
| `input_cost_per_mtok` | number | no | Input cost metadata per million tokens. |
| `output_cost_per_mtok` | number | no | Output cost metadata per million tokens. |

Supported capability flags:

| Field | Type | Description |
|---|---:|---|
| `tools` | boolean | Model can use tools. |
| `images` | boolean | Model accepts image inputs. |
| `reasoning` | boolean | Model can emit reasoning parts. |
| `structured_output` | boolean | Model supports structured output constraints. |

Custom provider example:

```json
{
  "provider": {
    "exe-openai": {
      "name": "exe.dev OpenAI Gateway",
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      },
      "models": {
        "gpt-5.6-terra": {
          "api": "openai_responses",
          "context_window": 1050000,
          "max_output": 128000,
          "capabilities": {
            "tools": true,
            "images": true,
            "reasoning": true,
            "structured_output": true
          }
        }
      }
    }
  }
}
```

Route overlay example for an existing catalog provider:

```json
{
  "provider": {
    "openai": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
      }
    }
  }
}
```
