---
title: "Tools"
group: "Core"
order: 106
---

# Tools

Tools are functions that the model can call during a session turn. An agent stores an allow-list of tool names. When a session runs, Wingman resolves these names to live `tool.Tool` implementations. It sends JSON Schema definitions to the model provider. It dispatches each model tool call.

## Built-In Tools

Wingman ships these built-ins:

| Name          | Purpose                                                                            | Requires `work_dir` |
| ------------- | ---------------------------------------------------------------------------------- | ------------------- |
| `bash`        | Execute a `bash -c` command with an optional timeout.                              | Yes                 |
| `read`        | Read a file or directory with `filePath`, optional `offset`, and optional `limit`. | Yes                 |
| `write`       | Write or overwrite `filePath`, creating parent directories as needed.              | Yes                 |
| `edit`        | Replace `oldString` with `newString` in `filePath`. Optionally use `replaceAll`.   | Yes                 |
| `apply_patch` | Apply a file-oriented patch described by `patchText`.                              | Yes                 |
| `glob`        | List files matching a glob pattern.                                                | Yes                 |
| `grep`        | Search text files with a regular expression.                                       | Yes                 |
| `webfetch`    | Fetch HTTP(S) content as markdown, text, or HTML.                                  | No                  |
| `websearch`   | Search the web for current information through a configured search provider.       | No                  |
| `skill`       | Load discovered local Agent Skill instructions or supporting files.                | No                  |

Directory-scoped tools require a session with a working directory. Before you allow file or shell tools, create the session with `working_directory` or `workspace_id`. You can also move the session with `POST /sessions/{id}/move`.

These commands find and authenticate with the managed daemon.

```bash
SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Project\",\"working_directory\":\"$PWD\"}" | jq -r .id)
```

For repeated work in one directory, use a [Workspace](/concepts/workspaces).
Create sessions with `workspace_id`.

`DirectoryScopedTool` is a session-start requirement. It is not a security sandbox. It requires a non-empty working directory. It does not confine a tool process, network access, or all filesystem operations to that directory. `bash` starts in that directory and can run arbitrary shell commands. Enabled tools and Wingman process OS permissions are the security boundary.

`bash` has a default timeout of two minutes. Its optional `timeout` is a Go duration, for example `30s` or `5m`. Invalid values use the default. Wingman does not impose a separate maximum. It streams combined standard output and standard error during the command.

`webfetch` performs only an HTTP(S) `GET`. Its default timeout is 30 seconds. It limits a supplied timeout to 120 seconds. It accepts only `200 OK`. It rejects responses larger than 5 MiB. Markdown is the default output format. HTML conversion is basic.

## Allow Tools On An Agent

Agents store tool names in `tools`:

```bash
wingman api createAgent -d '{
        "name": "Researcher",
        "instructions": "Answer with citations when useful.",
        "tools": ["websearch", "webfetch", "grep", "glob", "read"],
        "model_ref": "anthropic/claude-sonnet-5"
      }'
```

Agent creation and tool-list updates reject unknown and duplicate names. If an
allowed tool is unavailable when a session starts, the session fails to start.
Wingman never silently removes a tool.

Wingman adds `skill` automatically when it discovers local skills. Do not add
`skill` to an Agent's `tools` list. See [Skills](/configure/skills).

## Web Search Configuration

`websearch` uses Exa by default. The provider comes from the Wingman server process environment when the tool runs. It does not come from the browser or model environment. Set `WINGMAN_WEBSEARCH_PROVIDER` to `exa` or `parallel`. Any other value fails the tool call.

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

