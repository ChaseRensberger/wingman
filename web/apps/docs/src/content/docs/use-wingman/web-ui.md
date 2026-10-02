---
title: "Use the Console"
description: "Open the local Wingman Console."
---

# Use the Console

The Console is Wingman's browser interface. It uses the same server address as the API.

## Local Console

After you start Wingman, open this URL:

```text
http://localhost:2424/console
```

Enter the credentials from `~/.config/wingman/service.env` in the browser
prompt. The managed service uses these credentials.
The foreground server uses them unless it has explicit environment credentials.

To open the local managed service's Console:

```bash
wingman console
```

To open a new session in the current directory, run `wingman console .`.
Wingman reuses the workspace for that directory or creates one if needed.
The Console saves the session when you send its first message.

After a disconnect, the Console waits for the server and reloads the current page when it returns.

## Model-call Diagnostics

Open Inspector, then select Copy diagnostics for an attempt under Model calls.
The snapshot includes provider errors, timing, and request IDs.
Read [Observability](/use-wingman/observability) for diagnostic fields, logs, and capture limits.

## Macros

Type `/` in a Session composer to show project macros. Type an argument after
the macro ID, then send the message. See [Macros](/configure/macros) to create
project macros.
