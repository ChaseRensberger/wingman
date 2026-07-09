---
title: "MCP Servers"
description: "Connect Model Context Protocol servers and use their tools in Wingman agents."
---

# Configure MCP Servers

Add Model Context Protocol (MCP) servers to `~/.config/wingman/wingman.json` to make their tools available to Wingman agents. If `XDG_CONFIG_HOME` is set, use `$XDG_CONFIG_HOME/wingman/wingman.json` instead.

Wingman supports local stdio servers and remote servers. Enabled servers connect when Wingman starts. The Console can show their status and connect or disconnect configured servers, but server configuration stays in `wingman.json`.

## Add A Local Server

Use `type: "local"` to start an MCP server as a subprocess over stdio:

```json
{
  "mcp": {
    "project-tools": {
      "type": "local",
      "command": ["/absolute/path/to/mcp-server", "--project", "/absolute/path/to/project"],
      "cwd": "/absolute/path/to/project",
      "environment": {
        "EXAMPLE_TOKEN": "..."
      }
    }
  }
}
```

`command` runs directly, without shell expansion. Put the executable and every argument in separate array items.

## Chrome DevTools

To use Chrome DevTools MCP, configure it as a local server:

```json
{
  "mcp": {
    "chrome-devtools": {
      "type": "local",
      "command": ["npx", "-y", "chrome-devtools-mcp@latest"]
    }
  }
}
```

This requires `npx` and a local Google Chrome installation. To connect to an existing Chrome instance started with remote debugging, add `"--browser-url=http://127.0.0.1:9222"` to `command`.

## Add A Remote Server

Use `type: "remote"` for a remote MCP endpoint:

```json
{
  "mcp": {
    "company-tools": {
      "type": "remote",
      "url": "https://mcp.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ..."
      },
      "timeout": 30000
    }
  }
}
```

Wingman tries the streamable HTTP transport first, then falls back to SSE when the server requires it. `timeout` is measured in milliseconds.

## Use MCP Tools In An Agent

Restart Wingman after changing `wingman.json`. It connects enabled MCP servers at startup and lists their tools through the Console's Tools page or `GET /tools`.

Wingman prefixes each MCP tool with its server name. For example, a remote tool named `search` from `company-tools` becomes `company_tools_search`.

Add that name to an agent's `tools` allow-list:

```json
{
  "name": "Company Researcher",
  "instructions": "Use the company search tool when it helps answer the request.",
  "model_ref": "anthropic/claude-sonnet-5",
  "tools": ["company_tools_search"]
}
```

Only connected MCP tools are available to agents. Use the Console at `http://127.0.0.1:2323/console/tools` to inspect configured servers, tool names, and connection errors.

To inspect the daemon directly:

```bash
curl -sS http://127.0.0.1:2323/mcp | jq
curl -sS http://127.0.0.1:2323/tools | jq '.tools[] | select(.source == "mcp")'
```

`/mcp` lists every configured server and its connection status. `/tools` lists the MCP tools currently available to agents.

## Enable And Disable Servers

MCP servers are enabled by default. Set `enabled` to `false` to keep a server configured without connecting it at startup:

```json
{
  "mcp": {
    "company-tools": {
      "type": "remote",
      "url": "https://mcp.example.com/mcp",
      "enabled": false
    }
  }
}
```

Use the Console to connect or disconnect enabled configured servers without changing the file.

## Current Limits

- MCP OAuth login is not implemented. Supply any required credentials through configured request headers or a local server's environment.
- MCP servers run with the same operating-system permissions as the Wingman process. Only configure servers you trust.
- MCP configuration is daemon-wide, not stored in SQLite or attached to individual agents.
