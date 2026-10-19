---
title: "Configuration Schema"
description: "Fields accepted in wingman.json."
---

# Configuration Schema

Wingman reads `~/.config/wingman/wingman.json`, or `$XDG_CONFIG_HOME/wingman/wingman.json` when set.
Use one JSON object without comments or trailing commas. Unknown configuration fields stop startup.
File values override defaults. CLI flags override file values.
See [Global Configuration](/configure/config) for applying changes.

## Top-level fields

| Field               | Type                          | Purpose                                  |
| ------------------- | ----------------------------- | ---------------------------------------- |
| `server`            | Object                        | Address, database, and logs              |
| `provider`          | Object                        | Provider routes and custom models        |
| `mcp`               | Object                        | MCP servers                              |
| `plugins`           | Object                        | Extra global plugin directories          |
| `skills`            | Object                        | Extra global skill directories           |
| `references`        | Object array                  | Named directories for additional context |
| `permissions`       | String, object, or rule array | Server-wide tool rules                   |
| `agent_permissions` | Object                        | Rules by agent name or ID                |

All top-level fields are optional.

## `server`

| Field        | Type             | Default                             | CLI flag       |
| ------------ | ---------------- | ----------------------------------- | -------------- |
| `host`       | String           | `127.0.0.1`                         | `--host`       |
| `port`       | Integer, 1–65535 | `2424`                              | `--port`       |
| `db`         | String           | `~/.local/share/wingman/wingman.db` | `--db`         |
| `log_level`  | String           | `info`                              | `--log-level`  |
| `log_format` | String           | `json`                              | `--log-format` |

Log levels are `debug`, `info`, `warn`, and `error`. Formats are `json` and `text`.
The database path supports `~` and `~/...`.
Ephemeral mode is a CLI flag, not a file field.

## `plugins`

`dirs` is an array of extra global directory paths. Paths support `~` and `~/...`.
The default `~/.config/wingman/plugins/` remains included and does not follow `XDG_CONFIG_HOME`.
`--plugin-dir` overrides the extra directory list. `--no-plugins` disables external plugins and has no file equivalent.
See [Plugins](/concepts/plugins) for manifests and project directories.

## `skills`

`dirs` is an array of extra global skill directories. Paths support `~` and `~/...`.
The default skill directory remains included and follows `XDG_CONFIG_HOME`.
See [Skills](/configure/skills) for discovery order and file limits.

## `references`

Each entry requires a unique `name` and a `path`. The path must be absolute or start with `~/`.
An optional `description` tells the agent when to use the directory.
References add directory information to the model input without loading file contents or granting file access.

```json
{
  "references": [
    {
      "name": "manuals",
      "path": "~/Documents/manuals",
      "description": "Product manuals and operating procedures"
    }
  ]
}
```

## `mcp`

Map each server name to a definition. See [MCP Servers](/configure/mcp) for examples.

| Field               | Type         | Purpose                                                                |
| ------------------- | ------------ | ---------------------------------------------------------------------- |
| `type`              | String       | Required. `local` or `remote`.                                         |
| `command`           | String array | Required for local servers. Executable followed by arguments.          |
| `cwd`               | String       | Local working directory. Supports `~` and `~/...`.                     |
| `environment`       | String map   | Local process environment variables.                                   |
| `url`               | String       | Required for remote servers. Absolute HTTP or HTTPS URL.               |
| `headers`           | String map   | Headers for a remote server.                                           |
| `enabled`           | Boolean      | Defaults to `true`.                                                    |
| `discovery_timeout` | Integer      | Connection and discovery timeout in milliseconds. Defaults to `30000`. |
| `execution_timeout` | Integer      | Tool-call timeout in milliseconds. Defaults to `30000`.                |

Zero timeouts use the defaults. Negative timeouts are invalid.
Remote connections try streamable HTTP, then SSE. MCP OAuth is not supported.

## `permissions` and `agent_permissions`

Effects are `allow`, `ask`, and `deny`.
`permissions` applies to every run. `agent_permissions` maps agent names or IDs to rule sets.
See [Permissions](/configure/permissions) for syntax, matching, and precedence.

## `provider`

Map each provider ID to a definition. It can change a catalog route or add a provider.
Save credentials through the provider API, not these fields.
See [Providers](/configure/providers) for examples.

| Field        | Type         | Purpose                                                                 |
| ------------ | ------------ | ----------------------------------------------------------------------- |
| `name`       | String       | Display name. Defaults to the ID for a new provider.                    |
| `auth_types` | Object array | Advertised authentication methods. Each has `type` and optional `name`. |
| `options`    | Object       | Route configuration below.                                              |
| `models`     | Object       | Models keyed by ID. Needed to select models from a new provider.        |

Route `options` accept:

| Field        | Type       | Purpose                                                                    |
| ------------ | ---------- | -------------------------------------------------------------------------- |
| `baseURL`    | String     | Provider destination. Defaults to the catalog URL.                         |
| `auth`       | Boolean    | Defaults to `true`. `false` suppresses stored and environment credentials. |
| `authHeader` | String     | Override the credential header.                                            |
| `authScheme` | String     | Prefix for a custom credential header, such as `Bearer`.                   |
| `query`      | String map | Static query parameters.                                                   |

Default credential headers are `x-api-key` for Anthropic, `x-goog-api-key` for Gemini, and `Authorization: Bearer ...` for other supported protocols.
Setting `authHeader` uses the raw key unless `authScheme` supplies a prefix.

Each model in `models` accepts:

| Field                                         | Type         | Purpose                                                                                        |
| --------------------------------------------- | ------------ | ---------------------------------------------------------------------------------------------- |
| `provider`, `id`                              | String       | Defaults to the enclosing keys. Explicit values must match them.                               |
| `api`                                         | String       | Required protocol. See [Supported Protocols](/configure/models#supported-protocols).           |
| `base_url`                                    | String       | Model destination. Otherwise uses the provider destination.                                    |
| `env`                                         | String array | Variables to read for credentials when authentication is enabled.                              |
| `context_window`, `max_output`                | Integer      | Model token limits.                                                                            |
| `capabilities`                                | Object       | Boolean flags: `tools`, `images`, `reasoning`, `structured_output`.                            |
| `input_cost_per_mtok`, `output_cost_per_mtok` | Number       | Cost per million tokens.                                                                       |
| `variants`                                    | Object array | Named request configurations. Each has `id`, optional `provider_options`, and optional `http`. |

A custom model needs a destination from `base_url` or its provider.
Variant `http` accepts `headers`, `query`, and `body`. Use provider-supported values.
