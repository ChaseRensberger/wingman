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

The web UI is a same-origin client of the HTTP API documented in [HTTP API](/reference/referenceapi). It is useful for managing providers, agents, Workspaces, and sessions without writing `curl` requests by hand.

## Sessions and Workspaces

The Sessions page shows all sessions by default. Use the Workspace dropdown to filter to a saved Workspace or to sessions with no Workspace.

Create, edit, and delete Workspaces from the Sessions page. Wingman does not create a default Workspace automatically. When setting a Workspace or session working directory, enter an absolute path on the machine running Wingman.

Workspace filters and session detail are reflected in the URL:

- `/web/sessions` shows all sessions.
- `/web/sessions?workspace=wsp_...` shows sessions for one Workspace.
- `/web/sessions?workspace=none` shows sessions without a Workspace.
- `/web/sessions/ses_...` opens a session, whether or not it belongs to a Workspace.

Session detail pages include the Workspace in the breadcrumb when present, for example `Home > Sessions > Wingman > New session`. The Workspace breadcrumb returns to the filtered sessions hub.

## Development Proxy

When developing the web UI, run the Vite dev server separately and proxy `/web` through Wingman:

```bash
wingman serve --ui-dev http://localhost:5173
```

Normal users do not need `--ui-dev`.
