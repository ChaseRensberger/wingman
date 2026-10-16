---
title: "Global Configuration"
description: "Set server options, provider routes, and shared tools."
---

# Global Configuration

Wingman reads server configuration from:

```text
~/.config/wingman/wingman.json
```

To use a different configuration root, set `XDG_CONFIG_HOME`. For example,
`XDG_CONFIG_HOME=~/settings` uses `~/settings/wingman/wingman.json`.

## Configuration Files

| Setting                                                                                                                      | Location                                                             |
| ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- |
| Server bind address, database path, logs, skill and plugin directories, provider routes, custom provider models, MCP servers | `~/.config/wingman/wingman.json` and CLI flags                       |
| Provider API keys                                                                                                            | SQLite auth store through `PUT /provider/auth`                       |
| External plugin manifests                                                                                                    | `~/.config/wingman/plugins/` plus extra plugin directories           |
| Local Agent Skills                                                                                                           | `~/.config/wingman/skills/` plus project and extra skill directories |

Create agents through the HTTP API or Console, not `wingman.json`.

## File format

Use one JSON object without comments or trailing commas. Unknown fields stop startup.
CLI flags override file values. After file changes, restart a foreground server with its required flags.
For a managed service, run `wingman service start` again. This updates the saved service flags from the file.
Include any explicit flags that you still need. See [Configuration Schema](/reference/config-schema) for supported fields.

## exe.dev Example

The [example configuration](https://github.com/ChaseRensberger/wingman/blob/main/wingman.example.json)
uses local access, text logs, and the exe.dev LLM Gateway.
A gateway forwards model requests to a provider. These gateway addresses work inside an exe.dev box.
On that box, download the example to a new configuration file:

```bash
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/wingman"
mkdir -p "$config_dir"
(set -C; curl -fsSL https://raw.githubusercontent.com/chaserensberger/wingman/main/wingman.example.json > "$config_dir/wingman.json")
```

The command refuses to overwrite an existing configuration file. If the file
already exists, merge the example's `provider` entries into it instead.

Apply the file as described above. OpenAI and Anthropic requests use the gateway without sending stored provider keys.
See [Providers](/configure/providers#exedev-gateway-example) for gateway routes.

## Defaults

Wingman listens on `127.0.0.1:2424`. It stores persistent data in SQLite at:

```text
~/.local/share/wingman/wingman.db
```
