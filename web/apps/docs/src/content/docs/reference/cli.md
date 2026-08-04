---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

The `wingman` binary runs the local Wingman HTTP server, manages its background service, updates release installations, and prints build information.

```bash
wingman <command> [flags]
```

When working from the repository, replace `wingman` with `go run ./cmd/wingman`.

## Commands

| Command | Description |
|---|---|
| `serve` | Start the HTTP server in the foreground. |
| `up` | Install, enable, and start Wingman as a background service. |
| `down` | Stop and remove the Wingman background service. |
| `restart` | Restart the Wingman background service. |
| `status` | Show the Wingman background service status. |
| `console` | Open the managed daemon Console. |
| `auth pair` | Create a one-use Console or client pairing link. |
| `auth sessions` | List browser and native auth sessions. |
| `auth revoke` | Revoke one auth session. |
| `update` | Check for or install a verified release update. |
| `version` | Print version information. |

## Server Commands

`wingman serve` starts the server in the foreground:

```bash
wingman serve
```

`wingman up` installs and starts a background service:

```bash
wingman up
```

On Linux, `wingman up` re-executes itself through `sudo` when required. It
installs `/etc/systemd/system/wingman.service` and runs the service as the
invoking user. On macOS, it installs a per-user LaunchAgent. Both service forms
wait for authenticated readiness before the command returns.

`wingman up` accepts the same runtime flags as `wingman serve`; selected values are written into the generated service definition.

## Runtime Flags

| Flag | Default | Description |
|---|---|---|
| `--host` | `127.0.0.1` | Host to bind to. |
| `--port` | `2323` | Port to listen on. |
| `--db` | `~/.local/share/wingman/wingman.db` | SQLite database path. |
| `--ephemeral` | `false` | Run without persistence. |
| `--log-format` | `json` | Log format: `json` or `text`. |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `--ui-dev` | none | Proxy `/console` to a Vite dev server URL. |
| `--plugin-dir` | none | Additional global plugin directory. Can be repeated. |
| `--no-plugins` | `false` | Disable out-of-process plugin loading. |

Examples:

```bash
wingman serve --host 127.0.0.1 --port 2424
wingman serve --db ./wingman.db
wingman serve --ephemeral
wingman up --port 2424
```

## Console and Auth Commands

Open the Console for the managed daemon:

```bash
wingman console
```

Create a one-use link for a remote HTTPS origin:

```bash
wingman auth pair --url https://wingman.example.com
```

Use `--client cli_...` to bind the new session to a registered client.

List auth sessions, or filter by client:

```bash
wingman auth sessions
wingman auth sessions --client cli_...
```

Revoke one session:

```bash
wingman auth revoke ats_...
```

These commands target the verified daemon that `wingman up` registered. They do
not scan ports or target an unregistered foreground server.

## Service Commands

Check the generated service:

```bash
wingman status
```

The command reports discovery as `ready`, `starting`, `stale`, `incompatible`,
or missing. It then displays the systemd or launchd status.

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

## Update

`wingman update` downloads the latest stable GitHub release for the current Linux or macOS architecture, verifies the downloaded archive against the release's `checksums.txt`, and atomically replaces the resolved executable:

```bash
wingman update
```

The executable directory must be writable. This works with the default installer location (`~/.wingman/bin`) and other writable standalone installs. Installations owned by a package manager or another user should be updated through that original installation method.

The public installer also verifies the downloaded release archive against the
published `checksums.txt` before it installs the binary.

If Wingman is running through systemd or launchd, the command restarts that running managed service after replacement. It does not start an installed but stopped service.

Check availability without writing files:

```bash
wingman update --check
```

Install a particular stable or prerelease version:

```bash
wingman update --version 0.2.0-beta.1
```
