---
title: "Clients"
---

# Clients

One Wingman server supports multiple applications. A client identity is a saved ID and display name for an application.
Each identity groups its sessions and workspaces.

A session belongs to one client. Any caller with daemon access can select a registered client with `X-Wingman-Client`.

Client identities do not restrict access to providers, tools, logs, plugins, or files.

Without `X-Wingman-Client`, requests use the default client, `WingClient` (`cli_wingclient`).

Client IDs must start with `cli_`. Keep the ID stable when the application restarts.
Display names must be unique, ignoring case. Registering a client does not create credentials.

Register a client before you select it. These commands use the local background service:

```bash
wingman api createClient -d '{"id":"cli_example","name":"Example app"}'
```

```bash
wingman api createSession \
  -H "X-Wingman-Client: cli_example" \
  -d '{"title":"From my app"}'
```

`GET /workspaces` lists workspaces for the selected client. It does not create them automatically.
