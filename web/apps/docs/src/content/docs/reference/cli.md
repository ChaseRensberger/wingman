---
title: "CLI"
description: "Wingman commands and runtime flags."
---

# CLI

Use `wingman <command> [flags]` to run the server and call its API.
Run `wingman <command> --help` for command-specific help.
See [Quick Start](/start-here/quickstart) for installation.

## Commands

| Command               | Purpose                                                        |
| --------------------- | -------------------------------------------------------------- |
| `serve`               | Run an HTTP server in your terminal.                           |
| `service start`       | Install and start the current user's background service.       |
| `service stop`        | Stop and remove the service.                                   |
| `service restart`     | Restart the installed service with its saved flags.            |
| `service status`      | Read service status.                                           |
| `api`                 | Send an authenticated request to the managed local service.    |
| `pair`                | Display the managed server URL, credentials, and QR code.      |
| `console [directory]` | Open the managed server's Console, optionally for a directory. |
| `clients create`      | Register a client identity.                                    |
| `skills add <url>`    | Install a skill from an HTTPS Git repository.                  |
| `update`              | Install the latest stable release.                             |
| `version`             | Print the version, commit, and build date.                     |

Service commands require Linux with systemd or macOS with launchd. Run them as your normal user, without `sudo`.
`pair` starts the managed service if needed. Treat its output as a password.
See [Authentication](/concepts/authentication) for connection details and remote access.

## Runtime flags

`serve` and `service start` accept these flags. Configuration-file values replace the defaults shown here.

| Flag           | Default                             | Purpose                                                     |
| -------------- | ----------------------------------- | ----------------------------------------------------------- |
| `--host`       | `127.0.0.1`                         | Listen address.                                             |
| `--port`       | `2424`                              | Listen port.                                                |
| `--db`         | `~/.local/share/wingman/wingman.db` | Database path.                                              |
| `--ephemeral`  | `false`                             | Run without saved resources.                                |
| `--log-format` | `json`                              | `json` or `text`.                                           |
| `--log-level`  | `info`                              | `debug`, `info`, `warn`, or `error`.                        |
| `--plugin-dir` | None                                | Extra global plugin directory. Repeat for more directories. |
| `--no-plugins` | `false`                             | Disable external plugins.                                   |

The managed service saves resolved runtime flags when you run `service start`.
To change those values, run `service start` again with the required flags or updated file.
A plain `service restart` keeps the saved flags, which still override file values.

## API command

`wingman api` finds the managed local server and adds its credentials.
Use an HTTP method and path, or an OpenAPI operation ID:

```bash
wingman api get /sessions
wingman api createSession -d '{"title":"Research"}'
wingman api streamSessionEvents --param id=ses_example --param after=0
```

Replace `ses_example` with an existing session ID.
Use `--param name=value` for path and query parameters.
The command reads operation IDs from the server's `/openapi.json`.

`-d` or `--data` supplies a JSON body. Repeat `-H` or `--header` for extra headers.
Response bodies stream to standard output. HTTP errors print the response body and exit with an error.
For a foreground or remote server, use direct HTTP or an SDK instead.

## Other commands

`wingman console .` opens a new session for the current directory.
It reuses or creates a workspace. The Console saves the session when you send its first message.
Without a directory, it opens the home page.

`wingman skills add <url> --global` installs a skill for all projects.
Without `--global`, it installs in the current project. The command requires Git.
See [Skills](/configure/skills) for repository requirements.

`wingman update --check` reads update availability without installing it.
`wingman update --version VERSION` installs a specific release.
See [Run the Server](/use-wingman/run-server#update-wingman) for update behavior and installation permissions.
