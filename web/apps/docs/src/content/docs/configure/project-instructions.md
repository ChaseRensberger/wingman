---
title: "Project Instructions"
description: "Add global and project AGENTS.md instructions to Wingman runs."
order: 4
---

# Project Instructions

Put shared instructions in `AGENTS.md`. Wingman adds them to the agent's instructions for each run.

## Sources

Wingman reads these files in order:

1. `~/.config/wingman/AGENTS.md`, or the equivalent path under `XDG_CONFIG_HOME`.
2. `AGENTS.md` in the session working directory.

Wingman does not search parent or nested directories. A missing file is valid.
If Wingman cannot read an existing file, it rejects the new run.

If a session has no working directory, Wingman reads only the global file.
The prompt contains agent instructions, the current date, then these files.

## Run Snapshots

Saved runs capture these instructions when Wingman accepts the request. Later file edits affect only new runs.
Retrying the same `request_id` uses the saved instructions without reading the files again.
Ephemeral runs read the same files before execution but do not save a copy.

`AGENTS.md` adds instructions. It does not enable tools or override permission rules.
