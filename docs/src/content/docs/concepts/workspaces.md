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

## Create A Session In A Workspace

Create or reuse a Workspace, then create a session with `workspace_id`:

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces | jq -r '.[0].id')

SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Wingman records `workspace_id` on the session and copies the Workspace path into the session's `work_dir` when the Workspace has one. Dirless Workspaces create sessions without a working directory. Later Workspace path edits do not rewrite existing sessions.

Do not send both `working_directory` and `workspace_id` when creating or updating a session. Use `workspace_id` when the session belongs to a saved context; use `working_directory` for an ad hoc directory.

## Web UI

The Sessions page treats Workspaces as optional filters, not as required session parents:

- `/web/sessions` shows all sessions.
- `/web/sessions?workspace=wsp_...` filters sessions to one Workspace.
- `/web/sessions?workspace=none` filters sessions without a Workspace.
- `/web/sessions/{session-id}` opens a session, whether or not it belongs to a Workspace.

Workspace create/edit/delete controls stay on the Sessions page instead of a separate top-level Workspace page.
