---
title: "Permissions"
description: "Allow tools, block actions, or require approval."
---

# Permissions

An agent's `tools` list decides which tools the model can call.
Permission rules decide whether each call runs. If no rule matches, Wingman allows the call.

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

When an `ask` rule matches, the Console offers three choices:

- Allow once permits the waiting call.
- Always allow remembers the exact action and resources for this session.
- Reject blocks the call and returns a permission error to the model.

Remembered approvals satisfy later `ask` rules, but cannot override `deny`.
Requests time out after five minutes. Cancellation or server shutdown interrupts a pending request before tool execution.

The one-shot `/run` endpoint declines `ask` immediately.
An embedded Go run also declines it unless you supply a `PermissionPrompter`.

## API clients

Read pending state from `GET /sessions/{id}/permission-requests`.
Reply with `once`, `always`, or `reject` through the [permission API](/reference/referenceapi#tools-and-approvals).
Saved streams emit `session.permission.requested` and `session.permission.resolved` with the complete request.

Use saved request status and tool-use metadata for program logic. Do not parse model-facing error text.
Permission failures include `metadata.permission` with `effect`, `action`, and `resource`.
