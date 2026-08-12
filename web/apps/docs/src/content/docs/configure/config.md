---
title: "Global Config"
description: "Understand where Wingman configuration lives and which page to use next."
order: 3
---

# Global Config

Wingman uses per-user configuration. By default, configuration files are in:

```text
~/.config/wingman/
```

Use this directory for daemon-wide configuration.

> **Security:** The managed service uses private generated credentials in
> `~/.config/wingman/service.env`. Authentication does not provide tenant
> isolation.

To use a different config root, set `XDG_CONFIG_HOME`. For example,
`XDG_CONFIG_HOME=~/settings` uses `~/settings/wingman/wingman.json`.

## Configuration Surfaces

Wingman has three main configuration locations:

| Concern | Where it lives |
|---|---|
| Server bind address, database path, logs, plugin dirs, provider routes, custom provider models, MCP servers | `~/.config/wingman/wingman.json` and CLI flags |
| Provider API keys | SQLite auth store through `PUT /provider/auth` |
| External plugin manifests | `~/.config/wingman/plugins/` plus any extra plugin dirs |

The HTTP API stores agents. Agents are not in `wingman.json`.

## Config File

The global configuration file is:

```text
~/.config/wingman/wingman.json
```

The file uses strict JSON. Comments cause startup to fail. Trailing commas cause
startup to fail. Trailing JSON values cause startup to fail. Unknown keys cause
startup to fail. The error identifies the configuration file.

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

Flags passed to `wingman serve` or `wingman service start` override configuration values.

For exact fields, see [Config Schema](/reference/config-schema).

## Common Tasks

| Task | Go to |
|---|---|
| Start the local server or managed service | [Run the Server](/use-wingman/run-server) |
| Store API keys | [Providers](/configure/providers#store-provider-auth) |
| Route a cataloged provider through a gateway | [Providers](/configure/providers#route-a-provider-through-a-gateway) |
| Add a reusable custom provider/model | [Providers](/configure/providers#add-a-custom-provider) |
| Choose between `model_ref` and `model_route` | [Models](/configure/models) |
| Load external plugins | [Plugins](/concepts/plugins#external-plugins) |
| Connect MCP servers and tools | [MCP Servers](/configure/mcp) |
| Check all supported config fields | [Config Schema](/reference/config-schema) |

## Defaults

Wingman listens on `127.0.0.1:2323`. It stores persistent data in SQLite at:

```text
~/.local/share/wingman/wingman.db
```
