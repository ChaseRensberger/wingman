---
title: "Clients"
group: "Core"
order: 101
---

# Clients

One Wingman instance supports multiple applications. A client identity groups their sessions.

A session belongs to one client. Any caller with daemon access can select a registered client with `X-Wingman-Client`.

Client identity is not tenant isolation. It does not isolate providers, tools,
logs, plugins, or filesystem access.

Every persisted session and Workspace belongs to a client. If you omit `X-Wingman-Client`, Wingman uses the built-in default client. Its name is `WingClient`. Its ID is `cli_wingclient`. Manual API calls and local scripts work without configuration.

Client IDs must start with `cli_` and remain stable. Display names are unique without case sensitivity. A new client does not receive credentials.

To make a request in a client context, send the client ID with `X-Wingman-Client`:

This command finds and authenticates with the managed daemon.

```bash
wingman api createSession \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

If you omit the header, Wingman uses `X-Wingman-Client: cli_wingclient`.

Client identity also scopes Workspaces. `GET /workspaces` returns the Workspaces for the active client. It does not create Workspaces automatically.
