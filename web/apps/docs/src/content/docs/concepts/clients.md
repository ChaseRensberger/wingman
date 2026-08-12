---
title: "Clients"
group: "Core"
order: 101
---

# Clients

Wingman supports multiple applications on one instance. A client identity lets
Wingman attribute and list sessions.

A session belongs to one client. Any caller with daemon access can select a
registered client with `X-Wingman-Client`.

Client identity is not tenant isolation. It does not isolate providers, tools,
logs, plugins, or filesystem access.

Every persisted session and Workspace belongs to a client. If you omit
`X-Wingman-Client`, Wingman uses the built-in default client. Its name is
`WingClient`. Its ID is `cli_wingclient`. Manual `curl` calls and local scripts
work without setup.

Client IDs are explicit and stable. They must start with `cli_`. Display names
are unique without case sensitivity. Creating a client registers its attribution
identity. It does not issue credentials.

To make a request in a client context, send the client ID with
`X-Wingman-Client`:

This command uses HTTP Basic authentication. Load managed-service credentials
with `source ~/.config/wingman/service.env`; the username defaults to `wingman`.
See [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

If you omit the header, Wingman uses `X-Wingman-Client: cli_wingclient`.

Client identity also scopes Workspaces. `GET /workspaces` returns the Workspaces
for the active client. It does not create Workspaces automatically.
