---
title: "Global Config"
description: "Configure the local server, storage, logging, plugins, and provider route defaults."
order: 3
---

# Global Config

Wingman is configured globally for the current user. The user config directory is:

```text
~/.config/wingman/
```

Use that directory for settings that apply across clients and projects.

## Current Configuration Surfaces

Wingman has three configuration surfaces:

| Concern | Where it lives |
|---|---|
| Server bind address, database path, logging, plugin dirs, provider route overlays | `~/.config/wingman/wingman.json` and CLI flags |
| Provider API keys | `PUT /provider/auth` |
| Global external plugin manifests | `~/.config/wingman/plugins/` |

Agents are stored in SQLite through the HTTP API. They do not live in a JSON config file.

## Global Config File

The config file is:

```text
~/.config/wingman/wingman.json
```

It contains values that do not change between clients: server defaults, storage path, logging, plugin directories, and provider route overlays.

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

CLI flags override config values. Provider secrets stay in the provider auth store, not in JSON config.

The file must be valid JSON. Comments and trailing commas are not allowed.

Use [Providers](/configure/providers) for provider auth and route behavior. Use [Config Schema](/reference/config-schema) for the exact supported fields.

## Server Address

By default, Wingman listens on `127.0.0.1:2323`.

```bash
wingman serve
```

Change the bind address with `--host` and `--port`:

```bash
wingman serve --host 0.0.0.0 --port 2424
```

Use `127.0.0.1` for local-only access. Use `0.0.0.0` only on trusted networks; Wingman does not provide inbound auth or multi-tenant isolation.

## Storage

Wingman stores persistent data in SQLite. The default database path is:

```text
~/.local/share/wingman/wingman.db
```

Use `--db` to choose a different path:

```bash
wingman serve --db ./wingman.db
```

Run without persistence with `--ephemeral`:

```bash
wingman serve --ephemeral
```

Ephemeral mode does not persist sessions, messages, agents, clients, or provider credentials. Use it for one-shot local runs and embedding scenarios, not a normal long-running install.

## Provider Auth

Model providers need credentials before Wingman can call them. Provider API keys are stored in SQLite through `/provider/auth`, not in `wingman.json`.

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

Check configured provider status:

```bash
curl -sS http://localhost:2323/provider/auth | jq
```

The status response reports whether a provider is configured, but it does not return the secret. See [Providers](/configure/providers) for deleting credentials, environment fallback, and gateway routing.

## Provider Route Overlays

Provider route overlays change where a cataloged provider sends requests. They are process configuration, not SQLite data. They do not create provider records, store secrets, or change persisted agents.

For example, this routes `openai/*` model refs through the exe.dev LLM Gateway and disables stored/env auth for that provider route:

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

With that config, agents can keep normal catalog model refs such as `openai/gpt-5.5`. See [Providers](/configure/providers#auth-behavior) for `auth` behavior and gateway examples.

## Model Selection

Agents usually select a model with `model_ref`:

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "model_ref": "anthropic/claude-sonnet-4-6",
  "options": { "max_tokens": 4096 }
}
```

The model ref format is:

```text
provider/model
```

Examples include `anthropic/claude-sonnet-4-6`, `openai/gpt-5.5`, and `opencode/claude-sonnet-4-6`.

For custom models, pass `model_route` when creating or updating an agent, or when sending a message. Prefer provider route overlays for cataloged providers; `model_route` is the per-agent/per-request override. See [Models](/configure/models) for choosing between provider overlays and `model_route`.

## Plugins

Wingman loads external plugins from the global plugin directory:

```text
~/.config/wingman/plugins/
```

Each plugin can live directly in that directory or inside its own subdirectory. A plugin is discovered by its manifest file:

```text
~/.config/wingman/plugins/
└── greet/
    ├── wingman-plugin.json
    └── greet-plugin.js
```

Add another global plugin directory with `--plugin-dir`:

```bash
wingman serve --plugin-dir /path/to/plugins
```

The flag can be repeated:

```bash
wingman serve --plugin-dir ./team-plugins --plugin-dir ./local-plugins
```

Disable external plugin loading entirely with:

```bash
wingman serve --no-plugins
```

See [Plugins](/concepts/plugins) for plugin manifest and protocol details.

## Logs

Wingman logs in JSON at `info` level by default.

```bash
wingman serve --log-format json --log-level info
```

Use text logs while developing locally:

```bash
wingman serve --log-format text --log-level debug
```

## System Service

`wingman up` installs and starts Wingman as a systemd service:

```bash
sudo wingman up
```

Pass server flags to bake them into the service:

```bash
sudo wingman up --host 127.0.0.1 --port 2323 --db /var/lib/wingman/wingman.db
```

Restart the service after editing `~/.config/wingman/wingman.json`:

```bash
wingman restart
```

Check and remove the service with:

```bash
wingman status
sudo wingman down
```
