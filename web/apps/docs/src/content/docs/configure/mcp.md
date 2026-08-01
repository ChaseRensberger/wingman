---
title: "MCP Servers"
description: "Connect Model Context Protocol servers and use their tools in Wingman agents."
---

# Configure MCP Servers

Add Model Context Protocol (MCP) servers to `~/.config/wingman/wingman.json` to make their tools available to Wingman agents. If `XDG_CONFIG_HOME` is set, use `$XDG_CONFIG_HOME/wingman/wingman.json` instead.

Wingman supports local stdio servers and remote servers. The daemon rejects an
invalid type, missing local command, invalid remote URL, or negative timeout at
startup. Enabled servers connect when Wingman creates an execution scope.

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

`command` runs directly, without shell expansion. Put the executable and every argument in a separate array item. `cwd` supports `~` and `~/...` paths.

## Chrome DevTools

(as an example)

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

## Add A Remote Server

Use `type: "remote"` for a remote MCP endpoint. Currently remote MCPs need an authorization header and you can't run like `wingman mcp auth {name}` to use OAuth for a remote MCP but this is coming soon.

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

Only connected MCP tools are available to agents. Use the Console at `http://127.0.0.1:2323/console/tools` to inspect the directoryless scope.

Agent writes reject disconnected or unknown MCP tool names. If two sanitized MCP
names collide with each other or another tool source, catalog composition fails
explicitly. MCP input and output schemas are preserved; successful
`structuredContent` is validated against the advertised output schema and stored
separately from model-facing text and client metadata.

To inspect the daemon directly:

These commands use `WINGMAN_TOKEN` from [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS http://127.0.0.1:2323/mcp \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" | jq
curl -sS http://127.0.0.1:2323/tools \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" | jq '.tools[] | select(.source == "mcp")'
```

`/mcp` lists every configured server and its connection status. `/tools` lists the MCP tools currently available to agents.

Each execution scope owns separate MCP connections. Sessions in one canonical
working directory share those connections. Reconnect and disconnect operations
on `/mcp` apply to the directoryless scope.

An MCP reconnect stages the connection and tool discovery before publication.
If the candidate fails, the current healthy connection stays active. A
successful replacement rejects new calls through stale tools, drains active
calls for a bounded period, and then closes the old connection.

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
- MCP servers run with the same permissions as the Wingman process. Only configure servers you trust.
- MCP configuration is daemon-wide, but each execution scope owns its runtime connections.
