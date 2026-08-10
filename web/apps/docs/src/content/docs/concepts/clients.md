---
title: "Clients"
group: "Core"
order: 101
---

# Clients

Wingman is client-agnostic: several applications can use one instance. A client
identity lets Wingman attribute and list sessions.

A session is bound to a single client. Any caller with daemon access can select a
registered client with `X-Wingman-Client`.

Client identity is not tenant isolation. It does not isolate providers, tools,
logs, plugins, or filesystem access.

Every persisted session and Workspace belongs to a client. If you omit `X-Wingman-Client`, Wingman uses the built-in default client named `WingClient` with ID `cli_wingclient`, so manual `curl` calls and local scripts still work without setup.

Client IDs are explicit and stable. They must start with `cli_`; display names are unique case-insensitively. Creating a client registers its attribution identity; it does not issue credentials.

To make a request in a client context, send the client ID with
`X-Wingman-Client`:

This command uses `WINGMAN_DAEMON_PASSWORD` with HTTP Basic authentication. See [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

Omitting the header is equivalent to using `X-Wingman-Client: cli_wingclient`.

Client identity also scopes Workspaces. `GET /workspaces` returns the Workspaces for the active client; it does not create any Workspaces automatically.
