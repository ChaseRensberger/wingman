---
title: "Quickstart"
description: "Run Wingman locally and send your first agent message."
draft: false
order: 2
---

# Quickstart

This guide starts the Wingman HTTP server, creates an agent, creates a session, and sends one message.

## Prerequisites

- `curl`
- `jq`
- An Anthropic API key in `ANTHROPIC_API_KEY`

If you are working from the repository, you also need Go installed.

## Start the server

From a release install:

```bash
wingman serve
```

From the repository:

```bash
go run ./cmd/wingman serve
```

The server listens on `127.0.0.1:2323` by default and stores data in SQLite at `~/.local/share/wingman/wingman.db`.

Check that it is running:

```bash
curl -sS http://localhost:2323/health
```

Expected response:

```json
{ "status": "ok" }
```

## Configure provider auth

Store your Anthropic API key in Wingman's local auth store:

```bash
curl -sS -X PUT http://localhost:2323/provider/auth \
  -H "Content-Type: application/json" \
  -d "{\"providers\":{\"anthropic\":{\"type\":\"api_key\",\"key\":\"${ANTHROPIC_API_KEY}\"}}}"
```

The key is persisted in the server's SQLite database. Auth status responses only report whether a provider is configured; they do not return the secret.

## Create an agent

An agent is a reusable definition: instructions, allowed tools, provider, model, and model options.

```bash
AGENT_ID=$(curl -sS -X POST http://localhost:2323/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Quickstart Assistant",
    "instructions": "You are concise and helpful.",
    "tools": ["read", "glob", "grep"],
    "provider": "anthropic",
    "model": "claude-haiku-4-5",
    "options": {"max_tokens": 1024}
  }' | jq -r .id)

printf 'agent: %s\n' "$AGENT_ID"
```

## Create a session

A session is the running conversation. It owns the message history and optional working directory.

```bash
SESSION_ID=$(curl -sS -X POST http://localhost:2323/sessions \
  -H "Content-Type: application/json" \
  -d "{\"title\":\"Quickstart\",\"working_directory\":\"$(pwd)\"}" | jq -r .id)

printf 'session: %s\n' "$SESSION_ID"
```

The working directory must already exist. Directory-scoped tools such as `read`, `glob`, `grep`, `write`, `edit`, and `bash` run relative to this session directory.

## Send a message

```bash
curl -sS -X POST "http://localhost:2323/sessions/${SESSION_ID}/message" \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"${AGENT_ID}\",\"message\":\"What files are in this directory?\"}" | jq
```

The response includes the assistant's final text, any tool calls, token usage, and step count:

```json
{
  "response": "...",
  "tool_calls": [],
  "usage": { "input_tokens": 0, "output_tokens": 0 },
  "steps": 1
}
```

## Stream a message

Use the streaming endpoint when you want lifecycle events as the agent runs:

```bash
curl -N -X POST "http://localhost:2323/sessions/${SESSION_ID}/message/stream" \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d "{\"agent_id\":\"${AGENT_ID}\",\"message\":\"Summarize this project in one paragraph.\"}"
```

Each event is sent as server-sent events with an `event:` type and a JSON `data:` envelope.

## Next steps

- Read [Architecture](/architecture) for the high-level runtime shape.
- Read [Server](/server) for server flags, persistence, and route families.
- Read [SDK](/sdk) if you want to embed Wingman directly in a Go program.
