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
    "openai": {
      "options": {
        "baseURL": "http://169.254.169.254/gateway/llm/openai/v1",
        "auth": false
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
| `provider` | object | no | Provider route overlays for cataloged providers. |
| `plugins` | object | no | External plugin discovery defaults. |
| `models` | object | no | Reserved model-related defaults. |

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

## `provider`

`provider` is a map keyed by provider ID. It overlays WingModels catalog provider routes at daemon startup. It is not persisted in SQLite and does not store credentials.

Supported provider fields:

| Field | Type | Required | Description |
|---|---:|---:|---|
| `options` | object | no | Runtime route options for this provider. |

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

## Reserved `models` Fields

| Field | Type | Default | Description |
|---|---:|---|---|
| `default` | string | empty | Parsed model ref reserved for future default-model behavior. |

`models.default` is parsed by the server, but it is not currently applied to agent creation or message execution. It is reserved for future default-model behavior.

For now, agents should set `model_ref`, or callers should pass `model_ref` on message requests.

Example:

```json
{
  "models": {
    "default": "anthropic/claude-sonnet-4-6"
  }
}
```
