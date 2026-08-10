---
title: "Authentication"
group: "Core"
order: 102
description: "Authenticate to the Wingman daemon."
---

# Authentication

All about authenticating with the Wingman server.

## Daemon Password

Wingman always requires a daemon password. `wingman serve` uses
`WINGMAN_PASSWORD` when it is set; otherwise, it creates or reuses the private
`password` file in its state directory. The managed service always uses its own
private `password` file.

Native clients must obtain the daemon password through a secure channel and use
HTTP Basic authentication with the username `wingman`. For a local managed
service, set `WINGMAN_DAEMON_PASSWORD` with `export
WINGMAN_DAEMON_PASSWORD="$(wingman service password)"`. For a foreground server
started with `WINGMAN_PASSWORD`, the client uses the same value but must set
`WINGMAN_DAEMON_PASSWORD` itself:

```bash
curl -u "wingman:${WINGMAN_DAEMON_PASSWORD}" http://localhost:2323/ready
```

Use TLS or an SSH tunnel before sending the password to a remote daemon. The
password is not a multi-user authorization system.

## Console Session

The Console asks for the daemon password and sets a signed cookie with these
properties:

- `HttpOnly`
- `SameSite=Strict`
- `Path=/`
- `Secure` over TLS

The cookie does not contain the daemon password. Restart the Console session
after changing the password.

## Client Identity

Any caller with daemon access can register a client and select a registered client
with `X-Wingman-Client`.

Client binding organizes sessions and Workspaces. It does not isolate providers,
tools, logs, plugins, or the filesystem.
