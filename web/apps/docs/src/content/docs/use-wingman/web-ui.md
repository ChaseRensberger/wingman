---
title: "Use the Console"
description: "Open Wingman's bundled local console UI."
---

# Use the Console

Wingman includes a local console UI served by the same HTTP server as the API.
On a loopback listener, opening the Console sets an HttpOnly,
`SameSite=Strict` session cookie. JavaScript cannot read the daemon token.

Once you start Wingman open:

```text
http://localhost:2323/console
```

Wingman does not set this cookie on a non-loopback listener. Use a native client
or trusted reverse proxy that sends the bearer token for remote access.

The Console displays connection loss and retries readiness checks with bounded
backoff. After the daemon returns, the Console reloads the active page from
durable API state. An active session then rediscovers queued or running work and
resumes its event stream.
