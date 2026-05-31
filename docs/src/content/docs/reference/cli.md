---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

The `wingman` binary runs the local Wingman HTTP server, manages the Linux systemd service, and prints build information.

```bash
wingman <command> [flags]
```

When working from the repository, replace `wingman` with `go run ./cmd/wingman`.

## Commands

| Command | Description |
|---|---|
| `serve` | Start the HTTP server in the foreground. |
| `up` | Install, enable, and start Wingman as a systemd service. |
| `down` | Stop, disable, and remove the Wingman systemd service. |
| `restart` | Restart the Wingman systemd service. |
| `status` | Show the Wingman systemd service status. |
| `version` | Print version information. |

## Server Commands

`wingman serve` starts the server in the foreground:

```bash
wingman serve
```

`wingman up` installs `/etc/systemd/system/wingman.service`, enables it at boot, and starts it immediately:

```bash
wingman up
```

`wingman up` re-executes itself through `sudo` when needed. The service runs `wingman serve` as the user that invoked it, so the default database stays under that user's home directory. Linux/systemd is the supported service manager.

`wingman up` accepts the same runtime flags as `wingman serve`; selected values are written into the generated systemd unit.

## Runtime Flags

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

Examples:

```bash
wingman serve --host 127.0.0.1 --port 2424
wingman serve --db ./wingman.db
wingman serve --ephemeral
wingman up --port 2424
```

Bind to `0.0.0.0` only on trusted networks. Wingman does not provide inbound auth or multi-tenant isolation.

## Service Commands

Check the generated systemd service:

```bash
wingman status
```

Restart the service after editing `~/.config/wingman/wingman.json`:

```bash
wingman restart
```

To change service flags such as `--host`, `--port`, `--db`, or `--plugin-dir`, run `wingman up` again with the new flags.

Stop and remove the service:

```bash
wingman down
```

## Development Proxy

Proxy the embedded web route to a local Vite server while developing the web UI:

```bash
wingman serve --ui-dev http://localhost:5173
```

Normal users do not need `--ui-dev`.

## Version

Print the binary version, commit, and build date:

```bash
wingman version
```

Example output:

```text
wingman dev (commit: none, built: unknown)
```
