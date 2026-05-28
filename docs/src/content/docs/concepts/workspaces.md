---
title: "Workspaces"
group: "Core"
order: 103
---

# Workspaces

A Workspace is a persisted directory context. It gives a client a named place to start and optionally group sessions.

Each Workspace stores:

- A stable `wsp_` ID.
- A display name.
- A filesystem path.
- The owning Wingman client.

The built-in default client always has a default Workspace named `Wingman`. If you omit `X-Wingman-Client`, `GET /workspaces` uses the built-in `Wingman` client and creates that default Workspace if it does not already exist.

## Create A Session In A Workspace

Create or reuse a Workspace, then create a session with `workspace_id`:

```bash
WORKSPACE_ID=$(curl -sS http://localhost:2323/workspaces | jq -r '.[0].id')

SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Wingman copies the Workspace path into the session's `work_dir` and records `workspace_id` on the session. Later Workspace path edits do not rewrite existing sessions.

Do not send both `working_directory` and `workspace_id` when creating or updating a session. Use `workspace_id` when the session belongs to a persisted workspace; use `working_directory` for an ad hoc directory.

## Web UI

The Sessions page treats Workspaces as optional filters, not as required session parents:

- `/web/sessions` shows recent sessions and Workspace cards.
- `/web/sessions/workspaces/{workspace-slug}` shows sessions for one Workspace.
- `/web/sessions/{session-id}` opens a session, whether or not it belongs to a Workspace.

The `Sessions` breadcrumb returns to `/web/sessions` so you can switch context. Workspace create/edit/delete controls stay on the Sessions page instead of a separate top-level Workspace page.
