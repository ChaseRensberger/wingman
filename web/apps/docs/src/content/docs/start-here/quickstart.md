---
title: "Quick Start"
description: "Run Wingman locally and send your first agent message."
---

# Quick Start

This guide starts a background service and sends a message from your terminal.
Run the commands on the computer that will run Wingman. Use the same terminal for all steps.
For browser access, read [Use the Console](/use-wingman/web-ui).

## Prerequisites

- Linux with a user systemd service, or macOS with launchd
- Bash and `curl`
- `jq` for reading and creating JSON
- An Anthropic API key

## Install

Install Wingman:

```bash
curl -fsSL https://wingman.actor/install | bash
```

If your shell cannot find `wingman`, open a new terminal or add its default installation directory:

```bash
export PATH="$HOME/.wingman/bin:$PATH"
```

## Start the server

Start the service as your normal user:

```bash
wingman service start
```

`wingman api` connects to the local managed service and adds its credentials automatically.

## Read the server status

```bash
wingman api getHealth
```

Expected response:

```json
{ "status": "ok" }
```

## Configure provider auth

Store the Anthropic API key in Wingman. Replace `{key}` with your key:

```bash
export ANTHROPIC_API_KEY={key}
```

```bash
wingman api setProviderAuth \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

Wingman saves the key in its database. Auth status responses do not return the secret.

## Create an agent

An agent defines instructions, a model, and allowed tools.
Create an agent that can read and search files:

```bash
AGENT_ID=$(wingman api createAgent -d '{
    "name": "Quickstart Assistant",
    "instructions": "You are concise and helpful.",
    "tools": ["read", "glob", "grep"],
    "model_ref": "anthropic/claude-sonnet-5"
  }' | jq -r .id)

printf 'agent: %s\n' "$AGENT_ID"
```

## Create a session

A session stores a conversation and an optional working directory.

```bash
SESSION_ID=$(wingman api createSession \
  -d "$(jq -n --arg path "$PWD" '{title: "Quickstart", working_directory: $path}')" | jq -r .id)

printf 'session: %s\n' "$SESSION_ID"
```

The working directory must exist on the Wingman computer. The agent's file tools use this directory.

## Send a message

```bash
wingman api messageSession --param "id=${SESSION_ID}" \
  -d "{\"request_id\":\"quickstart-1\",\"agent_id\":\"${AGENT_ID}\",\"message\":\"What files are in this directory?\"}" | jq
```

A run records the work for one submitted message. Wingman saves the request in a queue and returns its run ID:

```json
{
  "run_id": "run_...",
  "status": "queued",
  "session_version": 2
}
```

If the response is lost, repeat this request with the same `request_id` and
input. The server returns the same run instead of queuing duplicate work.

## Read the response

Read the session events to follow progress and see the agent's response:

```bash
wingman api streamSessionEvents \
  --param "id=${SESSION_ID}" \
  --param after=0
```

Events include a type and JSON data. Press Ctrl+C to stop reading the stream without canceling the run.

To read the saved conversation afterward, run:

```bash
wingman api getSession --param "id=${SESSION_ID}" | jq .history
```

## Next steps

- Read [Global Configuration](/configure/config) for server flags, storage, logs, and plugins.
- Read [Providers](/configure/providers) for provider auth and gateway routing.
- Read [Sessions](/concepts/sessions) for the session lifecycle and ephemeral runs.
- Read [API](/reference/referenceapi) if you are building your own client.
