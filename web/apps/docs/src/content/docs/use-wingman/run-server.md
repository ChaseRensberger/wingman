---
title: "Run the Server"
description: "Start Wingman as a foreground process or per-user managed service."
---

# Run the Server

Wingman runs as a local HTTP server. By default, it listens on `127.0.0.1:2323`.
It stores persistent data in `~/.local/share/wingman/wingman.db`.

## Foreground Server

If you develop or test Wingman, run it in the foreground:

```bash
wingman serve
```

To view the server status, run this command:

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

## Managed Service

To install and start Wingman in the background, run this command:

```bash
wingman service start
```

The managed service runs for the current user. It does not require `sudo`.

`wingman service start` returns after the daemon passes its readiness check.
The state files are in
`${XDG_STATE_HOME:-$HOME/.local/state}/wingman`:

- `registration.json` contains the instance ID, version, URL, PID, and creation time. It is public service-discovery data.
- `daemon.lock` selects one managed daemon for this state directory.

The managed-service credentials are in `~/.config/wingman/service.env`.
Only the file owner can read this private file.

To view the service status, run this command:

```bash
wingman service status
```

To stop and remove the service, run this command:

```bash
wingman service stop
```

## Updates

To update a release installation to the latest stable release, run this command:

```bash
wingman update
```

Wingman downloads the archive for the current Linux or macOS architecture.
It compares the archive with the release `checksums.txt` file.
It replaces the executable atomically and restarts a running managed service.
The executable directory must be writable.
If a package manager manages the executable, use that package manager to update it.

To view updates without changes, run this command:

```bash
wingman update --check
```

To install a specific release, including a prerelease, run this command:

```bash
wingman update --version 0.1.15
```

## Address and Port

To change the bind address, use `--host` and `--port`:

```bash
wingman serve --host 127.0.0.1 --port 2424
```

Wingman disables cross-origin browser access by default. The Console is served
from `/console` on the API origin.

## Authentication

The managed service uses credentials from `~/.config/wingman/service.env`.
Managed native clients find and use these credentials automatically.

An explicit foreground server also requires HTTP Basic authentication.
It uses configured `WINGMAN_USERNAME` and `WINGMAN_PASSWORD` values.
Otherwise, it creates or reuses the credentials in `service.env`.

For protected routes, use HTTP Basic authentication:

```bash
curl -sS http://localhost:2323/ready \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}"
```

The Console uses browser HTTP Basic Auth. It has no password form or session
cookie. Before you send credentials to a remote machine, use TLS or an SSH
tunnel. Read
[Authentication](/concepts/authentication) and [HTTP API Basics](/build-clients/http-api-basics#authentication).

## Ephemeral Mode

To run without persistence, use this command:

```bash
wingman serve --ephemeral
```

In ephemeral mode, use `POST /run` with an inline agent specification.
Persistent resources are unavailable, including agents, sessions, clients, and provider authentication.
