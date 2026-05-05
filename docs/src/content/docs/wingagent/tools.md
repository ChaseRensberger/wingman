---
title: "Tools"
group: "Concepts"
draft: false
order: 105
---

# Tools

Tools are capabilities the model may invoke during a session. They let the agent interact with the local filesystem, shell, or external web resources.

## Built-in tools

Built-in tools live under `tool`.

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

## SDK usage

Built-in tools are constructors in `tool`.

```go
import (
    "github.com/chaserensberger/wingman/wingagent/session"
    "github.com/chaserensberger/wingman/tool"
)

s := session.New(
    session.WithModel(p),
    session.WithTools(
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

Tools see the session's `WorkDir` as their `workDir` parameter. Set it via `session.WithWorkDir(dir)` or `s.SetWorkDir(dir)`.

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
    "model": "claude-haiku-4-5"
  }'
```

The HTTP server only resolves built-in tool names. Custom tools are an SDK concern.

## Custom tools

Custom tools implement the `tool.Tool` interface:

```go
type Tool interface {
    Name() string
    Description() string
    Definition() ToolDefinition
    Execute(ctx context.Context, params map[string]any, workDir string) (string, error)
}
```

`Name` must be stable and unique within the session's tool set. `Definition` returns a JSON Schema describing the parameters; the loop forwards it to the provider so the model can plan calls. `Execute` runs synchronously when invoked; the loop runs tool calls in parallel by default (one goroutine per call within a single assistant turn) and waits for all of them before the next iteration.

To opt a tool into sequential execution within a turn, embed `tool.SequentialTool`:

```go
type myTool struct {
    tool.SequentialTool
    // ...
}
```

The loop respects this marker and serializes calls to that tool relative to the others in the same turn.

## Tool gates and rewriting

Hooks let you gate, rewrite, or observe tool calls without modifying the tools themselves:

- `BeforeToolCall` may rewrite arguments or return `loop.ErrSkipTool` to deny.
- `AfterToolCall` may rewrite the result string.
- `ToolExecutionStartEvent` / `ToolExecutionEndEvent` give read-only observability.

See [Lifecycle hooks](./lifecycle).
