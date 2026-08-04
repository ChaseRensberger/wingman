---
title: "Authentication"
group: "Core"
order: 102
description: "Understand Wingman credentials, pairing, sessions, and current security boundaries."
---

# Authentication

All about authenticating with the Wingman server.

## Credential Types

| Credential | Purpose | Lifetime | Storage |
|---|---|---|---|
| Owner credential | Local administration and recovery | Stable | Private daemon state file |
| Pairing credential | Creates one client session | Five minutes and one use | Hash in daemon memory |
| Browser session | Authenticates the bundled Console | 30 days | Hash in SQLite and an HttpOnly cookie |
| Bearer session | Authenticates a native client | 30 days | Hash in SQLite and client storage |

The owner credential is located at `${XDG_STATE_HOME:-$HOME/.local/state}/wingman/credential`.

The owner credential has two uses:

- It authenticates trusted local administration and recovery clients.
- It creates, lists, and revokes pairings and auth sessions.

Do not send the owner credential to a browser. Do not add it to a pairing URL.

## Pairing Flow

1. An owner creates a pairing credential.
2. Wingman stores only its SHA-256 hash.
3. The client redeems the credential before it expires.
4. Wingman consumes the credential atomically.
5. Wingman creates a client-bound auth session.

The pairing link stores the credential in the URL fragment:

```text
https://wingman.example.com/console#pairing=...
```

Browsers do not send a URL fragment in an HTTP request. The Console removes the
fragment before it sends the credential in a JSON request.

## Browser Sessions

The Console redeems a pairing credential in `cookie` mode. Wingman sets a
host-only cookie with these properties:

- `HttpOnly`
- `SameSite=Strict`
- `Path=/`
- `Secure` for a non-loopback host or a TLS connection

The cookie contains a random session token. It does not contain the owner
credential.

## Native Sessions

A native client redeems a pairing credential in `bearer` mode. The response
returns one bearer session token:

```text
Authorization: Bearer <session-token>
```

The client must store this token as a secret. Wingman stores only its hash.

The owner can revoke one session without changing other client credentials.
You can use `wingman auth sessions` and `wingman auth revoke` to manage sessions.

## Client Identity

An auth session belongs to one registered Wingman Client. This binding prevents
a paired client from selecting another client with `X-Wingman-Client`.

The owner credential can still select a registered client with that header.
This behavior supports local administration and client provisioning.

Client binding organizes sessions and Workspaces. It does not isolate providers,
tools, logs, plugins, or the filesystem.
