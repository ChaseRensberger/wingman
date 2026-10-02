---
title: "Clients"
group: "Core"
order: 101
---

# Clients

One Wingman instance supports multiple applications. A client identity groups their sessions.

A session belongs to one client. Any caller with daemon access can select a registered client with `X-Wingman-Client`.

Client identities do not restrict access to providers, tools, logs, plugins, or files.

Without `X-Wingman-Client`, requests use the default client, `WingClient` (`cli_wingclient`).

Client IDs must start with `cli_` and remain stable. Display names are unique without case sensitivity. A new client does not receive credentials.

Select a client with `X-Wingman-Client`. This example connects to the local managed service:

```bash
wingman api createSession \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

`GET /workspaces` lists Workspaces for the selected client. It does not create them automatically.
