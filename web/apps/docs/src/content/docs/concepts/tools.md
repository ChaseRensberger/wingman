---
title: "Tools"
group: "Core"
order: 106
---

# Tools

Tools are functions that a model can call. An agent's `tools` list selects which tools it can use.

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

File and shell tools require a working directory. Set `working_directory` or use a [Workspace](/concepts/workspaces) with a path.
This example connects to the local managed service:

```bash
SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Project\",\"working_directory\":\"$PWD\"}" | jq -r .id)
```

The working directory does not restrict file or network access. Tools run with Wingman's operating system permissions.
`bash` can run arbitrary commands. Enable it only for trusted work.

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
Wingman adds `skill` automatically when it discovers local skills. Do not add
`skill` to an Agent's `tools` list. See [Skills](/configure/skills).

## Web Search Configuration

Set search variables in the Wingman server's environment, not the browser.
`WINGMAN_WEBSEARCH_PROVIDER` accepts `exa` or `parallel`. The default is `exa`. Other values fail the tool call.

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

`query` is required. Exa also accepts `numResults`, `livecrawl`, `type`, and `contextMaxCharacters`. Parallel receives only the query.
For a known URL, use `webfetch` instead.

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

`Definition()` describes the tool's JSON Schema and execution requirements. Wingman rejects invalid definitions and duplicate names.
`Execute` receives the model's input and returns a result:

- `Text` goes to the model.
- `Structured` holds data for clients.
- `Metadata` holds display hints for clients.

Wingman saves all three fields. It does not send `Structured` or `Metadata` to the model.
If the tool declares `output_schema`, `Structured` must match it. Invalid output fails with `error_type: result_validation`.

Call `invocation.Progress.Report(outputDelta, metadata)` for live progress.
Progress is not replayed. Wingman saves the final result and retains partial output if execution fails.

## Durable Execution Lifecycle

Persisted sessions assign each invocation a Wingman `tool_use_id` with a `tlu_` prefix. It is separate from the provider `call_id`, which providers can reuse. The durable lifecycle is:

```text
proposed -> authorized -> started -> completed | failed | interrupted
         \-> declined
```

Wingman records `started` before execution. Invalid, denied, or skipped calls become `declined` without running.
An `ask` rule waits for [approval](/configure/permissions#interactive-approval) before authorization.

On restart, unfinished calls become `interrupted`. Recovery reuses completed results instead of executing those calls again.
Calls that did not start can continue. Wingman repeats a started call only when its saved and current tool definitions permit replay.
The built-in `read`, `glob`, `grep`, `webfetch`, and `websearch` tools permit replay. Their new results can differ from earlier observations.

In Go, set `tool.Definition.ReplaySafe` to `true` only when repeating the same call is safe.
The default is `false`. File mutation tools, shell commands, MCP tools, and external plugin tools do not permit automatic replay by default.
Tool hooks can also run again after an interruption. Keep their effects safe to repeat.
Wingman retains saved authorization and input when it resumes an authorized call.

A tool can change external state before a crash, even if its result was not saved. If replay is not permitted, recovery fails with `recovery_blocked`.
Read `GET /sessions/{id}/tool-uses` for execution status. Inspect external effects before you submit replacement work.
Request-ID deduplication does not prevent duplicate external effects. A tool that permits replay must provide that guarantee itself.

File tools use `filePath`,
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

If any tool is sequential, Wingman runs the whole batch in sequence. External tools use `sequential: true`.
They can also declare a permission action and `resource_fields`. Wingman reads resources from validated input, or uses `*` if none exist.

## Custom Tools

Use Go plugins in embedded applications or custom binaries.
Use external plugins to add tools to `wingman serve` without rebuilding it.
See [Plugins](/concepts/plugins) for installation and examples.

## Tool Results

Session history stores tool calls and results on the assistant message that requested them.
Tool errors return to the model so it can respond or try another action.
A tool error alone does not fail the turn.