The tool accepts a required `query`. It accepts optional `numResults`, `livecrawl`, `type`, and `contextMaxCharacters` fields. Exa receives optional search controls. Parallel receives only the query. The Parallel service can ignore controls and return different results. Both providers are external services. The selected service determines availability, authentication requirements, result quality, and live-crawl support. Use `websearch` for current or discoverable information. If you have a specific URL, use `webfetch`.

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
	Text       string
	Structured any
	Metadata   map[string]any
}
```

`Definition()` returns the JSON-Schema-shaped declaration for the model. It also returns optional execution traits and an `output_schema`. Wingman validates definitions when it composes the catalog. It rejects duplicate names. `Execute` runs after the model emits a matching tool call. A tool can call `invocation.Progress.Report(outputDelta, metadata)` to publish live progress. Tools without incremental work ignore it. `Result.Text` returns to the model. `Result.Structured` is optional client-neutral structured content. `Result.Metadata` contains client rendering hints. Both are durable. Neither is model-visible output.

Progress events are live-only. Wingman stores the final result, metadata, error, and timing on the assistant-owned tool part. Reconnecting clients recover the completed state without replaying each output chunk. If execution fails after it produces output, Wingman retains that partial output separately from the error. If a definition declares `output_schema`, successful execution must return matching `Structured` content. Missing or invalid content settles the tool use as failed with `error_type: result_validation`. Wingman preserves returned text, structured content, and metadata for diagnosis.

## Durable Execution Lifecycle

Persisted sessions assign each invocation a Wingman `tool_use_id` with a `tlu_` prefix. It is separate from the provider `call_id`, which providers can reuse. The durable lifecycle is:

```text
proposed -> authorized -> started -> completed | failed | interrupted
         \-> declined
```

Wingman checkpoints proposed IDs on the assistant message before tool workers start. Hooks can then rewrite input. Validation and permissions then run. Wingman stores allowed input as `authorized`. `started` must commit before `Execute` runs. Unknown tools, invalid input, skipped calls, denied calls, and calls that require unavailable approval settle as `declined` without execution.

An `ask` permission suspends between proposal and authorization. Wingman stores the pending request. It emits `session.permission.requested`. No tool side effect can occur while it waits. `once` or `always` continues authorization. Reject, timeout, cancellation, and recovery settle the request and tool without execution. An `always` reply remembers only the exact requested action/resources for that session.

On restart, unfinished tool uses become `interrupted`. Wingman does not replay them automatically. This prevents blind repetition. It is not an exactly-once guarantee. A process can crash after an external side effect and before terminal settlement. If recovery decisions need the authoritative lifecycle, inspect `GET /sessions/{id}/tool-uses`. Do not use transcript presentation state.

File-oriented tools use OpenCode-style model-facing argument names: `filePath`,
`oldString`, `newString`, `replaceAll`, `content`, and `patchText`.
Search-scoped tools use `path` for the base path of a search (`glob`, `grep`).

Tools that need a working directory implement `DirectoryScopedTool`:

```go
type DirectoryScopedTool interface {
    Tool
    DirectoryScoped()
}
```

This marker causes Wingman to reject a session without a working directory before it starts. It does not provide sandboxing or path-based access control.

External tools declare the equivalent with `directory_scoped: true` in their
definition.

Tools that must not run in parallel with other tool calls implement `SequentialTool`:

```go
type SequentialTool interface {
    Tool
    Sequential() bool
}
```

If any tool in a batch is sequential, Wingman runs the whole batch sequentially. External definitions declare the equivalent with `sequential: true`. They can also declare a permission action and `resource_fields`. Wingman reads those fields from validated input. If none contain resources, it uses `*`.

## Custom Tools

There are two extension paths:

- In-process Go plugins can register `tool.Tool` implementations through the plugin registry.
- External plugins can expose tool specifications from a manifest and run tool calls over stdio JSON-RPC.

If you control the embedding process and need typed hooks, use Go tools. If you want to extend the stock `wingman serve` binary without rebuilding it, use external plugin tools.

See [Plugins](/concepts/plugins) for plugin installation and manifest details.

## Tool Results

Tool text returns to the model. Session history keeps each invocation on its assistant message as a tool part. It has pending, running, completed, or error state. Optional structured content and metadata remain on that part. They are not model-visible output. If a pre-tool hook stops a run, Wingman keeps the pending tool part in history. It does not discard the model action. Tool errors become error-shaped results for the model to process. They do not fail the session turn unless the surrounding loop or request is canceled.
