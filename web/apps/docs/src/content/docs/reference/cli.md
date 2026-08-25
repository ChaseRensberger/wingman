---
title: "CLI"
group: "Reference"
order: 999
description: "Wingman command-line interface reference."
---

# CLI

The `wingman` binary runs the local Wingman HTTP server. It manages the
background service, installs release updates, and prints build information.

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

The service runs for the user who runs the command. It does not require `sudo`.
Its public registration is `~/.local/state/wingman/registration.json`. Its
generated private credentials are `~/.config/wingman/service.env`. The default
SQLite database is `~/.local/share/wingman/wingman.db`. The command waits until
the service is ready.

`wingman service start` accepts the same runtime flags as `wingman serve`.
The command writes selected values to the generated service definition.

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
| `--port`       | `2323`                              | Port to listen on.                                   |
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

`wingman api` finds the verified managed daemon and uses its HTTP Basic Auth
credentials. Call an endpoint with an HTTP method and path:

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

## Console Command

Open the Console for the managed daemon:

```bash
wingman console
```

The Console uses the browser HTTP Basic Auth prompt for managed-service
credentials. It has no password form or session cookie.

## Service Commands

Check the generated service:

```bash
wingman service status
```

The command reports `ready`, `starting`, `stale`, `incompatible`, or missing.
It then shows the managed-service status.

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

`wingman update` downloads the latest stable GitHub release for the current
Linux or macOS architecture. It compares the downloaded archive with the
release `checksums.txt`. It atomically replaces the resolved executable:

```bash
wingman update
```

The executable directory must be writable. This works with the default installer
location (`~/.wingman/bin`) and other writable standalone installations. Update
an installation owned by a package manager or another user with its original
installation method.

The public installer also compares the downloaded release archive with the
published `checksums.txt` before it installs the binary.

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
