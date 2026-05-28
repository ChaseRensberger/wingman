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

The web UI is a client of the same HTTP API documented in [HTTP API](/reference/referenceapi). It is useful for managing providers, agents, Workspaces, and sessions without writing `curl` requests by hand.

## Sessions And Workspaces

The Sessions page starts with Workspace cards. A Workspace is a named directory workspace; clicking one opens the sessions created for that Workspace.

The default `Wingman` Workspace is created automatically for the built-in client. Create, edit, and delete Workspaces from the Sessions page.

Workspace filters and session detail are reflected in the URL:

- `/web/sessions` shows recent sessions and Workspace cards.
- `/web/sessions/workspaces/wingman` shows sessions for the default Workspace.
- `/web/sessions/ses_...` opens a session, whether or not it belongs to a Workspace.

Session detail pages include the Workspace in the breadcrumb when present, for example `Home > Sessions > Wingman > New session`. The `Sessions` breadcrumb returns to the sessions hub.

## Development Proxy

When developing the web UI, run the Vite dev server separately and proxy `/web` through Wingman:

```bash
wingman serve --ui-dev http://localhost:5173
```

Normal users do not need `--ui-dev`.
