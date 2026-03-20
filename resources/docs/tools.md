---
title: "Tools"
group: "Concepts"
draft: false
order: 105
---

# Tools

Tools are capabilities that an agent can invoke while a session is running. They let the model interact with the local filesystem, shell, or external web resources.

## Built-in tools

Wingman currently ships with eight built-in tools:

| Tool | Name | Purpose |
|---|---|---|
| Bash | `bash` | Execute shell commands |
| Read | `read` | Read files from disk |
| Write | `write` | Create or overwrite files |
| Edit | `edit` | Make exact string replacements in files |
| Glob | `glob` | Find files by glob pattern |
| Grep | `grep` | Search file contents with regex |
| WebFetch | `webfetch` | Fetch URL content as text or markdown |
| Perplexity | `perplexity_search` | Search the web through Perplexity |

## Server usage

On the server, agents reference built-in tools by name.

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodeAssistant",
    "instructions": "You are a helpful coding assistant.",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": "anthropic",
    "model": "claude-sonnet-4-5"
  }'
```

## SDK usage

In the SDK, built-in tools are created with constructors from `tool/`.

```go
agent.New("MyAgent",
    agent.WithTools(
        tool.NewBashTool(),
        tool.NewReadTool(),
        tool.NewWriteTool(),
        tool.NewEditTool(),
        tool.NewGlobTool(),
        tool.NewGrepTool(),
        tool.NewWebFetchTool(),
        tool.NewPerplexityTool(),
    ),
)
```

## Custom tools

Custom tools are supported in the SDK by implementing `core.Tool`.

```go
type Tool interface {
    Name() string
    Description() string
    Definition() core.ToolDefinition
    Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}
```

The HTTP server only resolves the built-in tool names.
