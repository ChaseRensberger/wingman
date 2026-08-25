---
title: "Permissions"
description: "Control tool actions that run automatically, request approval, or are blocked."
---

# Permissions

Permissions control the result when an agent calls a tool.

Wingman has two layers:

| Layer            | Purpose                                                          |
| ---------------- | ---------------------------------------------------------------- |
| Agent `tools`    | Which tools the model can see and call.                          |
| Permission rules | Whether a tool call is allowed, requests approval, or is denied. |

## Effects

Each matching rule resolves to one effect:

| Effect  | Behavior                                                                     |
| ------- | ---------------------------------------------------------------------------- |
| `allow` | Run the tool call.                                                           |
| `deny`  | Block the tool call and return a model-visible permission error.             |
| `ask`   | Create a durable approval request and suspend before the tool is authorized. |

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

Global permissions are runtime policy. SQLite does not store them. They do not
change stored agents.

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

SQLite stores agents. The `permissions` field is part of the agent definition.
Set it through the agent API:

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

When `ask` wins, Wingman stores a request before it authorizes or starts the tool.
The bundled console shows the action and each resource. It has three choices:

- **Allow once** permits only the waiting call.
- **Always allow** permits the waiting call. It remembers each exact action/resource pair for this session.
- **Reject** declines the tool call and returns a model-visible permission error.

Remembered grants are separate from authored agent and daemon rules. They satisfy later `ask` decisions in the same session.
They cannot override a `deny`. Pending requests time out after five minutes.
Canceling the run interrupts them without running the tool. Stopping the daemon also interrupts them without running the tool.

API clients can list and answer requests through the session permission endpoints.
A non-interactive Go `run.Config` without a `PermissionPrompter` declines `ask` immediately.
It does not wait indefinitely.

## Client Behavior

Denied and rejected tool calls return failed tool results. The model-facing output remains plain text.
Clients must use structured metadata and durable permission records instead of parsing that text.

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

Wingman assembles effective permissions at run time:

1. Stored agent permissions from SQLite.
2. Global `permissions` from `wingman.json`.
3. Name-matched `agent_permissions` from `wingman.json`.
4. ID-matched `agent_permissions` from `wingman.json`.

Daemon-local configuration can restrict or refine stored agents without rewriting them.

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
