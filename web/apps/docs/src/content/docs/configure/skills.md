---
title: "Skills"
description: "Install, create, and control local Agent Skills."
---

# Skills

Skills are Markdown instructions for specific tasks. The model sees each skill description and can load its instructions when needed.

## Install A Skill

Install one skill in the current project:

```bash
wingman skills add https://github.com/aminblg/simpleenglish
```

The command requires Git and a repository with exactly one skill. It does not replace an existing skill.

Install one skill for all projects:

```bash
wingman skills add https://github.com/aminblg/simpleenglish --global
```

The project command installs files below this directory:

```text
<current-directory>/.wingman/skills/
```

The global command installs files below this directory:

```text
~/.config/wingman/skills/
```

If `XDG_CONFIG_HOME` is set, Wingman uses its `wingman` directory instead.

## Create A Skill

Use one directory for each skill. Put `SKILL.md` in the directory.

```text
.wingman/skills/
└── release-notes/
    ├── SKILL.md
    └── references/
        └── style.md
```

```md
---
name: Release Notes
description: Write concise release notes from merged changes.
---

Read `references/style.md`. Then write the release notes.
```

Paths in `SKILL.md` are relative to the skill directory.
Wingman saves up to ten supporting files when it accepts a saved run.
It selects files by sorted relative path, not by references in the instructions.
It excludes `.git` and nested skill directories. A supporting file cannot link outside the skill directory.

Wingman also accepts one Markdown file at a skill source root:

```text
.wingman/skills/review.md
```

A flat skill cannot have supporting files.

## IDs And Frontmatter

The skill ID comes from the file path. It does not come from `name`.

| File                            | Skill ID        |
| ------------------------------- | --------------- |
| `skills/review.md`              | `review`        |
| `skills/release-notes/SKILL.md` | `release-notes` |
| `skills/team/release/SKILL.md`  | `release`       |

Use a unique lowercase name with hyphens, such as `release-notes`.
The `name` field is for display. The `description` tells the model when to load the skill.

Wingman does not advertise a skill without a description. The user can still
request that skill by its ID.

## Discovery

Wingman reads these sources in order:

| Scope        | Directory or skill                    |
| ------------ | ------------------------------------- |
| Built-in     | `wingskill`                           |
| Global       | `~/.config/wingman/skills`            |
| Extra global | Each directory in `skills.dirs`       |
| Project      | `<working-directory>/.wingman/skills` |

A later source with the same ID replaces an earlier source.
A session without a working directory uses the built-in and global sources.
Wingman does not search other project directories. Within each skill source, it searches nested directories for `SKILL.md`.

The built-in `wingskill` loads Wingman documentation. A global or project skill with the same ID replaces it.

## Configure Sources

Add shared skill directories in `~/.config/wingman/wingman.json`:

```json
{
  "skills": {
    "dirs": ["~/shared-wingman-skills"]
  }
}
```

Restart the managed service after you change `wingman.json`:

```bash
wingman service restart
```

See [Configuration Schema](/reference/config-schema#skills) for the field reference.

## Load A Skill

Wingman adds the `skill` tool when it finds one or more skills. The built-in `wingskill` normally makes this tool available.
Do not add `skill` to an agent's `tools` list.

The model receives each available skill ID and description. It then loads a
matching skill. You can request a skill directly:

```text
Use the simple-english skill to rewrite this procedure.
```

The model does not receive the skill body until it loads the skill.
To load a captured supporting file, the model supplies the skill ID and relative `file` path to the `skill` tool.

## Permissions

The `skill` permission action controls skill loading. The resource is the skill
ID.

```json
{
  "permissions": {
    "skill": {
      "experimental-*": "ask",
      "internal-*": "deny"
    }
  }
}
```

A denied skill is not advertised. A denied load returns a permission error.
An `ask` rule creates an approval request.

## Run Snapshots

Saved runs capture skill instructions and supporting files when Wingman accepts the request.
Later edits affect new runs only. Retrying the same `request_id` uses the saved copy.
The built-in `wingskill` loads documentation from the running binary instead of saving page contents.

Ephemeral runs resolve skills before execution. They do not store a snapshot.
