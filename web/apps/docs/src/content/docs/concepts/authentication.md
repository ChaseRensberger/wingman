---
title: "Authentication"
group: "Core"
order: 102
description: "Understand owner and client bearer tokens."
---

# Authentication

All about authenticating with the Wingman server.

## Credential Types

| Credential | Purpose | Lifetime | Storage |
|---|---|---|---|
| Owner credential | Local administration and recovery | Stable | Private daemon state file |
| Local Console session | Authenticates the bundled Console | Local browser session | HttpOnly cookie |
| Client bearer token | Authenticates one client | Until rotated or revoked | Client secret storage |

The owner credential is located at `${XDG_STATE_HOME:-$HOME/.local/state}/wingman/credential`.

The owner credential has two uses:

- It authenticates trusted local administration and recovery clients.
- It manages clients and auth sessions.

Do not send the owner credential to a browser or remote client.

## Client Bearer Tokens

Create a client from the local owner Console or with `POST /clients`. The response
returns one opaque bearer token. Transfer it through a trusted channel and store it
as a client secret:

```text
Authorization: Bearer <client-token>
```

Wingman stores only the token hash. Rotate a client's token from the local
Console **Clients** page or with `POST /clients/{id}/token`. Rotation invalidates
the previous token.

## Local Console Session

On a loopback host, Wingman creates an owner Console session and sets a host-only
cookie with these properties:

- `HttpOnly`
- `SameSite=Strict`
- `Path=/`
- `Secure` for a non-loopback host or a TLS connection

The cookie does not contain the owner credential. Do not expose the owner Console
session to remote browsers.

Use `wingman auth sessions` and `wingman auth revoke` to inspect or revoke
sessions when needed.

## Client Identity

A client bearer token belongs to one registered Wingman Client. This binding
prevents a client from selecting another client with `X-Wingman-Client`.

The owner credential can still select a registered client with that header.
This behavior supports local administration and client provisioning.

Client binding organizes sessions and Workspaces. It does not isolate providers,
tools, logs, plugins, or the filesystem.
