---
title: "Sessions"
group: "Core"
order: 102
---

# Sessions

A session is the stateful execution record in Wingman. It holds the conversation history, drives model turns, dispatches tools calls, emits events, and in general, controls the lifecycle of an agent. `*session.Session` is a thin wrapper over `agent/loop`. 

This distinction is load-bearing:

- An agent is a reusable definition.
- A session is a running conversation.
- A session is not permanently bound to one agent.
- A session is not permanently bound to one model/`model_ref`.
- Each message chooses the agent configuration for that turn.

This enables context handoff between different provider/model combinations.

## Usage

```bash
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{
        "agent_id": "builder",
        "message": "Build me a B2B SAAS that has $10,000,000 ARR. Make not mistakes."
      }'
```

Wingman loads the session history, loads the selected agent, builds the runtime configuration, runs the turn, and appends the resulting messages back to the session.

The session table intentionally does not have an `agent_id` or `model_ref` column, if a client wants to display a default agent for a session, that is client UI state, not harness state.

## Run Ephemerally

Persistence is optional, many common llm tasks shouldn't be persisted. Maybe you want to build a client to automatically write git commit messages for you based on active changes. In this case you likely wouldn't need a session that is long running or save to disk. Think of this like your one-shot mode.

```bash
curl -sS -X POST http://localhost:2323/run \
  -H "Content-Type: application/json" \
  -d '{
        "agent_id": "builder",
        "message": "Build me a B2B SAAS that has $10,000,000 ARR. Make not mistakes."
      }'
```

## No sub-agents/sub-session

In Wingman, there is no concept of sub-sessions, sessions are perfectly capable of spinning up other sessions and reading their results but (at least at the moment) sessions don't have an optional parent reference.
