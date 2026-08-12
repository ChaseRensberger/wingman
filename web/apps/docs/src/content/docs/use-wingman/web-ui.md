---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

Wingman includes a local Console on the same HTTP origin as the API. It uses
browser HTTP Basic Auth.

## Local Console

After you start Wingman, open:

```text
http://localhost:2323/console
```

Enter the generated credentials from `~/.config/wingman/service.env` in the
browser prompt. The managed service and foreground server use these credentials
unless the foreground process has explicit environment credentials.

To open the managed daemon from the CLI, run:

```bash
wingman console
```

If the connection drops, the Console retries its readiness check. When the daemon
returns, the Console reloads the active page from the API.

The Console has no password form, session cookie, or `/auth/login` endpoint.
See [Authentication](/concepts/authentication) for the authentication model.
