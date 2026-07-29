---
title: "Use the Console"
description: "Open Wingman's bundled local console UI."
---

# Use the Console

Wingman includes a local console UI served by the same HTTP server as the API.

Start Wingman:

```bash
wingman serve
```

Open:

```text
http://localhost:2323/console
```

The console UI is a same-origin client of the HTTP API documented in [HTTP API](/reference/referenceapi). It is useful for managing providers, agents, Workspaces, and sessions without writing `curl` requests by hand.

## Sessions and Workspaces

The Sessions page shows all sessions by default. Use the Workspace dropdown to filter to a saved Workspace or to sessions with no Workspace.

Create, edit, and delete Workspaces from the Sessions page. Wingman does not create a default Workspace automatically. Working-directory paths name locations on the machine running Wingman, not the browser's machine. Absolute paths are clearest; relative paths are resolved from the Wingman process's current working directory.

Workspace filters and session detail are reflected in the URL:

- `/console/sessions` shows all sessions.
- `/console/sessions?workspace=wsp_...` shows sessions for one Workspace.
- `/console/sessions?workspace=none` shows sessions without a Workspace.
- `/console/sessions/ses_...` opens a session, whether or not it belongs to a Workspace.

Session detail pages include the Workspace in the breadcrumb when present, for example `Home > Sessions > Wingman > New session`. The Workspace breadcrumb returns to the filtered sessions hub.

For a new session still titled `New session`, the console asks the selected model to generate a title from the first prompt. It sends that prompt in a separate, ephemeral request while sending the normal session message, so the provider receives the first prompt twice. That extra request can incur provider usage charges and is subject to the provider's data-handling policy; its title-generation conversation is not saved as session history. Edit the title before sending the first message to skip automatic generation.

## Tools, MCP, and Logs

The **Tools** page lists the tools currently available to agents, grouped by built-in tools, plugins, and connected MCP servers. For MCP servers configured in `wingman.json`, it shows connection status, discovered tool count, and the most recent connection error, and can connect or disconnect a server. It does not edit MCP configuration, plugin directories, server configuration, or permission policy, and MCP OAuth is not available in this beta.

The **Logs** page refreshes every two seconds and shows recent logs from the running Wingman process. Use its text and level filters to narrow the displayed entries. It is a process-local, recent-log view rather than durable log storage or a cross-process log aggregator.

## Settings

Console Settings are browser-local presentation preferences: display name, color mode, and theme. They are stored in the browser and do not configure the Wingman server, providers, agents, API keys, MCP servers, or other clients/browsers.

## Development Proxy

When developing the console UI, run the Vite dev server separately and proxy `/console` through Wingman:

```bash
wingman serve --ui-dev http://localhost:5173
```

Normal users do not need `--ui-dev`.
