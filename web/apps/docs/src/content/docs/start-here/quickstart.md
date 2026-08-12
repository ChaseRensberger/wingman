---
title: "Quick Start"
description: "Run Wingman locally and send your first agent message."
order: 2
---

# Quick Start

This guide uses the CLI and HTTP API. For the bundled browser UI, read [Use the Console](/use-wingman/web-ui).

## Prerequisites

- `curl`
- `jq`
- An Anthropic API key

## Install

```bash
curl -fsSL https://wingman.actor/install | bash
```

## Enable

```bash
wingman service start
```

To run Wingman in the foreground, use `wingman serve`.

Wingman runs as a per-user managed service. Managed native clients discover its
public registration at `~/.local/state/wingman/registration.json` and use the
generated private credentials in `~/.config/wingman/service.env` automatically.
For these `curl` commands, load the generated credentials:

```bash
source ~/.config/wingman/service.env
```

## Make Sure That It Is Running

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

## Configure provider auth

Store the Anthropic API key in the local Wingman auth store. Replace `{key}`
with your key:

```bash
export ANTHROPIC_API_KEY={key}
```

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The server saves the key in its SQLite database. Auth status responses report
only whether a provider is configured. They do not return the secret.

## Create an agent

An agent is a reusable definition. It contains instructions, allowed tools, a
model, and model options.

```bash
AGENT_ID=$(curl -sS -X POST http://localhost:2323/agents \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
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

A session is a running conversation. It contains the message history and an
optional working directory.

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Quickstart\",\"working_directory\":\"$(pwd)\"}" | jq -r .id)

printf 'session: %s\n' "$SESSION_ID"
```

The working directory must already exist. Directory-scoped tools such as
`read`, `glob`, `grep`, `write`, `edit`, `apply_patch`, and `bash` run relative
to this session directory.

## Send a message

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d "{\"request_id\":\"quickstart-1\",\"agent_id\":\"${AGENT_ID}\",\"message\":\"What files are in this directory?\"}" | jq
```

The response confirms that the server accepted the message. Use the session
event stream to get progress and completion:

```json
{
  "run_id": "run_...",
  "status": "queued",
  "session_version": 2
}
```

If the response is lost, repeat this request with the same `request_id` and
input. The server returns the same run instead of queuing duplicate work.

## Stream a message

If you want lifecycle events as the agent runs, subscribe to session events:

```bash
curl -N "http://localhost:2323/sessions/${SESSION_ID}/events?after=0" \
  -u "${WINGMAN_USERNAME:-wingman}:${WINGMAN_PASSWORD}" \
  -H "Accept: text/event-stream"
```

The server sends each event as a server-sent event. Each event has an `event:`
type and a JSON `data:` envelope.

## Next steps

- Read [Global Config](/configure/config) for server flags, storage, logs, and plugins.
- Read [Providers](/configure/providers) for provider auth and gateway routing.
- Read [Sessions](/concepts/sessions) for the session lifecycle and ephemeral runs.
- Read [API](/reference/referenceapi) if you are building your own client.
