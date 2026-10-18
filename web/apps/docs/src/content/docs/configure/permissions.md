---
title: "Permissions"
description: "Allow tools, block actions, or require approval."
---

# Permissions

An agent's `tools` list decides which tools the model can call.
Permission rules decide whether each call runs.
Without a matching rule, file access outside the working directory requires approval. Other calls run without approval.

| Effect  | Result                                           |
| ------- | ------------------------------------------------ |
| `allow` | Run the tool.                                    |
| `ask`   | Wait for approval.                               |
| `deny`  | Block the call and return an error to the model. |

## Write rules

A rule matches an action and a resource. The action names the operation. The resource identifies its target.
Put server-wide rules in `wingman.json`:

```json
{
  "permissions": {
    "read": { "*": "allow", "*.env": "ask", "*.env.*": "ask", "*.env.example": "allow" },
    "edit": "ask",
    "bash": { "*": "ask", "git status*": "allow", "git push *": "deny" }
  }
}
```

Rules apply in the order written. The last matching rule wins.
Put general rules before exceptions. Keep that order when an application writes the JSON.
If a call touches several resources, a denial for any resource blocks the call.

| Action                  | Resource                                                         |
| ----------------------- | ---------------------------------------------------------------- |
| `read`                  | File or directory path                                           |
| `edit`                  | Paths changed by `edit`, `write`, or `apply_patch`               |
| `grep`, `glob`          | Search pattern                                                   |
| `bash`                  | Command string                                                   |
| `webfetch`              | URL                                                              |
| `websearch`             | Search query                                                     |
| `skill`                 | Skill ID                                                         |
| MCP or plugin tool name | `*`, unless the tool declares another action and resource fields |

Resource patterns use wildcards. `*` matches any sequence, including `/`. `?` matches one byte.
These are not filesystem glob rules. For example, `docs/*.md` also matches `docs/api/client.md`.

For one rule across all actions, use `"permissions": "ask"`.
The API also accepts ordered arrays of `{ "action": "bash", "resource": "*", "effect": "ask" }` rules.

## Working directory and outside access

The working directory sets the default location for file paths and commands.
It does not isolate the agent from other files on the server.
Permission rules control tool actions. They do not replace operating-system restrictions, a container, or a virtual machine.

File permissions use relative paths inside the working directory and absolute paths outside it.
Wingman follows symbolic links when it identifies the permission target, including links to files that do not yet exist.
Outside directory targets use a trailing `/*`.

Without a matching rule, `read`, `write`, `edit`, and `apply_patch` request approval for the outside directory.
An outside search with `grep` or `glob` also requires `read` approval, in addition to its search-pattern permission.
Read approval does not permit file changes. Wingman uses the separate `edit` action for changes.

To permit reads from a reference directory without approval, add rules to `wingman.json`:

```json
{
  "permissions": [
    { "action": "read", "resource": "~/projects/reference/*", "effect": "allow" },
    { "action": "edit", "resource": "~/projects/reference/*", "effect": "ask" }
  ]
}
```

Use `deny` instead of `ask` to block file changes there.
A broad rule such as `{ "action": "read", "resource": "*", "effect": "allow" }` also permits outside reads.
An outside directory approval covers its files and subdirectories. Search permissions apply to the requested root, not each matching file.

Use the `path` field to select a search directory.
Glob patterns must be relative and cannot contain `..` path components.
Recursive searches do not follow symbolic links. To search a linked directory, select it explicitly with `path`.

`bash` permissions match the command text, not the files that a command accesses.
An allowed command can access other files with the operating-system permissions of the Wingman process.
Use an isolated execution environment when outside files must remain inaccessible.

## Agent rules and precedence

Set stored agent `permissions` through the agent API.
Use `agent_permissions` in `wingman.json` for server-local overrides by agent name or ID:

```json
{
  "agent_permissions": {
    "Reviewer": { "edit": "deny", "bash": "deny" }
  }
}
```

Wingman applies these sources in order:

1. Stored agent permissions.
2. Global `permissions`.
3. Name-matched `agent_permissions`.
4. ID-matched `agent_permissions`.

Later matching rules win. File-based rules do not change the stored agent.

## Interactive approval

When an action requires approval, the Console offers three choices:

- Allow once permits the waiting call.
- Always allow remembers the action and listed resources for this session.
- Reject blocks the call and returns a permission error to the model.

Remembered approvals satisfy later `ask` rules, but cannot override `deny`.
Outside directory requests show an absolute directory ending in `/*`.
Remembering this request permits the same action in that directory and its subdirectories, but not in other sessions.
Stored session approvals survive a browser refresh or server restart. They have no time-based expiration.
Requests time out after five minutes. Cancellation or server shutdown interrupts a pending request before tool execution.

The one-shot `/run` endpoint declines `ask` immediately.
An embedded Go run also declines it unless you supply a `PermissionPrompter`.

## API clients

Read pending state from `GET /sessions/{id}/permission-requests`.
Reply with `once`, `always`, or `reject` through the [permission API](/reference/referenceapi#tools-and-approvals).
Saved streams emit `session.permission.requested` and `session.permission.resolved` with the complete request.

Use saved request status and tool-use metadata for program logic. Do not parse model-facing error text.
Permission failures include `metadata.permission` with `effect`, `action`, and `resource`.
