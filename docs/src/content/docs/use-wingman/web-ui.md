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

The web UI is a client of the same HTTP API documented in [HTTP API](/reference/referenceapi). It is useful for managing providers, agents, Bases, and sessions without writing `curl` requests by hand.

## Sessions And Bases

The Sessions page starts with Base cards. A Base is a named directory workspace; clicking one opens the sessions created for that Base.

The default `Wingman` Base is created automatically for the built-in client. Create, edit, and delete Bases from the Sessions page. Session detail pages include the Base in the breadcrumb, for example `Home > Sessions > Wingman > New session`.

## Development Proxy

When developing the web UI, run the Vite dev server separately and proxy `/web` through Wingman:

```bash
wingman serve --ui-dev http://localhost:5173
```

Normal users do not need `--ui-dev`.
