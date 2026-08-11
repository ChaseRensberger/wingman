---
title: "Run the Server"
description: "Start Wingman as a foreground process or system service."
---

# Run the Server

Wingman runs as a local HTTP server. By default, it listens on `127.0.0.1:2323`.
It stores persistent data in `~/.local/share/wingman/wingman.db`.

## Foreground Server

If you develop or test, run Wingman in the foreground:

```bash
wingman serve
```

To view the server status, run:

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

## Managed Service

To install and start Wingman in the background, run:

```bash
wingman service start
```

On Linux, `wingman service start` prompts for `sudo` before it writes
`/etc/systemd/system/wingman.service`. On macOS, it writes the per-user
LaunchAgent at `~/Library/LaunchAgents/actor.wingman.plist`. It does not require
`sudo`.

`wingman service start` returns after the registered daemon passes its readiness check.
The private state files are in
`${XDG_STATE_HOME:-$HOME/.local/state}/wingman`:

- `registration.json` contains the instance ID, version, URL, PID, and creation time. It has owner-only permissions.
- `password` contains the managed service password. It has owner-only permissions.
- `daemon.lock` selects one managed daemon for this state directory.

To view the service status, run:

```bash
wingman service status
```

To stop and remove the service, run:

```bash
wingman service stop
```

## Updates

To update a release installation to the latest stable release, run:

```bash
wingman update
```

Wingman downloads the archive for the current Linux or macOS architecture. It
compares the archive with the release's `checksums.txt`. It replaces the resolved
executable atomically. Wingman restarts a running systemd service or LaunchAgent
after replacement. The executable directory must be writable. If a package manager
or system installation manages the executable, use the original installer to update it.

To view update availability without making changes, run:

```bash
wingman update --check
```

To install a specific release, including a prerelease, run:

```bash
wingman update --version 0.1.15
```

## Address and Port

To change the bind address, use `--host` and `--port`:

```bash
wingman serve --host 127.0.0.1 --port 2424
```

Wingman does not enable cross-origin browser access by default. The bundled Console
is served from `/console` on the same origin as the API.

## Authentication

Wingman always requires a daemon password. If `WINGMAN_PASSWORD` is set,
`wingman serve` uses it. Otherwise, `wingman serve` creates or reuses the private
state `password` file. The managed service uses only its private password file.
`GET /health` and Console assets remain public.

For protected routes, use HTTP Basic authentication:

```bash
curl -sS http://localhost:2323/ready \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}"
```

The Console asks for the password. It stores a separate signed `HttpOnly`
session cookie. If you send a password to a remote machine, use TLS or an SSH
tunnel. See
[Authentication](/concepts/authentication) and [HTTP API Basics](/build-clients/http-api-basics#authentication).

## Ephemeral Mode

To run without persistence, use:

```bash
wingman serve --ephemeral
```

In ephemeral mode, use `POST /run` with an inline agent spec. Persistent resources,
including agents, sessions, clients, and provider auth, are unavailable.
