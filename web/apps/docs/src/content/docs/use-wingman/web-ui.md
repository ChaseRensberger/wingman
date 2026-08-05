---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

Wingman includes a local Console that uses the same HTTP origin as the API. It
uses an owner browser session and never stores the owner credential.

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

See [Authentication](/concepts/authentication) for the local Console session and
client bearer token model.
