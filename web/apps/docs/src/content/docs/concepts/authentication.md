---
title: "Authentication"
description: "Authenticate to the Wingman server."
---

# Authentication

The Wingman CLI server requires HTTP Basic Auth for all routes except `/health`.
Basic Auth sends a username and password with each request.

## Managed Service Credentials

The background service saves its URL in `~/.local/state/wingman/registration.json`.
It saves credentials in the private file `~/.config/wingman/service.env`.
`XDG_STATE_HOME` and `XDG_CONFIG_HOME` change these roots. Local clients with service discovery read these files automatically.

Run `wingman pair` to display the managed server URL, username, password, and a
QR code for another client.

`wingman api` connects to the local managed service and adds credentials automatically.

## Direct HTTP Requests

For a shell session that sends requests to the local managed daemon, load the
credentials and registered URL once. These commands require `jq`:

```bash
source "${XDG_CONFIG_HOME:-$HOME/.config}/wingman/service.env"
WINGMAN_URL=$(jq -r .url "${XDG_STATE_HOME:-$HOME/.local/state}/wingman/registration.json")
WINGMAN_AUTH="${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}"
```

Then send requests with `curl`:

```bash
curl -u "$WINGMAN_AUTH" "$WINGMAN_URL/ready"
```

`WINGMAN_AUTH` is a shell variable for `curl`. Do not store it in a file.
For more raw HTTP examples, read [HTTP API Basics](/build-clients/http-api-basics).

## Foreground Server Authentication

If `WINGMAN_PASSWORD` is non-empty, a foreground server uses it. `WINGMAN_USERNAME` then defaults to `wingman`.
Otherwise, the server creates or reuses credentials in `service.env`.

For an explicit server, set `WINGMAN_URL` to its URL. Set `WINGMAN_AUTH` from
that server's credentials.

For remote access, use an encrypted connection such as TLS, Tailscale, or an SSH tunnel.
Basic Auth does not encrypt network traffic. All callers with the server credentials have the same access.

## Console Authentication

Enter the server's credentials in the browser HTTP Basic Auth prompt. The Console has no separate login form.

## Client Identity

Any authenticated caller can register a client or select one with `X-Wingman-Client`.

Client identities group sessions and workspaces. They do not restrict access to providers, tools, logs, plugins, or files.
