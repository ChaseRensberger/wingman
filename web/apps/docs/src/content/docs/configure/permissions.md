---
title: "Permissions"
description: "Control which tool actions run automatically, ask for approval, or are blocked."
---

# Permissions

Permissions control what happens when an agent calls a tool.

Wingman has two layers:

| Layer | Purpose |
|---|---|
| Agent `tools` | Which tools the model can see and call. |
| Permission rules | Whether a tool call is allowed, asks for approval, or is denied. |

## Effects

Each matching rule resolves to one effect:

| Effect | Behavior |
|---|---|
| `allow` | Run the tool call. |
| `deny` | Block the tool call and return a model-visible permission error. |
| `ask` | Create a durable approval request and suspend before tool authorization. |

## Actions

Rules match an action and a resource.

| Action | Resource |
|---|---|
| `read` | File or directory path. |
| `edit` | File path for `edit` and `write`; every touched path for `apply_patch`. |
| `grep` | Search pattern. |
| `glob` | Glob pattern. |
| `bash` | Shell command string. |
| `webfetch` | URL. |
| `websearch` | Search query. |
| MCP or plugin tool name | `*` unless the tool has a first-class permission mapping in a later release. |

`edit`, `write`, and `apply_patch` all use the `edit` action because they mutate files.

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

Global permissions are runtime policy. They are not written into SQLite and do not mutate stored agents.

## Agent Overrides In Config

Use `agent_permissions` for daemon-local rules that apply to one stored agent.

Keys may be an agent ID or an agent name. If both match, the ID-specific rules run last.

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

Agents are stored in SQLite. Their `permissions` field is part of the agent definition and can be set through the agent API:

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

Rules are evaluated in order. The last matching rule wins.

This lets you put a catch-all first and exceptions after it:

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

The command `git status --short` is allowed. The command `git push origin main` is denied.

## Interactive Approval

When `ask` wins, Wingman persists a request before the tool is authorized or
started. The bundled console shows the action and every resource with three
choices:

- **Allow once** permits only the waiting call.
- **Always allow** permits the waiting call and remembers each exact
  action/resource pair for this session.
- **Reject** declines the tool call and returns a model-visible permission
  error.

Remembered grants are stored separately from authored Agent and daemon rules.
They satisfy later `ask` decisions in the same session, but cannot override a
`deny`. Pending requests time out after five minutes. Canceling the run or
stopping the daemon interrupts them without running the tool.

API clients can list and answer requests through the session permission
endpoints. A non-interactive Go `run.Config` without a `PermissionPrompter`
declines `ask` immediately; it never waits indefinitely.

## Client Behavior

Denied and rejected tool calls are returned as failed tool results. The
model-facing output remains plain text, but clients should use structured
metadata and durable permission records instead of parsing that text.

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

Persistent session streams emit durable `session.permission.requested` and
`session.permission.resolved` events containing the complete permission request.
Reload pending state from `GET /sessions/{id}/permission-requests`; do not depend
on having observed the original request event.

## Precedence

Effective permissions are assembled at run time:

1. Stored agent permissions from SQLite.
2. Global `permissions` from `wingman.json`.
3. Name-matched `agent_permissions` from `wingman.json`.
4. ID-matched `agent_permissions` from `wingman.json`.

Daemon-local config can restrict or refine stored agents without rewriting them.

## Supported Syntax

Use a single effect for everything:

```json
{ "permissions": "ask" }
```

Use action-level effects:

```json
{
  "permissions": {
    "read": "allow",
    "bash": "ask",
    "edit": "deny"
  }
}
```

Use action/resource maps for granular rules:

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

Resource patterns use simple wildcards: `*` matches any sequence, including `/`, and `?` matches one character.
