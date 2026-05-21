---
title: "Config Schema"
description: "Reference for ~/.config/wingman/wingman.jsonc."
group: "Reference"
order: 1001
---

# Config Schema

Wingman reads global configuration from:

```text
~/.config/wingman/wingman.jsonc
```

This file is for settings that apply across clients. It is not for agents, provider API keys, project-local behavior, or per-client preferences.

## Precedence

Configuration is resolved in this order:

1. Built-in defaults.
2. `~/.config/wingman/wingman.jsonc`.
3. CLI flags passed to `wingman serve` or `wingman up`.

CLI flags always win.

## Format

The file is parsed as JSON with comments:

- `// line comments` are allowed.
- `/* block comments */` are allowed.
- Trailing commas are not allowed.

## Example

```jsonc
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
| `models` | object | no | Model-related defaults. |

Unknown fields are ignored today. Do not rely on ignored fields becoming supported later.

## `server`

| Field | Type | Default | CLI override | Description |
|---|---:|---|---|---|
| `host` | string | `127.0.0.1` | `--host` | Address the HTTP server binds to. |
| `port` | number | `2323` | `--port` | Port the HTTP server listens on. |
| `db` | string | `~/.local/share/wingman/wingman.db` | `--db` | SQLite database path. `~` and `~/...` are expanded. |
| `log_level` | string | `info` | `--log-level` | Log level: `debug`, `info`, `warn`, or `error`. |
| `log_format` | string | `json` | `--log-format` | Log format: `json` or `text`. |

Example:

```jsonc
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

Wingman still includes the default global plugin directory:

```text
~/.config/wingman/plugins/
```

`plugins.dirs` adds more directories. It does not replace the default directory.

Example:

```jsonc
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

There is no config-file equivalent for `--no-plugins` yet.

## `provider`

`provider` is a map keyed by provider ID. It overlays WingModels catalog provider routes at daemon startup. It is not persisted in SQLite and does not store credentials.

Supported provider fields today:

| Field | Type | Required | Description |
|---|---:|---:|---|
| `options` | object | no | Runtime route options for this provider. |

Supported `options` fields today:

| Field | Type | Default | Description |
|---|---:|---|---|
| `baseURL` | string | catalog default | Base URL used for model requests for this provider. |
| `auth` | boolean | `true` | When `false`, Wingman sends no stored or environment credential for this provider route. |

Example:

```jsonc
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

## `models`

| Field | Type | Default | Description |
|---|---:|---|---|
| `default` | string | empty | Reserved default model ref for clients and future server defaults. |

`models.default` is parsed and documented, but the server does not currently apply it to agent creation or message execution. Agents should still set `model_ref`, or callers should pass `model_ref` on message requests.

Example:

```jsonc
{
  "models": {
    "default": "anthropic/claude-sonnet-4-6"
  }
}
```

## Intentionally Excluded

These do not belong in `wingman.jsonc` today:

| Concern | Where it lives |
|---|---|
| Agents | SQLite via `/agents` API. |
| Provider API keys | Provider auth store via `/provider/auth`. |
| Provider definitions | WingModels catalog TOML. |
| Sessions and message history | SQLite via session APIs. |
| Project-specific config | Deferred; no project-local config is supported yet. |
| Per-client preferences | Client-owned state. |

Keeping agents out of JSON avoids ambiguous behavior once multiple clients use the same Wingman daemon. The config file is reserved for daemon-wide defaults.
