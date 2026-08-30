---
title: "Skills"
description: "Install, create, and control local Agent Skills."
---

# Skills

Skills are Markdown instructions for specialized tasks. Wingman advertises a
skill to the model. The model loads the skill when the task matches its
description.

## Install A Skill

Install one skill in the current project:

```bash
wingman skills add https://github.com/aminblg/simpleenglish
```

The command requires Git. It performs a shallow clone. It accepts a repository
with exactly one discoverable skill. It prints the installed commit. It does not
replace an existing skill.

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

Paths in `SKILL.md` are relative to the skill directory. Wingman captures the
contents of referenced supporting files when it admits a persistent run.

Wingman also accepts one Markdown file at a skill source root:

```text
.wingman/skills/review.md
```

A flat skill cannot have supporting files.

## IDs And Frontmatter

The skill ID comes from the file path. It does not come from `name`.

| File | Skill ID |
| --- | --- |
| `skills/review.md` | `review` |
| `skills/release-notes/SKILL.md` | `release-notes` |
| `skills/team/release/SKILL.md` | `release` |

Use a unique lowercase kebab-case directory or file name. The `name` field is a
display name. The `description` tells the model when to load the skill.

Wingman does not advertise a skill without a description. The user can still
request that skill by its ID.

## Discovery

Wingman reads these sources in order:

| Scope | Directory |
| --- | --- |
| Global | `~/.config/wingman/skills` |
| Extra global | Each directory in `skills.dirs` |
| Project | `<working-directory>/.wingman/skills` |

A later source with the same ID replaces an earlier source. A session without a
working directory uses global sources only. Wingman does not search parent or
nested project directories.

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

See [Config Schema](/reference/config-schema#skills) for the field reference.

## Load A Skill

Wingman adds the native `skill` tool when it finds one or more skills. Do not
add `skill` to an Agent's `tools` list.

The model receives each available skill ID and description. It then loads a
matching skill. You can request a skill directly:

```text
Use the simple-english skill to rewrite this procedure.
```

The skill body does not enter the model context until the model loads it.

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

Persistent runs store the skill body and supporting-file contents at admission.
Later skill edits affect later runs only. A retry with the same `request_id`
returns the saved run. Wingman does not resolve skills again.

Ephemeral runs resolve skills before execution. They do not store a snapshot.
