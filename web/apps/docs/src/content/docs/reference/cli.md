---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

Use `wingman` to run the server, manage the background service, and install updates.

```bash
wingman <command> [flags]
```

## Commands

| Command           | Description                                                 |
| ----------------- | ----------------------------------------------------------- |
| `api`             | Make an authenticated request to the managed daemon.        |
| `serve`           | Start the HTTP server in the foreground.                    |
| `service start`   | Install, enable, and start Wingman as a background service. |
| `service stop`    | Stop and remove the Wingman background service.             |
| `service restart` | Restart the Wingman background service.                     |
| `service status`  | Show the Wingman background service status.                 |
| `pair`            | Show the managed server URL and credentials with a QR code. |
| `console`         | Open the managed daemon Console.                            |
| `clients create`  | Register an API client identity.                            |
| `skills add`      | Install a skill from an HTTPS Git repository.               |
| `update`          | Check for or install a verified release update.             |
| `version`         | Print version information.                                  |

## Server Commands

`wingman serve` starts the server in the foreground:

```bash
wingman serve
```

`wingman service start` installs and starts a background service:

```bash
wingman service start
```

The service runs for the current user without `sudo`. The command waits until it is ready.
Its URL is in `~/.local/state/wingman/registration.json` and credentials are in the private file `~/.config/wingman/service.env`.

`wingman service start` accepts the same runtime flags as `wingman serve`.
`wingman serve` prints its URL, username, and password when it uses generated
credentials. To show the managed service connection details later, run:

```bash
wingman pair
```

The command starts the managed service when it is absent, then prints its
connection URLs and HTTP Basic Auth credentials. It displays a QR code with the
same connection information. A service bound to `0.0.0.0` or `::` advertises
its non-loopback interface addresses.

## Runtime Flags

| Flag           | Default                             | Description                                          |
| -------------- | ----------------------------------- | ---------------------------------------------------- |
| `--host`       | `127.0.0.1`                         | Host to bind to.                                     |
| `--port`       | `2424`                              | Port to listen on.                                   |
| `--db`         | `~/.local/share/wingman/wingman.db` | SQLite database path.                                |
| `--ephemeral`  | `false`                             | Run without persistence.                             |
| `--log-format` | `json`                              | Log format: `json` or `text`.                        |
| `--log-level`  | `info`                              | Log level: `debug`, `info`, `warn`, or `error`.      |
| `--plugin-dir` | none                                | Additional global plugin directory. Can be repeated. |
| `--no-plugins` | `false`                             | Disable out-of-process plugin loading.               |

Examples:

```bash
wingman serve --host 127.0.0.1 --port 2424
wingman serve --db ./wingman.db
wingman serve --ephemeral
wingman service start --port 2424
```

## API Command

`wingman api` connects to the local managed service with its credentials.
Call an endpoint with an HTTP method and path:

```bash
wingman api get /sessions
wingman api post /sessions -d '{"title":"Explore repo"}'
```

You can also call an OpenAPI operation ID. The command loads operation IDs from
the managed daemon's `/openapi.json`. Use `--param name=value` for path and
query parameters:

```bash
wingman api createSession -d '{"title":"Explore repo"}'
wingman api getSession --param id=ses_...
wingman api streamSessionEvents --param id=ses_... --param after=0
```

To list the available operation IDs:

```bash
wingman api get /openapi.json | jq -r '.. | .operationId? // empty'
```

Use `-d` or `--data` for a request body. The command uses
`Content-Type: application/json` unless you set another value. Use repeatable
`-H` or `--header` flags for other request headers. Response bodies stream to
standard output. An HTTP error writes the response body and exits with an error.

The command supports the managed local daemon only. Use an HTTP client or a
Wingman SDK to connect to an explicit remote server.

## Skills Command

Install one skill for the current project:

```bash
wingman skills add https://github.com/aminblg/simpleenglish
```

Use `--global` to install below `~/.config/wingman/skills`. The command accepts
HTTPS Git repository URLs and requires Git. See [Skills](/configure/skills) for
skill files and configuration.

## Console Command

Open the Console for the managed daemon:

```bash
wingman console
```

To open a new session for the current directory, pass `.`:

```bash
wingman console .
```

You can also pass another directory, such as `wingman console ~/code/my-project`.
Wingman reuses the workspace for that directory or creates one if needed.
The new session uses that directory. The Console saves the session when you send its first message.
Without a directory, the command opens the Console home page.

Enter the service credentials in the browser HTTP Basic Auth prompt.

## Service Commands

Check the generated service:

```bash
wingman service status
```

The command reports `ready`, `starting`, `stale`, `incompatible`, or missing.
If you edit `~/.config/wingman/wingman.json`, restart the service:

```bash
wingman service restart
```

To change service flags such as `--host`, `--port`, `--db`, or `--plugin-dir`,
run `wingman service start` again with the new flags.

Stop and remove the service:

```bash
wingman service stop
```

## Version

Print the binary version, commit, and build date:

```bash
wingman version
```

Example output:

```text
wingman dev (commit: none, built: unknown)
```

## Update

`wingman update` downloads the latest stable release for your Linux or macOS architecture.
It compares the archive with `checksums.txt`, then replaces the executable:

```bash
wingman update
```

The executable directory must be writable, as it is at the default location, `~/.wingman/bin`.
For installations owned by a package manager or another user, use the original installation method.

If Wingman runs as a managed service, the command restarts it after replacement.
It does not start an installed service that is stopped.

Check availability without writing files:

```bash
wingman update --check
```

Install a particular stable or prerelease version:

```bash
wingman update --version 0.2.0-beta.1
```
