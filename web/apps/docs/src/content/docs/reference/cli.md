---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

The `wingman` binary runs the local Wingman HTTP server. It manages the
background service, updates release installations, and prints build information.

```bash
wingman <command> [flags]
```

## Commands

| Command | Description |
|---|---|
| `serve` | Start the HTTP server in the foreground. |
| `service start` | Install, enable, and start Wingman as a background service. |
| `service stop` | Stop and remove the Wingman background service. |
| `service restart` | Restart the Wingman background service. |
| `service status` | Show the Wingman background service status. |
| `service password [password]` | Show or set the managed service password. |
| `console` | Open the managed daemon Console. |
| `clients create` | Register an API client identity. |
| `update` | Check for or install a verified release update. |
| `version` | Print version information. |

## Server Commands

`wingman serve` starts the server in the foreground:

```bash
wingman serve
```

`wingman service start` installs and starts a background service:

```bash
wingman service start
```

If required on Linux, `wingman service start` re-executes through `sudo`. It
installs `/etc/systemd/system/wingman.service`. It runs the service as the
invoking user. Systemd manages its private daemon state at `/var/lib/wingman`.
The default SQLite database remains at `~/.local/share/wingman/wingman.db`.
On macOS, it installs a per-user LaunchAgent. Both service forms wait for
readiness before the command returns.

`wingman service start` accepts the same runtime flags as `wingman serve`.
Selected values are written into the generated service definition.

## Runtime Flags

| Flag | Default | Description |
|---|---|---|
| `--host` | `127.0.0.1` | Host to bind to. |
| `--port` | `2323` | Port to listen on. |
| `--db` | `~/.local/share/wingman/wingman.db` | SQLite database path. |
| `--ephemeral` | `false` | Run without persistence. |
| `--log-format` | `json` | Log format: `json` or `text`. |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `--plugin-dir` | none | Additional global plugin directory. Can be repeated. |
| `--no-plugins` | `false` | Disable out-of-process plugin loading. |

Examples:

```bash
wingman serve --host 127.0.0.1 --port 2424
wingman serve --db ./wingman.db
wingman serve --ephemeral
wingman service start --port 2424
```

## Console Command

Open the Console for the managed daemon:

```bash
wingman console
```

The Console prompts for the daemon password and creates a signed `HttpOnly`
session cookie.

## Service Commands

Check the generated service:

```bash
wingman service status
```

The command reports discovery as `ready`, `starting`, `stale`, `incompatible`,
or missing. It then displays the systemd or launchd status.

Restart the service after editing `~/.config/wingman/wingman.json`:

```bash
wingman service restart
```

To change service flags such as `--host`, `--port`, `--db`, or `--plugin-dir`,
run `wingman service start` again with the new flags.

Show the managed service password, or replace it by providing a new value:

```bash
wingman service password
wingman service password 'new-password'
```

When you set a new password, Wingman stops the service. Run `wingman service start`
to start it again.

Stop and remove the service:

```bash
wingman service stop
```

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

`wingman update` downloads the latest stable GitHub release for the current Linux
or macOS architecture. It verifies the downloaded archive against the release's
`checksums.txt`. It atomically replaces the resolved executable:

```bash
wingman update
```

The executable directory must be writable. This works with the default installer
location (`~/.wingman/bin`) and other writable standalone installs. Update an
installation owned by a package manager or another user through its original
installation method.

The public installer also verifies the downloaded release archive against the
published `checksums.txt` before it installs the binary.

If Wingman runs through systemd or launchd, the command restarts that managed
service after replacement. It does not start an installed but stopped service.

Check availability without writing files:

```bash
wingman update --check
```

Install a particular stable or prerelease version:

```bash
wingman update --version 0.2.0-beta.1
```
