---
title: "Run the Server"
description: "Start Wingman as a foreground process or system service."
---

# Run the Server

Wingman runs as a local HTTP server. By default, it listens on `127.0.0.1:2323`
and stores persistent data in `~/.local/share/wingman/wingman.db`.

## Foreground Server

Run Wingman in the foreground while developing or testing:

```bash
wingman serve
```

Check that it is running:

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

## Managed Service

Install and start Wingman in the background:

```bash
wingman service start
```

On Linux, `wingman service start` prompts for `sudo` before it writes
`/etc/systemd/system/wingman.service`. On macOS, it writes the per-user
LaunchAgent at `~/Library/LaunchAgents/actor.wingman.plist`. It does not need
`sudo`.

`wingman service start` returns after the registered daemon passes its readiness check. The
private state files are in
`${XDG_STATE_HOME:-$HOME/.local/state}/wingman`:

- `registration.json` contains the instance ID, version, URL, PID, and creation time. It has owner-only permissions.
- `password` contains the managed service password and has owner-only permissions.
- `daemon.lock` elects one managed daemon for this state directory.

Inspect the service:

```bash
wingman service status
```

Stop and remove it:

```bash
wingman service stop
```

## Updates

Update a release installation to the latest stable release:

```bash
wingman update
```

Wingman downloads the archive for the current Linux or macOS architecture. It
compares the archive with the release's `checksums.txt`. It then replaces the
resolved executable atomically. Wingman restarts a running systemd service or
LaunchAgent after replacement. The executable directory must be writable. Use
the original installer to update a package-manager-managed or system-wide
installation.

Check for an update without changing anything:

```bash
wingman update --check
```

Install a specific release, including a prerelease:

```bash
wingman update --version 0.1.15
```

## Address and Port

Change the bind address with `--host` and `--port`:

```bash
wingman serve --host 127.0.0.1 --port 2424
```

Wingman does not enable cross-origin browser access by default. The bundled Console is served from `/console` on the same origin as the API.

## Authentication

Wingman always requires a daemon password. `wingman serve` uses
`WINGMAN_PASSWORD` when it is set; otherwise, it creates or reuses the private
state `password` file. The managed service uses only its private password file.
`GET /health` and Console assets remain public.

Use HTTP Basic authentication for protected routes:

```bash
curl -sS http://localhost:2323/ready \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}"
```

The Console asks for the password. It stores a separate signed `HttpOnly`
session cookie. Before you send a password to a remote machine, use TLS or an
SSH tunnel. See
[Authentication](/concepts/authentication) and [HTTP API Basics](/build-clients/http-api-basics#authentication).

## Ephemeral Mode

Run without persistence:

```bash
wingman serve --ephemeral
```

In ephemeral mode, use `POST /run` with an inline agent spec. Persistent
resources, including agents, sessions, clients, and provider auth, are unavailable.
