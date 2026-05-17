---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

The `wingman` binary runs the local Wingman HTTP server and prints build information.

## Usage

```bash
wingman <command> [flags]
```

When working from the repository, replace `wingman` with `go run ./cmd/wingman`:

```bash
go run ./cmd/wingman <command> [flags]
```

## Commands

| Command | Description |
|---|---|
| `serve` | Start the HTTP server. |
| `version` | Print version information. |

## `wingman serve`

Starts the Wingman HTTP server. By default it binds to `127.0.0.1:2323` and stores data in SQLite at `~/.local/share/wingman/wingman.db`.

```bash
wingman serve [flags]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--host` | `127.0.0.1` | Host to bind to. |
| `--port` | `2323` | Port to listen on. |
| `--db` | `~/.local/share/wingman/wingman.db` | SQLite database path. |
| `--ephemeral` | `false` | Run without persistence. |
| `--log-format` | `json` | Log format: `json` or `text`. |
| `--log-level` | `info` | Log level: `debug`, `info`, `warn`, or `error`. |
| `--ui-dev` | none | Proxy `/web` to a Vite dev server URL. |

### Examples

Start with defaults:

```bash
wingman serve
```

Bind on all interfaces:

```bash
wingman serve --host 0.0.0.0 --port 2323
```

Use a custom database path:

```bash
wingman serve --db ./wingman.db
```

Run without persistence:

```bash
wingman serve --ephemeral
```

Proxy the embedded web route to a local Vite server during UI development:

```bash
wingman serve --ui-dev http://localhost:5173
```

## `wingman version`

Prints the binary version, commit, and build date.

```bash
wingman version
```

Example output:

```text
wingman dev (commit: none, built: unknown)
```
