---
title: "Authentication"
group: "Core"
order: 102
description: "Authenticate to the Wingman server."
---

# Authentication

Use this page to authenticate with the Wingman server.

## Managed Service Credentials

Wingman is a per-user managed service. Its public registration is at
`~/.local/state/wingman/registration.json`. Its generated HTTP credentials are
private and stored at `~/.config/wingman/service.env`.

Managed native clients discover the local registration and use the generated
credentials automatically. Do not copy the credentials into client settings.

## Foreground Server Authentication

An explicit foreground server also requires HTTP Basic authentication. It uses
`WINGMAN_USERNAME` and `WINGMAN_PASSWORD` when both values are configured.
Otherwise, it creates or reuses the credentials in `service.env`.

```bash
curl -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" http://localhost:2323/ready
```

Use TLS or an SSH tunnel before sending Basic Auth credentials to a remote
server. Basic Auth is not a multi-user authorization system.

## Console Authentication

The Console uses the browser's HTTP Basic Auth prompt. It has no password form,
session cookie, or `/auth/login` endpoint. Enter the generated credentials from
`service.env` in that prompt.

## Client Identity

Any caller with daemon access can register a client. Any caller with daemon
access can select a registered client with `X-Wingman-Client`.

Client binding organizes sessions and Workspaces. It does not isolate providers,
tools, logs, plugins, or filesystem access.
