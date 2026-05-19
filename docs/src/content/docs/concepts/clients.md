---
title: "Clients"
group: "Core"
order: 101
---

# Clients

Since Wingman is client agnostic, I wanted different clients on the same machine to be able to use a single Wingman instance as a dependency without interfering with eachother. They identify the caller at the API boundary so sessions can be attributed, listed, and governed per client/application.

Client registration is optional and all wingman primtives can exist in a default client (no client) state so if you don't want to worry about it, you don't have to.

If you want a request to run in a client context, send the client ID with `X-Wingman-Client`:

```bash
curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -H "X-Wingman-Client: cli_..." \
  -d '{"title":"From my app"}'
```
