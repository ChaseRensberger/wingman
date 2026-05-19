---
title: "Configure Wingman"
description: "Use ~/.config/wingman, plugins, provider auth, server flags, and service settings."
order: 3
---

# Configure Wingman

Wingman uses the normal Linux user config location for user-level files:

```text
~/.config/wingman/
```

That directory is where global plugins live, and it is the right place for user-owned Wingman configuration such as `wingman.json`. Runtime server settings are still controlled by `wingman serve` or `wingman up` flags, and provider credentials are stored through the HTTP API so secrets do not need to live in plain config files.

## Config Directory

A typical user config directory looks like this:

```text
~/.config/wingman/
├── wingman.json
└── plugins/
    └── example-plugin/
        └── wingman-plugin.json
```

Use this directory for files that should apply across projects. Use project-local `.wingman/` directories for configuration that should move with a repository.

The stock `wingman` server currently reads external plugins from `~/.config/wingman/plugins/`. Server runtime options are passed as CLI flags. If you keep a `wingman.json`, treat it as the user-level config file for clients, wrappers, scripts, or future Wingman config surfaces.

## `wingman.json`

`~/.config/wingman/wingman.json` is the conventional place for non-secret user preferences.

Do put durable preferences here:

- Default server URL.
- Preferred client ID.
- Default model or provider preference.
- Paths to local project folders or plugin directories.
- UI/client preferences.

Do not put provider API keys here. Store model provider credentials through `/provider/auth` instead.

Example:

```json
{
  "server": {
    "base_url": "http://localhost:2323"
  },
  "defaults": {
    "model_ref": "anthropic/claude-sonnet-4-6",
    "client_id": "local-cli"
  },
  "plugins": {
    "dirs": ["~/.config/wingman/plugins"]
  }
}
```

The important split is:

| Concern | Where it lives |
|---|---|
| Server bind address, database path, log level | `wingman serve` / `wingman up` flags |
| Provider API keys | `PUT /provider/auth` |
| Global external plugin manifests | `~/.config/wingman/plugins/` |
| Project-local plugin manifests | `<working-directory>/.wingman/plugins/` |
| Non-secret client/user preferences | `~/.config/wingman/wingman.json` |

## Server Address

By default, Wingman listens on `127.0.0.1:2323`.

```bash
wingman serve
```

Change the bind address with `--host` and `--port`:

```bash
wingman serve --host 0.0.0.0 --port 2424
```

Use `127.0.0.1` for local-only access. Use `0.0.0.0` only when you intentionally want other machines on the network to reach the server.

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

Ephemeral mode does not persist sessions, messages, agents, or provider credentials. Use it for throwaway local testing, not a normal long-running install.

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

When using WingModels directly as a Go SDK, the provider client can also read provider keys from environment variables such as `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, and `OPENCODE_API_KEY`. The Wingman server path should prefer the local auth store so clients do not need access to your shell environment.

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

For custom or not-yet-cataloged models, pass `model_route` when creating or updating an agent. See [WingModels](/core/wingmodels#custom-models) for the supported route shape.

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

Minimal manifest:

```json
{
  "id": "example.greet",
  "name": "Greeting Plugin",
  "command": ["node", "/home/you/.config/wingman/plugins/greet/greet-plugin.js"],
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

The `command` array is executed directly. Shell expansion is not applied, so use absolute paths or pass every argument explicitly.

Sessions also load project-local plugins from the session working directory:

```text
<working-directory>/.wingman/plugins/
```

Project-local plugins are useful when a repository needs its own tools. Global plugins are better for tools you want available everywhere.

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

See [Plugins](/core/plugins) for plugin manifest and protocol details.

## Logs

Wingman logs in JSON at `info` level by default.

```bash
wingman serve --log-format json --log-level info
```

Use text logs while developing locally:

```bash
wingman serve --log-format text --log-level debug
```

Supported log levels are `debug`, `info`, `warn`, and `error`.

## Web UI Development

Use `--ui-dev` when developing the web UI. Requests under `/web` are proxied to the Vite dev server:

```bash
wingman serve --ui-dev http://localhost:5173
```

You do not need this flag for normal CLI or API usage.

## Systemd Service

On Linux, `wingman up` installs and starts `wingman.service`. It accepts the same runtime flags as `wingman serve` and writes them into the generated systemd unit:

```bash
wingman up --host 127.0.0.1 --port 2323 --log-level info
```

Inspect the running service:

```bash
wingman status
```

Stop and remove the service:

```bash
wingman down
```

The service runs as the user who invoked `wingman up`, so the default database and plugin paths stay under that user's home directory.

## Common Setups

Local persistent server:

```bash
wingman serve
```

Local development with readable logs:

```bash
wingman serve --log-format text --log-level debug
```

Temporary test server:

```bash
wingman serve --ephemeral --log-format text
```

Persistent systemd service on the default port:

```bash
wingman up
```
