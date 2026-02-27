---
title: "Tools"
group: "Primitives"
draft: true
order: 103
---

# Tools

Tools are capabilities that agents can invoke during execution. Not all models support tools.

| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands (2min default timeout) |
| `read` | Read file contents |
| `write` | Write/overwrite files |
| `edit` | Find and replace in files |
| `glob` | Find files by pattern |
| `grep` | Search file contents with regex |
| `webfetch` | Fetch URL contents |

## Usage

### HTTP Server

Specify tools by name when creating an agent:

```bash
curl -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "CodeAssistant",
    "instructions": "You are a helpful coding assistant.",
    "tools": ["bash", "read", "write", "edit", "glob", "grep"],
    "provider": "anthropic",
    "model": "claude-sonnet-4-5",
    "options": {
      "max_tokens": 4096
    }
  }'
```

### Go SDK

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
    ),
)
```

## Custom Tools

Implement the `Tool` interface (SDK only). The server only resolves the 7 built-in tools by name.

```go
type Tool interface {
    Name() string
    Description() string
    Definition() core.ToolDefinition
    Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}
```
