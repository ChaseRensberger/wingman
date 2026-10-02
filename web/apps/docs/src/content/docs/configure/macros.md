---
title: "Macros"
description: "Create project macros for repeated Session instructions."
---

# Macros

A macro is a named template for a session message. It does not run shell commands or change later messages.

## Create A Macro

Put Markdown files in this directory:

```text
<working-directory>/.wingman/macros/
```

For example, create `.wingman/macros/review.md`:

```md
---
description: Review a change for errors and missing tests.
agent: reviewer
model: openai/gpt-5.6-terra
---

Review $ARGUMENTS. Report errors first, then missing tests.
```

The file body is the macro template. The `description`, `agent`, and `model`
fields are optional.

## Macro IDs

The relative file path without `.md` is the macro ID.

| File                                 | Macro ID          |
| ------------------------------------ | ----------------- |
| `.wingman/macros/review.md`          | `review`          |
| `.wingman/macros/review/security.md` | `review/security` |

Wingman reads only `.wingman/macros` below the Session working directory. It
does not search parent directories. It has no global macro directory.

## Arguments

Use `$ARGUMENTS` for all text after the macro ID:

```md
Review $ARGUMENTS for correctness.
```

Use `$1`, `$2`, and higher numbers for positional arguments. Single and double
quotes group text with spaces. The last numbered marker receives its argument
and all later arguments.

```md
Review $1. Focus on $2.
```

If the template has no argument marker, Wingman appends non-empty arguments
after a blank line.

Macro templates cannot use shell interpolation:

```text
!`git diff`
```

## Run A Macro

In the Console, type `/` to show project macros. Type a macro ID, then type its
arguments:

```text
/review Check the authentication changes and focus on missing tests.
```

Use the arrow keys, Tab, or Enter to select a macro from the list. The Console
sends the macro ID and the remaining text to Wingman.

Clients can list the macros for a Session:

```http
GET /sessions/{id}/macros
```

Clients run a macro with:

```http
POST /sessions/{id}/macros
Content-Type: application/json

{
  "request_id": "request-123",
  "macro_id": "review",
  "arguments": "Check the authentication changes.",
  "agent_id": "default-agent"
}
```

Retry the same input with the same `request_id` to avoid duplicate runs.
The macro's agent and model override the request values for that run only.
