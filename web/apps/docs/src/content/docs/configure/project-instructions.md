---
title: "Project Instructions"
description: "Add global and project AGENTS.md instructions to Wingman runs."
order: 4
---

# Project Instructions

`AGENTS.md` files add ambient instructions to an Agent for each run.

## Sources

Wingman reads these files in order:

1. `~/.config/wingman/AGENTS.md`, or the equivalent path under `XDG_CONFIG_HOME`.
2. `AGENTS.md` in the session working directory.

Wingman does not search parent or nested directories. A missing file is valid.
If Wingman cannot read a file, a new run admission fails.

If a session has no working directory, Wingman reads only the global file.
Wingman renders Agent instructions first, the current date second, and ambient
files last.

## Run Snapshots

Before Wingman admits a new persistent run, it resolves both files. The run
stores the authored Agent separately from the effective instructions.

The run also stores each source path, SHA-256 hash, resolution time, and order.
The run API returns this data. A later file change affects only later runs.

Retrying an admitted `request_id` returns its existing snapshot. Wingman does
not read the files again. Ephemeral runs resolve the same files before execution
but do not store a snapshot.

`AGENTS.md` changes model context. It does not enable tools or override Agent
and daemon permission rules.
