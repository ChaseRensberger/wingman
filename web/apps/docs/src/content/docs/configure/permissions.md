---
title: "Permissions"
description: "Control tool actions that run automatically, request approval, or are blocked."
---

# Permissions

Permissions allow tool calls, require approval, or block them.

Wingman has two layers:

| Layer            | Purpose                                                          |
| ---------------- | ---------------------------------------------------------------- |
| Agent `tools`    | Which tools the model can see and call.                          |
| Permission rules | Whether a tool call is allowed, requests approval, or is denied. |

## Effects

Each matching rule resolves to one effect:

| Effect  | Behavior                                                         |
| ------- | ---------------------------------------------------------------- |
| `allow` | Run the tool call.                                               |
| `deny`  | Block the tool call and return a model-visible permission error. |
| `ask`   | Wait for approval before running the tool.                       |

## Actions

Rules match an action and a resource.

| Action                  | Resource                                                                |
| ----------------------- | ----------------------------------------------------------------------- |
| `read`                  | File or directory path.                                                 |
| `edit`                  | File path for `edit` and `write`. Every touched path for `apply_patch`. |
| `grep`                  | Search pattern.                                                         |
| `glob`                  | Glob pattern.                                                           |
| `bash`                  | Shell command string.                                                   |
| `webfetch`              | URL.                                                                    |
| `websearch`             | Search query.                                                           |
| `skill`                 | Skill ID.                                                               |
| MCP or plugin tool name | `*`                                                                     |

`edit`, `write`, and `apply_patch` use the `edit` action because they change a file.

## Global Permissions

Put daemon-wide defaults in `~/.config/wingman/wingman.json`:

```json
{
  "permissions": {
    "read": {
      "*": "allow",
      "*.env": "ask",
      "*.env.*": "ask",
      "*.env.example": "allow"
    },
    "grep": "allow",
    "glob": "allow",
    "webfetch": "allow",
    "websearch": "allow",
    "edit": "ask",
    "bash": "ask"
  }
}
```

Global rules apply during a run without changing stored agents.

## Agent Overrides In Config

Use `agent_permissions` for daemon-local rules that apply to one stored agent.

Keys can be an agent ID or agent name. If both match, the ID-specific rules run last.

```json
{
  "agent_permissions": {
    "Plan": {
      "edit": "deny",
      "bash": "deny"
    },
    "Build": {
      "bash": {
        "*": "ask",
        "go test *": "allow",
        "go build *": "allow",
        "git status*": "allow",
        "git push *": "deny"
      }
    }
  }
}
```

## Stored Agent Permissions

Set an agent's `permissions` through the agent API:

```json
{
  "name": "Build",
  "tools": ["read", "grep", "glob", "edit", "write", "apply_patch", "bash"],
  "permissions": {
    "read": "allow",
    "grep": "allow",
    "glob": "allow",
    "edit": "ask",
    "bash": "ask"
  }
}
```

## Rule Order

Wingman evaluates rules in order. The last matching rule wins.

Put a catch-all first. Then put exceptions after it:

```json
{
  "permissions": {
    "bash": {
      "*": "ask",
      "git *": "allow",
      "git push *": "deny"
    }
  }
}
```

The command `git status --short` is allowed. The command `git push origin main`
is denied.

## Interactive Approval

When an `ask` rule matches, Wingman waits before running the tool.
The Console shows the action and resources with three choices:

- Allow once permits only the waiting call.
- Always allow permits the call and remembers the exact action and resources for this session.
- Reject blocks the call and returns a permission error to the model.

Remembered approvals satisfy later `ask` rules in the same session, but cannot override `deny`.
Requests time out after five minutes. Canceling the run or stopping Wingman interrupts them without running the tool.

API clients can list and answer requests through the session permission endpoints.
A non-interactive Go `run.Config` without a `PermissionPrompter` declines `ask` immediately.

## Client Behavior

Denied and rejected calls return a text error to the model.
Clients must read permission metadata and saved requests instead of parsing the error text.

Denied example:

```json
{
  "Output": "permission denied: bash git push origin main",
  "IsError": true,
  "Metadata": {
    "permission": {
      "effect": "deny",
      "action": "bash",
      "resource": "git push origin main"
    }
  }
}
```

Rejected approval example:

```json
{
  "Output": "permission required: edit src/main.go",
  "IsError": true,
  "Metadata": {
    "permission": {
      "effect": "ask",
      "action": "edit",
      "resource": "src/main.go"
    }
  }
}
```

Persistent session streams emit durable `session.permission.requested` and `session.permission.resolved` events.
Each event contains the complete permission request. Reload pending state from `GET /sessions/{id}/permission-requests`.
Do not depend on the original request event.

## Precedence

Wingman applies these rules in order. Later matching rules take precedence:

1. Stored agent permissions from SQLite.
2. Global `permissions` from `wingman.json`.
3. Name-matched `agent_permissions` from `wingman.json`.
4. ID-matched `agent_permissions` from `wingman.json`.

## Supported Syntax

To use one effect for everything, use:

```json
{ "permissions": "ask" }
```

To use action-level effects, use:

```json
{
  "permissions": {
    "read": "allow",
    "bash": "ask",
    "edit": "deny"
  }
}
```

To use action/resource maps for granular rules, use:

```json
{
  "permissions": {
    "edit": {
      "*": "ask",
      "docs/**/*.md": "allow"
    }
  }
}
```

Resource patterns use simple wildcards. `*` matches any sequence, including `/`.
`?` matches one character.
