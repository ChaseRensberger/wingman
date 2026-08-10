---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

Wingman includes a local Console that uses the same HTTP origin as the API. It
asks for the daemon password and stores only a signed browser session cookie.

## Local Console

After starting Wingman, open:

```text
http://localhost:2323/console
```

Enter the daemon password in the Console. The session cookie is `HttpOnly` and
`SameSite=Strict`.

You can also open the managed daemon from the CLI:

```bash
wingman console
```

If the connection drops, the Console retries its readiness check. When the
daemon returns, it reloads the active page from the API.

See [Authentication](/concepts/authentication) for the daemon password model.
