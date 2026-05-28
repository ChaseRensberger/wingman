---
title: "Tools"
group: "Core"
order: 106
---

# Tools

Tools are functions the model can call during a session turn. An agent stores an allow-list of tool names. When a session runs, Wingman resolves those names into live `tool.Tool` implementations, sends their JSON Schema definitions to the model provider, and dispatches any tool calls the model emits.

## Built-In Tools

Wingman ships these built-ins:

| Name | Purpose | Requires `work_dir` |
|---|---|---|
| `bash` | Execute a shell command with an optional timeout. | Yes |
| `read` | Read a file or directory with `filePath`, optional `offset`, and optional `limit`. | Yes |
| `write` | Write or overwrite `filePath`, creating parent directories as needed. | Yes |
| `edit` | Replace `oldString` with `newString` in `filePath`; optionally use `replaceAll`. | Yes |
| `apply_patch` | Apply a file-oriented patch described by `patchText`. | Yes |
| `glob` | List files matching a glob pattern. | Yes |
| `grep` | Search text files with a regular expression. | Yes |
| `webfetch` | Fetch HTTP(S) content as markdown, text, or HTML. | No |

Directory-scoped tools require the session to have a working directory. Create or update the session with `working_directory`/`work_dir`, or create it from a Workspace with `workspace_id`, before allowing file or shell tools.

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Project\",\"working_directory\":\"$PWD\"}" | jq -r .id)
```

For repeated work in the same directory, prefer a [Workspace](/concepts/workspaces) and create sessions with `workspace_id`.

## Allow Tools On An Agent

Agents store tool names in `tools`:

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Researcher",
        "instructions": "Answer with citations when useful.",
        "tools": ["webfetch", "grep", "glob", "read"],
        "model_ref": "anthropic/claude-sonnet-4-6"
      }'
```

The model only sees tools that survive resolution. Unknown names are dropped when the session is built.

## Runtime Contract

In Go, a tool implements `tool.Tool`:

```go
type Tool interface {
    Name() string
    Description() string
    Definition() Definition
    Execute(ctx context.Context, params map[string]any, workDir string) (Result, error)
}

type Result struct {
    Text     string
    Metadata map[string]any
}
```

`Definition()` returns the JSON-Schema-shaped declaration sent to the model. `Execute` runs after the model emits a matching tool call. `Result.Text` is returned to the model as the tool result. `Result.Metadata` is persisted for clients that want richer rendering, such as file diff cards in the web UI.

File-oriented tools use OpenCode-style model-facing argument names: `filePath`, `oldString`, `newString`, `replaceAll`, `content`, and `patchText`. Search-scoped tools use `path` where it means the base path for a search (`glob`, `grep`).

Tools that touch the working directory implement `DirectoryScopedTool`:

```go
type DirectoryScopedTool interface {
    Tool
    DirectoryScoped()
}
```

Tools that should not run in parallel with other tool calls implement `SequentialTool`:

```go
type SequentialTool interface {
    Tool
    Sequential() bool
}
```

If any tool in a batch is sequential, Wingman runs the whole batch sequentially.

## Custom Tools

There are two extension paths:

- In-process Go plugins can register `tool.Tool` implementations through the plugin registry.
- External plugins can expose tool specs from a manifest and execute tool calls over stdio JSON-RPC.

Use Go tools when you control the embedding process and need typed hooks. Use external plugin tools when you want to extend the stock `wingman serve` binary without rebuilding it.

See [Plugins](/concepts/plugins) for plugin installation and manifest details.

## Tool Results

Tool outputs are returned to the model as text. Optional metadata is stored on the tool result part but is not sent as model-visible output. Tool errors become error-shaped tool results for the model to react to; they do not automatically fail the whole session turn unless the surrounding loop or request is cancelled.
