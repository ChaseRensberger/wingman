---
title: "Project Instructions"
description: "Add global and project AGENTS.md instructions to Wingman runs."
---

# Project Instructions

Put shared instructions in `AGENTS.md`. Wingman adds the file contents to the agent's instructions before each model call.
Create the file on the computer that runs Wingman.

## Sources

Wingman first reads `~/.config/wingman/AGENTS.md`, or the equivalent path under `XDG_CONFIG_HOME`.
It then reads ancestor `AGENTS.md` files, from the filesystem root through the session working directory.

After the `read` tool reads a file, Wingman discovers guidance in that file's parent directories.
For a symbolic link, discovery uses the resolved file location. Each instruction file appears once, even through multiple links.
Wingman retains discovered directories for the session. A missing instruction file is valid.
If Wingman cannot read an existing instruction file, it stops before the next model call.

If a session has no working directory, Wingman reads only the global file.
The prompt contains agent instructions, the current date, then these files.

## Changes and history

Wingman reads guidance again before each model call. Changes and deletions affect later calls, including calls in the same run.
Saved calls retain their instruction text, source paths, and discovery causes. Later file changes do not alter these records.
After a restart, successful file reads restore discovered directories, even if earlier conversation content was summarized.

The Console Inspector shows the input recorded for each call. A read entry links to the first affected call when it discovers guidance.
The saved run also retains its instructions at admission. That initial copy does not describe every later call.
Retrying the same `request_id` returns the existing run. Provider retries use the input prepared for that model step.
Ephemeral runs refresh guidance too, but do not retain historical input after execution.

`AGENTS.md` adds instructions. It does not enable tools or override permission rules.
