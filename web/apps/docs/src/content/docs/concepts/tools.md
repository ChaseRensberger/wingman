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
| `bash` | Execute a `bash -c` command with an optional timeout. | Yes |
| `read` | Read a file or directory with `filePath`, optional `offset`, and optional `limit`. | Yes |
| `write` | Write or overwrite `filePath`, creating parent directories as needed. | Yes |
| `edit` | Replace `oldString` with `newString` in `filePath`; optionally use `replaceAll`. | Yes |
| `apply_patch` | Apply a file-oriented patch described by `patchText`. | Yes |
| `glob` | List files matching a glob pattern. | Yes |
| `grep` | Search text files with a regular expression. | Yes |
| `webfetch` | Fetch HTTP(S) content as markdown, text, or HTML. | No |
| `websearch` | Search the web for current information through a configured search provider. | No |

Directory-scoped tools require the session to have a working directory. Create the session with `working_directory` or `workspace_id`, or move it with `POST /sessions/{id}/move`, before allowing file or shell tools.

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Project\",\"working_directory\":\"$PWD\"}" | jq -r .id)
```

For repeated work in the same directory, prefer a [Workspace](/concepts/workspaces) and create sessions with `workspace_id`.

`DirectoryScopedTool` is a session-start requirement, not a security sandbox. It ensures a non-empty working directory is present; it does not confine a tool's process, network access, or every filesystem operation to that directory. In particular, `bash` runs with that directory as its starting directory and can run arbitrary shell commands. Treat enabled tools and the Wingman process's OS permissions as the security boundary.

`bash` defaults to a two-minute timeout. Its optional `timeout` is parsed as a Go duration (for example, `30s` or `5m`); invalid values fall back to the default, and Wingman does not impose a separate maximum. It streams combined standard output and standard error while the command runs.

`webfetch` performs an HTTP(S) `GET` only. It defaults to a 30-second timeout, clamps a supplied timeout to 120 seconds, accepts only `200 OK`, and rejects responses larger than 5 MiB. Markdown is the default output format; HTML conversion is intentionally basic.

## Allow Tools On An Agent

Agents store tool names in `tools`:

```bash
curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
        "name": "Researcher",
        "instructions": "Answer with citations when useful.",
        "tools": ["websearch", "webfetch", "grep", "glob", "read"],
        "model_ref": "anthropic/claude-sonnet-5"
      }'
```

The model only sees tools that survive resolution. Unknown names are dropped when the session is built.

## Web Search Configuration

`websearch` uses Exa by default. The provider is read from the environment of the Wingman server process when the tool runs, not from the browser or the model environment. Set `WINGMAN_WEBSEARCH_PROVIDER` to `exa` or `parallel`; any other value fails the tool call.

For Exa, Wingman includes `EXA_API_KEY` when it is set:

```bash
export WINGMAN_WEBSEARCH_PROVIDER=exa
export EXA_API_KEY=your_exa_key
```

For Parallel, Wingman sends `PARALLEL_API_KEY` as a bearer token when it is set:

```bash
export WINGMAN_WEBSEARCH_PROVIDER=parallel
export PARALLEL_API_KEY=your_parallel_key
```

The tool accepts a required `query` plus optional `numResults`, `livecrawl`, `type`, and `contextMaxCharacters` fields. Exa receives those optional search controls. Parallel currently receives the query only, so its service may ignore those controls and return results with different behavior. Both providers are external services: availability, authentication requirements, result quality, and live-crawl support are determined by the selected service. Use `websearch` when the model needs current or discoverable information; use `webfetch` when you already have a specific URL.

## Runtime Contract

In Go, a tool implements `tool.Tool`:

```go
type Tool interface {
    Name() string
    Description() string
    Definition() Definition
    Execute(ctx context.Context, invocation Invocation) (Result, error)
}

type Invocation struct {
    Input    map[string]any
    WorkDir  string
    Progress *Progress
}

type Result struct {
    Text     string
    Metadata map[string]any
}
```

`Definition()` returns the JSON-Schema-shaped declaration sent to the model. `Execute` runs after the model emits a matching tool call. A tool may call `invocation.Progress.Report(outputDelta, metadata)` to publish live progress; tools without incremental work ignore it. `Result.Text` is returned to the model as the final tool result. `Result.Metadata` is persisted for clients that want richer rendering, such as file diff cards in the web UI.

Progress events are live-only. The final result, metadata, error, and timing are
stored on the assistant-owned tool part, so reconnecting clients recover the
completed state without replaying every output chunk. If execution fails after
producing output, Wingman retains that partial output separately from the error.

File-oriented tools use OpenCode-style model-facing argument names: `filePath`, `oldString`, `newString`, `replaceAll`, `content`, and `patchText`. Search-scoped tools use `path` where it means the base path for a search (`glob`, `grep`).

Tools that need a working directory implement `DirectoryScopedTool`:

```go
type DirectoryScopedTool interface {
    Tool
    DirectoryScoped()
}
```

This marker causes Wingman to reject a session without a working directory before it starts; it does not provide sandboxing or path-based access control.

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

Tool outputs are returned to the model as text. Session history keeps each
invocation on its assistant message as a tool part with pending, running,
completed, or error state. Optional metadata is retained on that part but is
not sent as model-visible output. If a pre-tool hook stops a run, Wingman keeps
the pending tool part in history rather than discarding the model action. Tool
errors become error-shaped results for the model to react to; they do not
automatically fail the whole session turn unless the surrounding loop or
request is cancelled.
