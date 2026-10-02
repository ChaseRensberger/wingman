---
title: "MCP Servers"
description: "Connect Model Context Protocol servers and use their tools with Wingman agents."
---

# Configure MCP Servers

Model Context Protocol (MCP) connects agents to external tools.
Add servers to `~/.config/wingman/wingman.json`.
Their tools then become available to Wingman agents. If `XDG_CONFIG_HOME` is set, use `$XDG_CONFIG_HOME/wingman/wingman.json` instead.

Local servers use standard input and output. Remote servers use HTTP.

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

After changing `wingman.json`, restart Wingman. Connected tools appear on the Console Tools page and at `GET /tools`.

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

Agents can use only connected tools. Agent creation and updates reject unknown or disconnected tool names.
Duplicate names after normalization prevent the tool catalog from loading.
To list servers and tools on the local managed service:

```bash
wingman api listMCPServers | jq
wingman api listTools | jq '.tools[] | select(.source == "mcp")'
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
