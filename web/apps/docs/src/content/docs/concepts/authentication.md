---
title: "Authentication"
group: "Core"
order: 102
description: "Authenticate to the Wingman daemon."
---

# Authentication

Use this page to authenticate with the Wingman server.

## Daemon Password

Wingman always requires a daemon password. If `WINGMAN_PASSWORD` is set,
`wingman serve` uses it. Otherwise, it creates or reuses the private `password`
file in its state directory. The managed service always uses its own private
`password` file.

Native clients must get the daemon password through a secure channel. They use
HTTP Basic authentication with the username `wingman`. For a local managed
service, set `WINGMAN_DAEMON_PASSWORD` with `export
WINGMAN_DAEMON_PASSWORD="$(wingman service password)"`. If a foreground server
uses `WINGMAN_PASSWORD`, set `WINGMAN_DAEMON_PASSWORD` to the same value:

```bash
curl -u "wingman:${WINGMAN_DAEMON_PASSWORD}" http://localhost:2323/ready
```

If you send the password to a remote daemon, use TLS or an SSH tunnel. The
password is not a multi-user authorization system.

## Console Session

The Console asks for the daemon password. It sets a signed cookie with these
properties:

- `HttpOnly`
- `SameSite=Strict`
- `Path=/`
- `Secure` over TLS

The cookie does not contain the daemon password. If you change the password,
restart the Console session.

## Client Identity

Any caller with daemon access can register a client. Any caller with daemon
access can select a registered client with `X-Wingman-Client`.

Client binding organizes sessions and Workspaces. It does not isolate providers,
tools, logs, plugins, or filesystem access.
