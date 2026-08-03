---
title: "Use the Console"
description: "Open Wingman's bundled local console UI."
---

# Use the Console

Wingman includes a local console UI served by the same HTTP server as the API.
On a loopback listener, opening the Console sets an HttpOnly,
`SameSite=Strict` session cookie. JavaScript cannot read the daemon token.

After starting Wingman, open:

```text
http://localhost:2323/console
```

Wingman does not set this cookie for a non-loopback request host. For remote
access, use a client or reverse proxy that sends the bearer token.

If the connection drops, the Console retries its readiness check. When the
daemon returns, it reloads the active page from the API.
