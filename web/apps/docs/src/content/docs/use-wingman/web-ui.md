---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

Wingman includes a local Console on the same HTTP origin as the API. It asks for
the daemon password. It stores only a signed browser session cookie.

## Local Console

After you start Wingman, open:

```text
http://localhost:2323/console
```

Enter the daemon password in the Console. The session cookie uses `HttpOnly` and
`SameSite=Strict`.

To open the managed daemon from the CLI, run:

```bash
wingman console
```

If the connection drops, the Console retries its readiness check. When the daemon
returns, the Console reloads the active page from the API.

See [Authentication](/concepts/authentication) for the daemon password model.
