---
title: "Global Config"
description: "Understand where Wingman configuration lives and which page to use next."
order: 3
---

# Global Config

Wingman is configured per local user. Global files live under:

```text
~/.config/wingman/
```

Use global config for daemon-wide settings that apply across clients and projects.

## Configuration Surfaces

Wingman has three main configuration surfaces:

| Concern | Where it lives |
|---|---|
| Server bind address, database path, logs, plugin dirs, provider route overlays | `~/.config/wingman/wingman.json` and CLI flags |
| Provider API keys | SQLite auth store through `PUT /provider/auth` |
| External plugin manifests | `~/.config/wingman/plugins/` plus any extra plugin dirs |

Agents are stored through the HTTP API. They do not live in `wingman.json`.

## Config File

The global config file is:

```text
~/.config/wingman/wingman.json
```

It is strict JSON: comments and trailing commas are not allowed.

Example:

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

CLI flags passed to `wingman serve` or `wingman up` override config file values.

For exact fields, see [Config Schema](/reference/config-schema).

## Common Tasks

| Task | Go to |
|---|---|
| Start the local server or systemd service | [Run the Server](/use-wingman/run-server) |
| Store API keys | [Providers](/configure/providers#store-provider-auth) |
| Route a cataloged provider through a gateway | [Providers](/configure/providers#route-a-provider-through-a-gateway) |
| Choose between `model_ref` and `model_route` | [Models](/configure/models) |
| Load external plugins | [Plugins](/concepts/plugins#external-plugins) |
| Check all supported config fields | [Config Schema](/reference/config-schema) |

## Defaults

Wingman listens on `127.0.0.1:2323` and stores persistent data in SQLite at:

```text
~/.local/share/wingman/wingman.db
```

Use `127.0.0.1` for local-only access. Bind to `0.0.0.0` only on trusted networks; Wingman does not provide inbound auth or multi-tenant isolation.

Run without persistence with:

```bash
wingman serve --ephemeral
```

In ephemeral mode, persisted resources such as agents, sessions, clients, Workspaces, and provider auth are unavailable. Use `POST /run` with an inline agent instead.
