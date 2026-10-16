---
title: "Workspaces"
---

# Workspaces

A workspace groups sessions and can supply a working directory. The directory belongs to the computer that runs Wingman.

Each workspace stores:

- A stable `wsp_` ID.
- A display name.
- An optional filesystem path.
- The owning Wingman client.

Create a workspace through the API, Console, or `wingman console <directory>`.
If you omit `X-Wingman-Client`, `GET /workspaces` lists workspaces for the built-in `WingClient` client.
Listing workspaces does not create a default workspace.

## Names and Paths

Names must be non-empty and unique within a client, ignoring case.
If you omit the name, Wingman uses the final directory name. A workspace without a path requires a name.

Paths must refer to existing directories on the Wingman server, not the browser's machine.
Wingman expands `~` from its home directory and relative paths from its current directory.

## Create A Session In A Workspace

Create a workspace on the local managed service, then create a session with `workspace_id`.
Run this example on the Wingman computer:

```bash
WORKSPACE_ID=$(wingman api createWorkspace \
  -d "$(jq -n --arg path "$PWD" '{path: $path}')" | jq -r .id)

SESSION_ID=$(wingman api createSession \
  -d "{\"title\":\"Explore repo\",\"workspace_id\":\"${WORKSPACE_ID}\"}" | jq -r .id)
```

Wingman copies the workspace path to the session's `work_dir` when you create or move the session.
Later workspace edits do not change existing sessions. A workspace without a path supplies no working directory.

Send either `workspace_id` or `working_directory`, not both, when creating or moving a session.
