---
title: "Authentication"
group: "Core"
order: 102
description: "Authenticate to the Wingman server."
---

# Authentication

This page explains Wingman server authentication.

## Managed Service Credentials

Wingman is a managed service for one user. Its public registration is in
`~/.local/state/wingman/registration.json`. Its generated HTTP credentials are private. Wingman stores them in `~/.config/wingman/service.env`.

Managed native clients find the local registration and use the generated credentials automatically. Do not copy these credentials into client configuration.

## Foreground Server Authentication

An explicit foreground server also requires HTTP Basic authentication. It uses `WINGMAN_USERNAME` and `WINGMAN_PASSWORD` when both values are configured. Otherwise, it creates or reuses credentials in `service.env`.

```bash
curl -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" http://localhost:2323/ready
```

Before you send Basic Auth credentials to a remote server, use TLS or an SSH tunnel. Basic Auth is not a multi-user authorization system.

## Console Authentication

The Console uses the browser HTTP Basic Auth prompt. It has no password form, session cookie, or `/auth/login` endpoint. Enter the generated credentials from `service.env` in this prompt.

## Client Identity

Any caller with daemon access can register a client. Any caller with daemon access can select a registered client with `X-Wingman-Client`.

Client binding organizes sessions and Workspaces. It does not isolate providers, tools, logs, plugins, or filesystem access.
