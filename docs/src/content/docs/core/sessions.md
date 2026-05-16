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

This enables [Context Handoff](/concepts/context-handoff) between different provider/model combinations.

## Usage

When a client (consumer of Wingman) sends a message, it passes the agent to use for that turn:

```bash
curl -sS -X POST http://localhost:2323/sessions/ses_.../message \
  -H "Content-Type: application/json" \
  -d '{
        "agent_id": "builder",
        "message": "Fix the failing test."
      }'
```

Wingman loads the session history, loads the selected agent, builds the runtime configuration, runs the turn, and appends the resulting messages back to the same session.

The session table intentionally does not have an `agent_id` or `model_ref` column, if a client wants to display a default agent for a session, that is client UI state, not harness state.




