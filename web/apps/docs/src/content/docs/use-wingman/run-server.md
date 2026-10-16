---
title: "Run the Server"
description: "Start Wingman as a foreground process or per-user managed service."
---

# Run the Server

Wingman runs as an HTTP server. By default, it listens on `127.0.0.1:2424`, which accepts connections from your computer only.
It stores persistent data in `~/.local/share/wingman/wingman.db`.

## Foreground Server

To run Wingman in your terminal:

```bash
wingman serve
```

Read the health endpoint:

```bash
curl -sS http://localhost:2424/health
```

Expected response:

```json
{ "status": "ok" }
```

## Managed Service

To start Wingman in the background:

```bash
wingman service start
```

The managed service runs in the background for the current user without `sudo`.
It requires systemd on Linux or launchd on macOS.

The command returns when Wingman is ready.
Credentials are in the private file `~/.config/wingman/service.env`.
`XDG_CONFIG_HOME` changes the configuration root.

Read the service status:

```bash
wingman service status
```

To stop and remove the service:

```bash
wingman service stop
```

## Update Wingman

To install the latest stable release:

```bash
wingman update
```

Wingman compares the download with the release checksum, replaces the executable, and restarts a running managed service.
The executable directory must be writable.
If a package manager manages the executable, use that package manager to update it.

To look for updates without installing them:

```bash
wingman update --check
```

To install a specific release, replace `VERSION` with its version number:

```bash
wingman update --version VERSION
```

## Address and Port

To change the address and port, use `--host` and `--port`:

```bash
wingman serve --host 127.0.0.1 --port 2424
```

Wingman does not enable cross-origin browser access. The Console and API use the same server address.

## Authentication

The managed service uses credentials from `~/.config/wingman/service.env`.
Managed native clients find and use these credentials automatically.

If `WINGMAN_PASSWORD` is set, a foreground server uses it with `WINGMAN_USERNAME`, which defaults to `wingman`.
Otherwise, it creates or reuses credentials in `service.env`.

Before using direct HTTP, load the URL and credentials from the [authentication setup](/concepts/authentication#direct-http-requests).
For remote access, use TLS or an SSH tunnel before sending credentials.

```bash
curl -sS "$WINGMAN_URL/ready" -u "$WINGMAN_AUTH"
```

The Console asks for these credentials through the browser's HTTP Basic Auth prompt.

## Ephemeral Mode

To run without saving data:

```bash
wingman serve --ephemeral
```

Ephemeral means that Wingman does not save runs or conversations.
In this mode, use `POST /run` with an agent definition in the request.
Saved agents, sessions, clients, workspaces, and provider credentials are unavailable.
Supply provider API keys through the server environment or use a provider route that does not require credentials.
