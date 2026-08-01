---
title: "Workspaces"
group: "Core"
order: 103
---

# Workspaces

A Workspace is a saved session context. It can point at a directory, or it can have no directory and act only as a label/filter.

Each Workspace stores:

- A stable `wsp_` ID.
- A display name.
- An optional filesystem path.
- The owning Wingman client.

Workspaces are user-created. If you omit `X-Wingman-Client`, `GET /workspaces` lists Workspaces for the built-in `Wingman` client, but it does not create a default Workspace.

## Names and Paths

Workspace names are trimmed and must be non-empty. They are unique per owning client, case-insensitively. When creating a Workspace with a path and no name, Wingman uses the path's final directory name; a dirless Workspace must provide a name.

Paths are resolved and validated by the Wingman server: surrounding whitespace is removed, `~` expands to that server process's home directory, and relative paths are made absolute from that process's current working directory. The path must already exist and be a directory. It is not a browser-local path, and Wingman does not resolve symlinks to a canonical path. Run the server on a machine that can access the directory you intend to use.

## Create A Session In A Workspace

Create or reuse a Workspace, then create a session with `workspace_id`:

These commands use `WINGMAN_TOKEN` from [HTTP API Basics](/build-clients/http-api-basics#authentication).

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" | jq -r '.[0].id')

SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Wingman records `workspace_id` on the session and copies the Workspace path into the session's `work_dir` when the Workspace has one. Dirless Workspaces create sessions without a working directory. Later Workspace path edits do not rewrite existing sessions. `POST /sessions/{id}/move` applies the same snapshot behavior when moving an existing session into a Workspace.

Do not send both `working_directory` and `workspace_id` when creating or moving a session. Use `workspace_id` when the session belongs to a saved context; use `working_directory` for an ad hoc directory.
