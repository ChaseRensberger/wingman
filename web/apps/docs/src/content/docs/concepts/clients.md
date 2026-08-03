---
title: "Clients"
group: "Core"
order: 101
---

# Clients

Wingman is client-agnostic: several applications can use one instance. The
client header identifies the caller so Wingman can attribute and list sessions.
It is organization metadata, not a security boundary. It does not isolate
storage, tools, providers, logs, or filesystem access.

Every persisted session and Workspace belongs to a client. If you omit `X-Wingman-Client`, Wingman uses the built-in default client named `Wingman` with ID `cli_wingman`, so manual `curl` calls and local scripts still work without setup.

Client names are unique case-insensitively. `Wingman` is reserved for the built-in default client.

If you want a request to run in a client context, send the client ID with `X-Wingman-Client`:

This command uses `WINGMAN_TOKEN` from [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

Omitting the header is equivalent to using `X-Wingman-Client: cli_wingman`.

Client identity also scopes Workspaces. `GET /workspaces` returns the Workspaces for the active client; it does not create any Workspaces automatically.
