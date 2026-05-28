---
title: "Clients"
group: "Core"
order: 101
---

# Clients

Since Wingman is client agnostic, different clients on the same machine can use a single Wingman instance as a dependency without interfering with each other. They identify the caller at the API boundary so sessions can be attributed, listed, and governed per client/application.

Every persisted session and Workspace belongs to a client. If you omit `X-Wingman-Client`, Wingman uses the built-in default client named `Wingman` with ID `cli_wingman`, so manual `curl` calls and local scripts still work without setup.

Client names are unique case-insensitively. `Wingman` is reserved for the built-in default client.

If you want a request to run in a client context, send the client ID with `X-Wingman-Client`:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

Omitting the header is equivalent to using `X-Wingman-Client: cli_wingman`.

Client identity also scopes Workspaces. `GET /workspaces` returns Workspaces for the active client and ensures that client's default `Wingman` Workspace exists.
