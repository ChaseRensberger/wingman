---
title: "Use the Console"
description: "Open the local Console or pair a remote browser."
---

# Use the Console

Wingman includes a Console that uses the same HTTP origin as the API. The
Console uses a revocable browser session. It never stores the owner credential.

## Local Console

After starting Wingman, open:

```text
http://localhost:2323/console
```

On a loopback host, Wingman automatically creates the browser session. The
session cookie is `HttpOnly` and `SameSite=Strict`.

You can also open the managed daemon from the CLI:

```bash
wingman console
```

## Remote Console

Expose the Console and API through one HTTPS origin. Then create a one-use link
on the machine that runs the managed daemon:

```bash
wingman auth pair --url https://wingman.example.com
```

Open the printed link in the remote browser. The link expires after five minutes
and works one time.

The reverse proxy must:

- Preserve the external `Host` header.
- Forward `/auth`, `/console`, and all API routes.
- Pass `Set-Cookie` response headers.
- Disable buffering for server-sent events.
- Keep long-lived `/run` and `/sessions/{id}/events` requests open.

Do not rewrite `Host` to `localhost`. Do not inject the owner credential into
browser requests.

If a link is not available, paste the link or raw pairing credential into the
pairing form in the Console connection banner.

## Revoke Access

List browser and native sessions:

```bash
wingman auth sessions
```

Revoke one session:

```bash
wingman auth revoke ats_...
```

If the connection drops, the Console retries its readiness check. When the
daemon returns, it reloads the active page from the API.

See [Authentication](/concepts/authentication) for the credential model and
current security boundary.
