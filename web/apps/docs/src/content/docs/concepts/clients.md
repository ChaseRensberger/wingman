---
title: "Clients"
group: "Core"
order: 101
---

# Clients

Wingman is client-agnostic: several applications can use one instance. A client
identity lets Wingman attribute and list sessions.

A session is bound to a single client. The client cannot select another client
with `X-Wingman-Client`. The owner credential can select a registered client for
local administration.

Client identity is not tenant isolation. It does not isolate providers, tools,
logs, plugins, or filesystem access.

Every persisted session and Workspace belongs to a client. If you omit `X-Wingman-Client`, Wingman uses the built-in default client named `WingClient` with ID `cli_wingclient`, so manual `curl` calls and local scripts still work without setup.

Client IDs are explicit and stable. They must start with `cli_`; display names are unique case-insensitively. Creating a client returns one opaque access token, which Wingman stores only as a hash. Rotate the token from the local Console **Settings** page or with `POST /clients/{id}/token` when it must be replaced.

If the owner makes a request in a client context, send the client ID with
`X-Wingman-Client`:

This command uses `WINGMAN_TOKEN` from [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

Omitting the header is equivalent to using `X-Wingman-Client: cli_wingclient`.

Client identity also scopes Workspaces. `GET /workspaces` returns the Workspaces for the active client; it does not create any Workspaces automatically.
