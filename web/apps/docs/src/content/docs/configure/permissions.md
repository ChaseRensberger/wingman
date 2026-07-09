---
title: "Permissions"
description: "Control which tool actions run automatically, ask for approval, or are blocked."
---

# Permissions

Permissions control what happens when a model calls a tool.

Wingman has two layers:

| Layer | Purpose |
|---|---|
| Agent `tools` | Which tools the model can see and call. |
| Permission rules | Whether a specific tool call is allowed, asks for approval, or is denied. |

If no permission rules are configured, Wingman preserves the historical behavior and allows configured tools to run.

## Effects

Each matching rule resolves to one effect:

| Effect | Behavior |
|---|---|
| `allow` | Run the tool call. |
| `deny` | Block the tool call and return a model-visible permission error. |
| `ask` | Require approval. In the current v1 implementation this blocks execution with a model-visible permission-required error and structured metadata; the interactive ask/reply API is planned next. |

## Actions

Rules match an action and a resource.

| Action | Resource |
|---|---|
| `read` | File or directory path. |
| `edit` | File path for `edit`, `write`, and `apply_patch`. |
| `grep` | Search pattern. |
| `glob` | Glob pattern. |
| `bash` | Shell command string. |
| `webfetch` | URL. |
| `websearch` | Search query. |
| plugin tool name | `*` unless the tool has a first-class permission mapping in a later release. |

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

Use `agent_permissions` for daemon-local overlays that apply to one stored agent.

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

## Client Behavior

Denied and approval-required tool calls are returned as failed tool results. The model-facing output remains plain text, but clients should use the structured metadata instead of parsing that text.

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

Approval-required example:

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

Future interactive approvals should use dedicated events such as `session.permission.asked` and `session.permission.replied`. Current v1 streams do not emit those events.

## Precedence

Effective permissions are assembled at run time:

1. Stored agent permissions from SQLite.
2. Global `permissions` from `wingman.json`.
3. Name-matched `agent_permissions` from `wingman.json`.
4. ID-matched `agent_permissions` from `wingman.json`.

This means daemon-local config can restrict or refine stored agents without rewriting them.

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

## Current Limits

Permissions are not a sandbox. They are a policy and UX layer before tool execution.

Current v1 limits:

- `ask` does not yet pause and wait for a client reply.
- `allow always` saved approvals are not implemented yet.
- `apply_patch` currently checks the broad `edit *` resource instead of each touched file path.
- External directory approvals are not separate because directory-scoped tools are contained to the session working directory.

For remote or multi-user deployments, Wingman still needs the deferred inbound-auth and sandbox story.
