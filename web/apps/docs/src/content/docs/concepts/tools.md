---
title: "Tools"
description: "Select tools and understand their execution rules."
---

# Tools

A tool is a function that a model can call. An agent's `tools` list selects which tools it can use.
[Permission rules](/configure/permissions) decide whether each call runs, waits for approval, or is denied.

## Built-in tools

| Tool          | Purpose                                      | Needs a working directory |
| ------------- | -------------------------------------------- | ------------------------- |
| `bash`        | Run a shell command.                         | Yes                       |
| `read`        | Read a file or directory.                    | Yes                       |
| `write`       | Create or overwrite a file.                  | Yes                       |
| `edit`        | Replace text in a file.                      | Yes                       |
| `apply_patch` | Apply a patch to files.                      | Yes                       |
| `glob`        | Find files by name pattern.                  | Yes                       |
| `grep`        | Search file contents.                        | Yes                       |
| `webfetch`    | Fetch an HTTP or HTTPS URL.                  | No                        |
| `websearch`   | Search the web.                              | No                        |
| `skill`       | Load skill instructions or supporting files. | No                        |
| `question`    | Request information or a decision.           | No                        |

Use `GET /tools` for current schemas and input fields. It also lists global plugin and connected MCP tools.
Agent creation and tool-list updates reject unknown or duplicate names.
Wingman adds `skill` automatically. Do not add it to an agent's tool list.

Add `question` to an agent's tool list when its client can answer questions.
The Console supports choices, multiple selections, custom text, and dismissal.
The tool uses saved sessions. Wingman omits it from one-shot `/run` requests.
See [Questions](/concepts/sessions#questions) for the request and reply behavior.

Set a session's `working_directory` for file and shell tools. This directory sets the default location, not an isolation boundary.
File tools request approval for outside access unless a permission rule allows or denies it.
Tools run with Wingman's operating-system permissions. Enable `bash` only for trusted work because it can run arbitrary commands.

`bash` runs `bash -c`. Its default timeout is two minutes.
The optional `timeout` is a Go duration such as `30s` or `5m`. Invalid duration strings use the default.
It streams combined standard output and standard error. Wingman imposes no separate maximum timeout.

`webfetch` sends `GET` requests and accepts only `200 OK` responses up to 5 MiB.
Its default timeout is 30 seconds, with a maximum of 120 seconds. Output defaults to Markdown. HTML conversion is basic.

## Code search and file changes

The `grep` and `glob` tools require ripgrep, a file-search program, on the computer that runs Wingman.
Install ripgrep and make sure that `rg` is on the Wingman process's `PATH`. The Wingman container includes it.
Wingman reports a tool error if `rg` is missing.

Both tools use ripgrep's ignore-file rules. Explicit inclusion patterns can override those rules.
Hidden files are excluded unless `hidden` is `true`. Both tools exclude the `.git` directory.
Searches stop after 30 seconds. The default `limit` is 100 results, and a result-limit notice tells the model to narrow its search.

`grep` accepts ripgrep regular expressions and skips binary files. Use `literal: true` for exact text or `caseSensitive: false` for case-insensitive matches.
Its `include` field accepts patterns such as `src/**/*.{ts,tsx}`. The `glob` tool accepts the same pattern syntax in `pattern`.

New Build agents include `apply_patch`, `edit`, and `write`. Existing agents keep their configured tool lists.
Wingman serializes its file changes to the same resolved paths. External programs can still change those files at the same time.
Text changes preserve an existing UTF-8 byte-order mark and a consistent CRLF line-ending style.

`apply_patch` validates all file sections before it changes any files. Context within each patch section must match consecutive lines.
A move cannot overwrite an existing destination. A write failure can still leave partial changes, which the tool reports.

## Web search configuration

Set these variables in the server process environment, not the browser:

| Provider         | Variables                                                          |
| ---------------- | ------------------------------------------------------------------ |
| Exa, the default | `WINGMAN_WEBSEARCH_PROVIDER=exa`, optional `EXA_API_KEY`           |
| Parallel         | `WINGMAN_WEBSEARCH_PROVIDER=parallel`, optional `PARALLEL_API_KEY` |

For example, start a foreground server with Exa:

```bash
export WINGMAN_WEBSEARCH_PROVIDER=exa
export EXA_API_KEY=your_exa_key
wingman serve
```

The managed service does not automatically import these search variables from your shell.
`query` is required. Exa also accepts `numResults`, `livecrawl`, `type`, and `contextMaxCharacters`. Parallel uses only the query.

## Durable execution lifecycle

Saved sessions assign each call a `tool_use_id` with a `tlu_` prefix.
This ID differs from the provider's `call_id`, which can repeat.

```text
proposed -> authorized -> started -> completed | failed | interrupted
         \-> declined
```

Wingman saves `started` before execution. Invalid, denied, and skipped calls become `declined` without running.
An `ask` permission rule waits for approval before authorization.

A question tool call can move from `started` to `waiting`.
After the client answers or dismisses the question, the same call resumes and returns the result to the model.
Wingman retains the question identity and answer across server restarts.

After a restart, Wingman reuses saved results. Calls that did not start can continue.
It repeats a started call only when both its saved and current definitions permit replay.
Built-in `read`, `glob`, `grep`, `webfetch`, and `websearch` calls permit replay. Their new results can differ.

File changes, shell commands, MCP tools, and external plugin tools do not permit automatic replay by default.
If a started call has no saved result and cannot be repeated safely, recovery fails with `recovery_blocked`.
Inspect its external effects before submitting replacement work.
See [Sessions](/concepts/sessions#run-status-and-recovery) for recovery limits.

## Custom tools

Go tools implement `tool.Tool`. Use [Go plugins](/extend/plugin-quickstart) to install them in an embedded session.
Use [RPC plugins](/extend/rpc-plugin-protocol) to add tools to the standard server.

`Execute(ctx, invocation)` receives input, working directory, execution IDs, and a progress callback.
Its `tool.Result` separates output by purpose:

| Field                 | Recipient                         |
| --------------------- | --------------------------------- |
| `Text`, `OutputParts` | The model                         |
| `Structured`          | Clients that need structured data |
| `Metadata`            | Clients that need display details |

If the definition declares `output_schema`, `Structured` must match it. Invalid output fails with `result_validation`.
Call `invocation.Progress.Report(outputDelta, metadata)` for live progress. Progress is not replayed.
Return partial output in the result if execution fails and you need to retain it.

Set `Definition.DirectoryScoped` to require a working directory.
Set `Definition.Sequential` to run the whole tool batch in order. The optional marker interfaces provide equivalent controls.
Set `Definition.ReplaySafe` only when repeating the same input is safe. Tool hooks must also tolerate repetition during recovery.

Tool errors return to the model. A tool error alone does not fail the turn.
