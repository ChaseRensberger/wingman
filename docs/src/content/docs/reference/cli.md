---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

The `wingman` binary runs the local Wingman HTTP server, manages the Linux systemd service, and prints build information.

## Usage

```bash
wingman <command> [flags]
```

When working from the repository, replace `wingman` with `go run ./cmd/wingman`:

```bash
go run ./cmd/wingman <command> [flags]
```

## Commands

| Command | Description |
|---|---|
| `serve` | Start the HTTP server. |
| `up` | Install, enable, and start Wingman as a systemd service. |
| `down` | Stop, disable, and remove the Wingman systemd service. |
| `status` | Show the Wingman systemd service status. |
| `version` | Print version information. |

## `wingman up`

Installs `/etc/systemd/system/wingman.service`, reloads systemd, enables the service at boot, and starts it immediately.

```bash
wingman up [flags]
```

The command re-executes itself through `sudo` when needed. The service runs `wingman serve` as the user that invoked it, so the default database stays under that user's home directory. Linux/systemd is the supported service manager.

### Flags

`wingman up` accepts the same runtime flags as `wingman serve`. The selected values are written into the generated systemd unit.

| Flag | Default | Description |
|---|---|---|
| `--host` | `127.0.0.1` | Host to bind to. |
| `--port` | `2323` | Port to listen on. |
| `--db` | `~/.local/share/wingman/wingman.db` | SQLite database path. |
| `--ephemeral` | `false` | Run without persistence. |
| `--log-format` | `json` | Log format: `json` or `text`. |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `--ui-dev` | none | Proxy `/web` to a Vite dev server URL. |
| `--plugin-dir` | none | Additional global plugin directory. Can be repeated. |
| `--no-plugins` | `false` | Disable out-of-process plugin loading. |

### Examples

Start Wingman now and at boot:

```bash
wingman up
```

Start the service on a different port:

```bash
wingman up --port 2424
```

Bind on all interfaces:

```bash
wingman up --host 0.0.0.0
```

## `wingman down`

Stops and disables `wingman.service`, removes `/etc/systemd/system/wingman.service`, and reloads systemd.

```bash
wingman down
```

## `wingman status`

Shows `systemctl status wingman.service`.

```bash
wingman status
```

## `wingman serve`

Starts the Wingman HTTP server in the foreground. By default it binds to `127.0.0.1:2323` and stores data in SQLite at `~/.local/share/wingman/wingman.db`.

```bash
wingman serve [flags]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--host` | `127.0.0.1` | Host to bind to. |
| `--port` | `2323` | Port to listen on. |
| `--db` | `~/.local/share/wingman/wingman.db` | SQLite database path. |
| `--ephemeral` | `false` | Run without persistence. |
| `--log-format` | `json` | Log format: `json` or `text`. |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `--ui-dev` | none | Proxy `/web` to a Vite dev server URL. |
| `--plugin-dir` | none | Additional global plugin directory. Can be repeated. |
| `--no-plugins` | `false` | Disable out-of-process plugin loading. |

### Examples

Start with defaults:

```bash
wingman serve
```

Bind on all interfaces:

```bash
wingman serve --host 0.0.0.0 --port 2323
```

Use a custom database path:

```bash
wingman serve --db ./wingman.db
```

Run without persistence:

```bash
wingman serve --ephemeral
```

Proxy the embedded web route to a local Vite server during UI development:

```bash
wingman serve --ui-dev http://localhost:5173
```

## `wingman version`

Prints the binary version, commit, and build date.

```bash
wingman version
```

Example output:

```text
wingman dev (commit: none, built: unknown)
```
