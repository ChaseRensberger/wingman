---
title: "Authentication"
group: "Core"
order: 102
description: "Authenticate to the Wingman server."
---

# Authentication

Wingman requires HTTP Basic Auth for all routes except `/health`.

## Managed Service Credentials

The managed service saves its URL in `~/.local/state/wingman/registration.json` and credentials in the private file `~/.config/wingman/service.env`.
Local clients that support service discovery load both automatically. Do not copy these credentials into client configuration.

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

Foreground servers use configured `WINGMAN_USERNAME` and `WINGMAN_PASSWORD` values, or create or reuse credentials in `service.env`.

For an explicit server, set `WINGMAN_URL` to its URL. Set `WINGMAN_AUTH` from
that server's credentials.

Basic Auth does not encrypt network traffic. Use a trusted network or a secure
transport, such as Tailscale, TLS, or an SSH tunnel, for remote access. Basic
Auth is not a multi-user authorization system.

## Console Authentication

Enter the server's credentials in the browser HTTP Basic Auth prompt. The Console has no separate login form.

## Client Identity

Any authenticated caller can register a client or select one with `X-Wingman-Client`.

Client identities group sessions and Workspaces. They do not restrict access to providers, tools, logs, plugins, or files.
