---
title: "Tools"
group: "Primitives"
order: 103
---

# Tools

Tools are capabilities that [agents](/docs/agents) can invoke during execution. Not all models support tool use.

## Built-in Tools

| Tool | Description |
|------|-------------|
| `bash` | Execute shell commands (2min default timeout) |
| `read` | Read file contents |
| `write` | Write/overwrite files |
| `edit` | Find and replace in files |
| `glob` | Find files by pattern |
| `grep` | Search file contents with regex |
| `webfetch` | Fetch URL contents |

## SDK Usage

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

## Server Usage

Specify tools by name when creating an [agent](/docs/agents):

```json
{
  "tools": ["bash", "read", "write", "edit", "glob", "grep"]
}
```

See [Agents — Server Usage](/docs/agents) for the full agent creation payload.

## Custom Tools

Implement the `Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    Definition() models.WingmanToolDefinition
    Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}
```

Register custom tools with a `tool.Registry`:

```go
registry := tool.NewRegistry()
registry.Register(myCustomTool)
```
