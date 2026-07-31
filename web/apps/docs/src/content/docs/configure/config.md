---
title: "Global Config"
description: "Understand where Wingman configuration lives and which page to use next."
order: 3
---

# Global Config

Wingman is configured per local user. By default, global files live under:

```text
~/.config/wingman/
```

Use global config for daemon-wide settings that apply across clients and projects.

> **Security:** (Currently) Wingman is a trusted-local control surface, not an authenticated multi-tenant service. Anyone who can reach it can use configured providers, inspect local directories, manage plugins or MCP connections, and start agents with enabled tools. Keep the server on a trusted local interface; `X-Wingman-Client` provides attribution only, not isolation.

Set `XDG_CONFIG_HOME` to use a different config root. For example,
`XDG_CONFIG_HOME=~/settings` makes the global config file
`~/settings/wingman/wingman.json`. This behavior is the same on Linux and
macOS.

## Configuration Surfaces

Wingman has three main configuration surfaces:

| Concern | Where it lives |
|---|---|
| Server bind address, database path, logs, plugin dirs, provider routes, custom provider models, MCP servers | `~/.config/wingman/wingman.json` and CLI flags |
| Provider API keys | SQLite auth store through `PUT /provider/auth` |
| External plugin manifests | `~/.config/wingman/plugins/` plus any extra plugin dirs |

Agents are stored through the HTTP API. They do not live in `wingman.json`.

## Config File

The global config file is:

```text
~/.config/wingman/wingman.json
```

It is strict JSON. Comments, trailing commas, trailing JSON values, and unknown
keys cause startup to fail. The error identifies the config file.

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

CLI flags passed to `wingman serve` or `wingman up` override config file values.

For exact fields, see [Config Schema](/reference/config-schema).

## Common Tasks

| Task | Go to |
|---|---|
| Start the local server or systemd service | [Run the Server](/use-wingman/run-server) |
| Store API keys | [Providers](/configure/providers#store-provider-auth) |
| Route a cataloged provider through a gateway | [Providers](/configure/providers#route-a-provider-through-a-gateway) |
| Add a reusable custom provider/model | [Providers](/configure/providers#add-a-custom-provider) |
| Choose between `model_ref` and `model_route` | [Models](/configure/models) |
| Load external plugins | [Plugins](/concepts/plugins#external-plugins) |
| Connect MCP servers and tools | [MCP Servers](/configure/mcp) |
| Check all supported config fields | [Config Schema](/reference/config-schema) |

## Defaults

Wingman listens on `127.0.0.1:2323` and stores persistent data in SQLite at:

```text
~/.local/share/wingman/wingman.db
```
