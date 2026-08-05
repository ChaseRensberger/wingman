---
title: "Authentication"
group: "Core"
order: 102
description: "Understand Wingman credentials, client enrollment, sessions, and current security boundaries."
---

# Authentication

All about authenticating with the Wingman server.

## Credential Types

| Credential | Purpose | Lifetime | Storage |
|---|---|---|---|
| Owner credential | Local administration and recovery | Stable | Private daemon state file |
| Enrollment credential | Creates one client session | Five minutes and one use | Hash in daemon memory |
| Browser session | Authenticates the bundled Console | 30 days | Hash in SQLite and an HttpOnly cookie |
| Bearer session | Authenticates a native client | 30 days | Hash in SQLite and client storage |

The owner credential is located at `${XDG_STATE_HOME:-$HOME/.local/state}/wingman/credential`.

The owner credential has two uses:

- It authenticates trusted local administration and recovery clients.
- It creates enrollment credentials and manages auth sessions.

Do not send the owner credential to a browser or remote client.

## Client Enrollment

An enrollment credential lets a client authenticate for the first time without
receiving the owner credential.

1. An owner registers or selects a Wingman Client.
2. The owner creates an enrollment credential for that client.
3. The owner transfers the credential through a trusted channel.
4. The client redeems the credential before it expires.
5. Wingman consumes the credential atomically and creates a client-bound auth
   session.

Create a credential from the machine that manages the daemon:

```bash
wingman auth enroll --client cli_example
```

The command prints the credential once. Give it to the client through a secret
manager, a paste flow, a QR payload, or another trusted transfer mechanism.
The credential is not a Console URL and is not tied to a particular client UI.

An owner can also create credentials from the local Console in **Settings >
Client authentication**. That view is available only to the loopback Console
bootstrap session; a remotely enrolled Console cannot manage other clients.

Under the hood:

1. Wingman generates a 32-byte random credential.
2. Wingman stores only its SHA-256 hash.
3. The credential expires after five minutes and works once.
4. Redeeming it creates a 30-day opaque auth session.

## Browser Sessions

The Console redeems an enrollment credential in `cookie` mode. Wingman sets a
host-only cookie with these properties:

- `HttpOnly`
- `SameSite=Strict`
- `Path=/`
- `Secure` for a non-loopback host or a TLS connection

The cookie contains a random session token. It does not contain the owner
credential.

## Native Sessions

A native client redeems an enrollment credential in `bearer` mode. The response
returns one bearer session token:

```text
Authorization: Bearer <session-token>
```

The client must store this token as a secret. Wingman stores only its hash.

The session token is opaque. It contains no identity or authorization claims;
Wingman hashes it and looks up the session in SQLite for every request.

The owner can revoke one session without changing other client credentials. Use
`wingman auth sessions` and `wingman auth revoke` from the CLI, or the local
Console authentication management view.

## Client Identity

An auth session belongs to one registered Wingman Client. This binding prevents
an enrolled client from selecting another client with `X-Wingman-Client`.

The owner credential can still select a registered client with that header.
This behavior supports local administration and client provisioning.

Client binding organizes sessions and Workspaces. It does not isolate providers,
tools, logs, plugins, or the filesystem.
