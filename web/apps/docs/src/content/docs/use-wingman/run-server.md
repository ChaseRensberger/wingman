---
title: "Run the Server"
description: "Start Wingman as a foreground process or system service."
---

# Run the Server

Wingman runs as a local HTTP server. By default it listens on `127.0.0.1:2323` and stores persistent data in SQLite at `~/.local/share/wingman/wingman.db`. Currently it has no inbound authentication, and a reachable caller can use configured providers, inspect directories, manage extensions, and start agents with enabled tools. This will likely change in the near future.

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

## Background Service

Install and start Wingman in the background:

```bash
wingman up
```

On Linux, `wingman up` prompts for `sudo` when it needs to write `/etc/systemd/system/wingman.service`. On macOS, it writes the per-user LaunchAgent at `~/Library/LaunchAgents/actor.wingman.plist` and does not need `sudo`.

Inspect the service:

```bash
wingman status
```

Stop and remove it:

```bash
wingman down
```

## Updates

Update a release installation to the latest stable release:

```bash
wingman update
```

Wingman downloads the archive for the current Linux or macOS architecture, verifies it against the release's `checksums.txt`, and atomically replaces the resolved executable. A running systemd service or LaunchAgent is restarted after the replacement. The executable's directory must be writable; package-manager-managed or system-wide installations may need to be updated through their original installer instead.

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

Wingman does not enable cross-origin browser access by default. The bundled console UI is served from `/console` on the same origin as the API.

## Ephemeral Mode

Run without persistence:

```bash
wingman serve --ephemeral
```

In ephemeral mode, use `POST /run` with inline agent specs. Persistent resources such as agents, sessions, clients, and provider auth are unavailable.
