---
title: "Workspaces"
group: "Core"
order: 103
---

# Workspaces

A Workspace groups sessions and can supply a working directory.

Each Workspace stores:

- A stable `wsp_` ID.
- A display name.
- An optional filesystem path.
- The owning Wingman client.

Users create Workspaces. If you omit `X-Wingman-Client`, `GET /workspaces` lists Workspaces for the built-in `WingClient` client. It does not create a default Workspace.

## Names and Paths

Names must be non-empty and unique within a client, ignoring case.
If you omit the name, Wingman uses the final directory name. A Workspace without a path requires a name.

Paths must refer to existing directories on the Wingman server, not the browser's machine.
Wingman expands `~` from its home directory and relative paths from its current directory.

## Create A Session In A Workspace

Select an existing Workspace on the local managed service, then create a session with `workspace_id`:

```bash
WORKSPACE_ID=$(wingman api listWorkspaces | jq -r '.[0].id')

SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Wingman copies the Workspace path to the session's `work_dir` when you create or move the session.
Later Workspace edits do not change existing sessions. A Workspace without a path supplies no working directory.

Send either `workspace_id` or `working_directory`, not both, when creating or moving a session.
