---
title: "Configure Wingman"
description: "Configure the local server, provider auth, plugins, and storage."
order: 3
---

# Configure Wingman

Wingman is configured globally for the current user. The user config directory is:

```text
~/.config/wingman/
```

Use that directory for settings that apply across clients and projects. Project-local `.wingman/` config is intentionally deferred; do not rely on it for current installs.

## Current Configuration Surfaces

Wingman has three configuration surfaces:

| Concern | Where it lives |
|---|---|
| Server bind address, database path, logging, plugin dirs, provider route overlays | `~/.config/wingman/wingman.jsonc` and CLI flags |
| Provider API keys | `PUT /provider/auth` |
| Global external plugin manifests | `~/.config/wingman/plugins/` |

Agents are stored in SQLite through the HTTP API. They do not live in a JSON config file.

## Global Config File

The config file is:

```text
~/.config/wingman/wingman.jsonc
```

It contains values that do not change between clients: server defaults, storage path, logging, plugin directories, and provider route overlays.

Example:

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

CLI flags override config values. Provider secrets stay in the provider auth store, not in JSON config.

The parser accepts JSON with `//` and `/* ... */` comments. Do not use trailing commas.

See [Config Schema](/reference/config-schema) for the exact supported fields.

## Server Address

By default, Wingman listens on `127.0.0.1:2323`.

```bash
wingman serve
```

Change the bind address with `--host` and `--port`:

```bash
wingman serve --host 0.0.0.0 --port 2424
```

Use `127.0.0.1` for local-only access. Use `0.0.0.0` only when you intentionally want other machines on the network to reach the server. Wingman does not currently provide inbound auth or multi-tenant isolation.

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

Model providers need credentials before Wingman can call them. Store API keys in Wingman's local auth store with `PUT /provider/auth`:

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

Check configured provider status:

```bash
curl -sS http://localhost:2323/provider/auth | jq
```

The status response reports whether a provider is configured, but it does not return the secret.

Remove a provider credential with:

```bash
curl -sS -X DELETE http://localhost:2323/provider/auth/anthropic
```

When using WingModels directly as a Go SDK, provider clients can also read provider keys from environment variables such as `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and `OPENCODE_API_KEY`. The Wingman server path should prefer the local auth store so clients do not need access to your shell environment.

## Provider Route Overlays

Provider route overlays change where a cataloged provider sends requests. They are process configuration, not SQLite data. They do not create provider records and do not change persisted agents.

For example, this routes `openai/*` model refs through the exe.dev LLM Gateway and disables stored/env auth for that provider route:

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

With that config, an agent can keep a normal catalog model ref:

```json
{
  "name": "Assistant",
  "instructions": "Be helpful and concise.",
  "model_ref": "openai/gpt-5.5"
}
```

Omitting `auth` uses Wingman's normal auth resolution: stored `/provider/auth` credentials first, then catalog environment variables. Set `auth` to `false` only for unauthenticated gateways or local endpoints where Wingman should not send any provider credential.

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

For custom or not-yet-cataloged models, pass `model_route` when creating or updating an agent, or when sending a message. Prefer provider route overlays for cataloged providers; `model_route` is the per-agent/per-request escape hatch. See [WingModels](/concepts/wingmodels#custom-models) for the supported route shape.

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

Check and remove the service with:

```bash
wingman status
sudo wingman down
```
