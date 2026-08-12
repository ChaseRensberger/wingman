---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

Wingman includes a local Console on the API HTTP origin. It uses browser HTTP
Basic Auth.

## Local Console

After you start Wingman, open this URL:

```text
http://localhost:2323/console
```

Enter the credentials from `~/.config/wingman/service.env` in the browser
prompt. The managed service uses these credentials.
The foreground server uses them unless it has explicit environment credentials.

To open the managed daemon from the CLI, run this command:

```bash
wingman console
```

If the connection drops, the Console repeats its readiness check.
When the daemon returns, the Console reloads the active API page.

The Console has no password form, session cookie, or `/auth/login` endpoint.
Read [Authentication](/concepts/authentication) for the authentication model.
