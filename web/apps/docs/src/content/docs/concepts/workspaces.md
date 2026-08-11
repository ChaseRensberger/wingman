---
title: "Workspaces"
group: "Core"
order: 103
---

# Workspaces

A Workspace is a saved session context. It can point to a directory. It can also
work as a label and filter.

Each Workspace stores:

- A stable `wsp_` ID.
- A display name.
- An optional filesystem path.
- The owning Wingman client.

Users create Workspaces. If you omit `X-Wingman-Client`, `GET /workspaces` lists
Workspaces for the built-in `WingClient` client. It does not create a default
Workspace.

## Names and Paths

Wingman trims Workspace names. Names must not be empty. Names are unique for each
owning client without case sensitivity. If you create a Workspace with a path
and no name, Wingman uses the path's final directory name. A dirless Workspace
must have a name.

The Wingman server resolves and validates paths. It trims whitespace. It expands
`~` from the server process's home directory. It resolves relative paths from
the server process's current directory. The path must already exist. The path
must be a directory. It is not a browser-local path. Run the server where it
can access the directory.

## Create A Session In A Workspace

Create or reuse a Workspace. Then create a session with `workspace_id`:

These commands use `WINGMAN_DAEMON_PASSWORD` with HTTP Basic authentication. See [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" | jq -r '.[0].id')

SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -u "wingman:${WINGMAN_DAEMON_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Wingman records `workspace_id` on the session. If the Workspace has a path,
Wingman copies it to the session's `work_dir`. Dirless Workspaces create sessions
without a working directory. Later Workspace path edits do not rewrite existing
sessions. `POST /sessions/{id}/move` uses the same snapshot behavior when it
moves an existing session into a Workspace.

Do not send both `working_directory` and `workspace_id` when you create or move
a session. If the session belongs to a saved context, use `workspace_id`. For an
ad hoc directory, use `working_directory`.
