---
title: "Clients"
group: "Core"
order: 101
---

# Clients

A Wingman client is an application or integration that consumes the Wingman HTTP API. Examples include the built-in web UI, a CLI, an editor plugin, a Formation runner, a third-party app, or a future remote application.

Clients are not agents, users, auth principals, or sandboxes. They identify the caller at the API boundary so sessions can be attributed, listed, and eventually governed per application.

Client registration is optional for local/default use. Requests can create sessions without a client, which is useful for manual `curl` calls, throwaway scripts, and simple local setups.

When a request does run in a client context, it sends the client ID with `X-Wingman-Client`:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```

The session records that `client_id` at creation time. Normal session metadata updates do not move a session between clients.

Today, client identity is attribution and organization, not authorization. Listing sessions by client is a convenience/default scope for applications that want "my sessions" behavior. It is not a security boundary until inbound auth and multi-tenancy exist.
