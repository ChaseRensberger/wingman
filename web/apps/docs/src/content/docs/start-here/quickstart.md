---
title: "Quick Start"
description: "Run Wingman locally and send your first agent message."
order: 2
---

# Quick Start

This guide uses the CLI and HTTP API. For the bundled browser UI, see [Use the Console](/use-wingman/web-ui).

## Prerequisites

- `curl`
- `jq`
- An Anthropic API key

## Install & Enable

Install Wingman with the release installer:

```bash
curl -fsSL https://wingman.actor/install | bash
```

The installer downloads the matching GitHub release archive and verifies it
against the release `checksums.txt` before it extracts the binary.

```bash
export PATH="$HOME/.wingman/bin:$PATH"
```

```bash
wingman up
```

To run it in the foreground instead, use `wingman serve`.

Load the owner credential for the local administration commands in this guide:

```bash
export WINGMAN_TOKEN=$(cat "${XDG_STATE_HOME:-$HOME/.local/state}/wingman/credential")
```

## Check that it is running

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

## Configure provider auth

Store your Anthropic API key in Wingman's local auth store. Replace `{key}` with your key:

```bash
export ANTHROPIC_API_KEY={key}
```

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The key is persisted in the server's SQLite database. Auth status responses only report whether a provider is configured; they do not return the secret.

## Create an agent

An agent is a reusable definition: instructions, allowed tools, model, and model options.

```bash
AGENT_ID=$(curl -sS -X POST http://localhost:2323/agents \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Quickstart Assistant",
    "instructions": "You are concise and helpful.",
    "tools": ["read", "glob", "grep"],
    "model_ref": "anthropic/claude-sonnet-5",
    "options": {"max_tokens": 1024}
  }' | jq -r .id)

printf 'agent: %s\n' "$AGENT_ID"
```

## Create a session

A session is the running conversation. It owns the message history and optional working directory.

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Quickstart\",\"working_directory\":\"$(pwd)\"}" | jq -r .id)

printf 'session: %s\n' "$SESSION_ID"
```

The working directory must already exist. Directory-scoped tools such as `read`, `glob`, `grep`, `write`, `edit`, `apply_patch`, and `bash` run relative to this session directory.

## Send a message

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "{\"request_id\":\"quickstart-1\",\"agent_id\":\"${AGENT_ID}\",\"message\":\"What files are in this directory?\"}" | jq
```

The response confirms that the server accepted the message. Use the session event stream for progress and completion:

```json
{
  "run_id": "run_...",
  "status": "queued",
  "session_version": 2
}
```

If the response is lost, repeating this request with the same `request_id` and
input returns the same run instead of queuing duplicate work.

## Stream a message

Subscribe to session events when you want lifecycle events as the agent runs:

```bash
curl -N "http://localhost:2323/sessions/${SESSION_ID}/events?after=0" \
  -H "Authorization: Bearer ${WINGMAN_TOKEN}" \
  -H "Accept: text/event-stream"
```

Each event is sent as server-sent events with an `event:` type and a JSON `data:` envelope.

## Next steps

- Read [Global Config](/configure/config) for server flags, storage, logs, and plugins.
- Read [Providers](/configure/providers) for provider auth and gateway routing.
- Read [Sessions](/concepts/sessions) for the session lifecycle and ephemeral runs.
- Read [API](/reference/referenceapi) if you are building your own client.
