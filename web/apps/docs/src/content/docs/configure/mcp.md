---
title: "MCP Servers"
description: "Connect Model Context Protocol servers and use their tools with Wingman agents."
---

# Configure MCP Servers

Add Model Context Protocol (MCP) servers to `~/.config/wingman/wingman.json`.
Their tools then become available to Wingman agents. If `XDG_CONFIG_HOME` is set, use `$XDG_CONFIG_HOME/wingman/wingman.json` instead.

Wingman supports local stdio servers and remote HTTP servers. Wingman validates MCP configuration at startup.
Enabled servers connect when Wingman creates an execution scope.

## Add A Local Server

To start an MCP server as a subprocess over stdio, use `type: "local"`:

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

`command` runs directly without shell expansion. Put the executable in one array item.
Put each argument in a separate array item. `cwd` supports `~` and `~/...` paths.

## Chrome DevTools example

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

Use `type: "remote"` for a remote MCP endpoint. Put required credentials in `headers`.
Wingman does not provide an MCP OAuth login command.

```json
{
  "mcp": {
    "company-tools": {
      "type": "remote",
      "url": "https://mcp.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ..."
      },
      "discovery_timeout": 30000,
      "execution_timeout": 120000
    }
  }
}
```

## Use MCP Tools In An Agent

After you change `wingman.json`, restart Wingman. After the restart, Wingman lists connected tools on the Console Tools page and at `GET /tools`.

Wingman prefixes each MCP tool with its server name. For example, the remote
`search` tool from `company-tools` becomes `company_tools_search`.

Add that name to an agent `tools` allow-list:

```json
{
  "name": "Company Researcher",
  "instructions": "Use the company search tool when it helps answer the request.",
  "model_ref": "anthropic/claude-sonnet-5",
  "tools": ["company_tools_search"]
}
```

Only connected MCP tools are available to agents. Use the Console at `http://127.0.0.1:2323/console/tools` to view the directoryless scope.

Agent writes reject disconnected or unknown MCP tool names. If sanitized MCP tool names collide, tool catalog creation fails.

To view the daemon directly, run these commands:

These commands use HTTP Basic authentication. Load managed-service credentials with `source ~/.config/wingman/service.env`.
The username defaults to `wingman`. See
[HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
curl -sS http://127.0.0.1:2323/mcp \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" | jq
curl -sS http://127.0.0.1:2323/tools \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" | jq '.tools[] | select(.source == "mcp")'
```

`/mcp` lists each configured server and its connection status. `/tools` lists the MCP tools available to agents.

`discovery_timeout` limits connection and tool discovery. `execution_timeout` limits each MCP tool call.
Both values are in milliseconds. If omitted, both default to `30000`.

## Enable And Disable Servers

MCP servers are enabled by default. To keep a server configured without connecting it at startup, set `enabled` to `false`:

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

- MCP OAuth login is not available. Supply required credentials through configured request headers or a local server environment.
- MCP servers run with the same permissions as the Wingman process. Configure only servers that you trust.
- MCP configuration is daemon-wide. Each execution scope owns its runtime connections.
