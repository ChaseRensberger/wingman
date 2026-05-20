---
title: "Use the Web UI"
description: "Open Wingman's bundled local web UI."
---

# Use the Web UI

Wingman includes a local web UI served by the same HTTP server as the API.

Start Wingman:

```bash
wingman serve
```

Open:

```text
http://localhost:2323/web
```

The web UI is a client of the same HTTP API documented in [HTTP API](/reference/referenceapi). It is useful for managing providers, agents, and sessions without writing `curl` requests by hand.

## Development Proxy

When developing the web UI, run the Vite dev server separately and proxy `/web` through Wingman:

```bash
wingman serve --ui-dev http://localhost:5173
```

Normal users do not need `--ui-dev`.
