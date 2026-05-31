---
title: "Run the Server"
description: "Start Wingman as a foreground process or system service."
---

# Run the Server

Wingman runs as a local HTTP server. By default it listens on `127.0.0.1:2323` and stores persistent data in SQLite at `~/.local/share/wingman/wingman.db`.

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

## System Service

On Linux, install and start Wingman as a systemd service:

```bash
wingman up
```

`wingman up` prompts for `sudo` when it needs to write the systemd unit.

Inspect the service:

```bash
wingman status
```

Stop and remove it:

```bash
wingman down
```

## Address and Port

Change the bind address with `--host` and `--port`:

```bash
wingman serve --host 127.0.0.1 --port 2424
```

Use `127.0.0.1` for local-only access. Bind to `0.0.0.0` only on trusted networks; Wingman does not provide inbound auth or multi-tenant isolation.

Wingman does not enable cross-origin browser access by default. The bundled web UI is served from `/web` on the same origin as the API.

## Ephemeral Mode

Run without persistence:

```bash
wingman serve --ephemeral
```

In ephemeral mode, use `POST /run` with inline agent specs. Persistent resources such as agents, sessions, clients, and provider auth are unavailable.
